package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"tangoforge/internal/config"
	"tangoforge/internal/task"
	"testing"
)

// mockLLMResponse 返回固定内容的 mock OpenAI 服务。
func mockLLMResponse(t *testing.T, content string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := fmt.Sprintf(`{"choices":[{"message":{"content":%s}}]}`, strconv.Quote(content))
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// apiServerWithLLM 构造带 mock LLM 配置的 API Server。
func apiServerWithLLM(t *testing.T, llmBody string) (*Server, *httptest.Server) {
	t.Helper()
	llmSrv := mockLLMResponse(t, llmBody)
	srv := newAPIServer(t, func(cfg *config.GlobalConfig) {
		cfg.LLM = config.LLMConfig{BaseURL: llmSrv.URL, Model: "mock", APIKind: "openai", TimeoutSec: 5, Retries: 0}
	})
	t.Cleanup(func() { _ = srv.Close() })
	return srv, llmSrv
}

const importDoc = `{"tasks":[{"title":"导入任务A","status":"doing","priority":"high","tags":["x"]}]}`

func TestImport_FullDraftFlow(t *testing.T) {
	srv, _ := apiServerWithLLM(t, importDoc)
	dir := importProjectViaAPI(t, srv)

	// 1. POST /api/import → 草稿。
	body, _ := json.Marshal(map[string]any{"content": "# 文档\n", "source_file": "docs/import.md"})
	rec := uiReq(t, srv, http.MethodPost, "/api/import", dir, string(body))
	out := mustCode(t, rec, http.StatusOK, "import")
	var draftResp struct {
		Data struct {
			ID        string `json:"id"`
			Status    string `json:"status"`
			TaskCount int    `json:"task_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &draftResp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, out)
	}
	if draftResp.Data.Status != "pending" || draftResp.Data.TaskCount != 1 {
		t.Fatalf("draft: %+v", draftResp.Data)
	}
	draftID := draftResp.Data.ID

	// 2. 草稿列表。
	rec = uiReq(t, srv, http.MethodGet, "/api/import/drafts", dir, "")
	out = mustCode(t, rec, http.StatusOK, "drafts list")
	if !strings.Contains(out, draftID) {
		t.Fatalf("列表应含草稿: %s", out)
	}

	// 3. 确认入库。
	rec = uiReq(t, srv, http.MethodPost, "/api/import/drafts/"+draftID+"/confirm", dir, "")
	out = mustCode(t, rec, http.StatusOK, "confirm")
	if !strings.Contains(out, `"created":1`) {
		t.Fatalf("确认结果: %s", out)
	}

	// 4. 任务可查（doing + high→4）。
	rec = uiReq(t, srv, http.MethodGet, "/api/tasks", dir, "")
	out = mustCode(t, rec, http.StatusOK, "task list")
	var treeResp struct {
		Data struct {
			Tree []task.TaskTreeNode `json:"tree"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &treeResp); err != nil {
		t.Fatal(err)
	}
	if len(treeResp.Data.Tree) != 1 || treeResp.Data.Tree[0].Title != "导入任务A" ||
		treeResp.Data.Tree[0].Status != "doing" || treeResp.Data.Tree[0].Priority != 4 {
		t.Fatalf("导入任务不符: %+v", treeResp.Data.Tree)
	}

	// 5. 确认后草稿不再出现在 pending 列表。
	rec = uiReq(t, srv, http.MethodGet, "/api/import/drafts", dir, "")
	out = mustCode(t, rec, http.StatusOK, "drafts after confirm")
	if strings.Contains(out, draftID) {
		t.Fatalf("confirmed 草稿不应在 pending 列表: %s", out)
	}
}

func TestImport_Discard(t *testing.T) {
	srv, _ := apiServerWithLLM(t, importDoc)
	dir := importProjectViaAPI(t, srv)

	body, _ := json.Marshal(map[string]any{"content": "# 文档\n", "source_file": "docs/a.md"})
	rec := uiReq(t, srv, http.MethodPost, "/api/import", dir, string(body))
	out := mustCode(t, rec, http.StatusOK, "import")
	var draftResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(out), &draftResp)

	rec = uiReq(t, srv, http.MethodDelete, "/api/import/drafts/"+draftResp.Data.ID, dir, "")
	mustCode(t, rec, http.StatusOK, "discard")

	// 任务池无变化。
	rec = uiReq(t, srv, http.MethodGet, "/api/tasks", dir, "")
	out = mustCode(t, rec, http.StatusOK, "tasks")
	if !strings.Contains(out, `"total":0`) {
		t.Fatalf("丢弃后任务应为空: %s", out)
	}
}

func TestImport_LLMNotConfigured(t *testing.T) {
	srv := newAPIServer(t, nil) // LLM 默认空配置
	dir := importProjectViaAPI(t, srv)

	body, _ := json.Marshal(map[string]any{"content": "x", "source_file": "a.md"})
	rec := uiReq(t, srv, http.MethodPost, "/api/import", dir, string(body))
	out := mustCode(t, rec, http.StatusUnprocessableEntity, "import without LLM")
	if apiCode(t, out) != "LLM_NOT_CONFIGURED" {
		t.Fatalf("code=%s", apiCode(t, out))
	}
}

func TestImport_ParseFailure(t *testing.T) {
	srv, _ := apiServerWithLLM(t, `{"tasks":[{"status":"todo"}]}`) // 缺 title
	dir := importProjectViaAPI(t, srv)

	body, _ := json.Marshal(map[string]any{"content": "x", "source_file": "a.md"})
	rec := uiReq(t, srv, http.MethodPost, "/api/import", dir, string(body))
	out := mustCode(t, rec, http.StatusUnprocessableEntity, "parse failure")
	if apiCode(t, out) != "IMPORT_FAILED" {
		t.Fatalf("code=%s body=%s", apiCode(t, out), out)
	}
}

func TestImport_AgentDefaultDenied(t *testing.T) {
	srv, _ := apiServerWithLLM(t, importDoc)
	dir := importProjectViaAPI(t, srv)

	// agent 默认 import.run=false → 403。
	body, _ := json.Marshal(map[string]any{"content": "x", "source_file": "a.md"})
	rec := agentReq(t, srv, http.MethodPost, "/api/import", dir, string(body))
	mustCode(t, rec, http.StatusForbidden, "agent import denied")
}

func TestImport_DraftNotFound(t *testing.T) {
	srv, _ := apiServerWithLLM(t, importDoc)
	dir := importProjectViaAPI(t, srv)

	rec := uiReq(t, srv, http.MethodPost, "/api/import/drafts/no-such/confirm", dir, "")
	out := mustCode(t, rec, http.StatusNotFound, "confirm unknown draft")
	if apiCode(t, out) != "DRAFT_NOT_FOUND" {
		t.Fatalf("code=%s", apiCode(t, out))
	}
}

func TestImport_DraftReviewAndEdit(t *testing.T) {
	srv, _ := apiServerWithLLM(t, importDoc)
	dir := importProjectViaAPI(t, srv)

	// 1. 导入 → 草稿。
	body, _ := json.Marshal(map[string]any{"content": "# 文档\n", "source_file": "docs/review.md"})
	rec := uiReq(t, srv, http.MethodPost, "/api/import", dir, string(body))
	out := mustCode(t, rec, http.StatusOK, "import")
	var draftResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(out), &draftResp)
	draftID := draftResp.Data.ID

	// 2. GET 明细：含完整任务树（状态机 key / 优先级 / 标题）。
	rec = uiReq(t, srv, http.MethodGet, "/api/import/drafts/"+draftID, dir, "")
	out = mustCode(t, rec, http.StatusOK, "draft detail")
	for _, want := range []string{`"source_file":"docs/review.md"`, `"title":"导入任务A"`, `"status":"doing"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("明细缺 %s: %s", want, out)
		}
	}

	// 3. PUT 整体更新任务树（编辑保存）。
	edit := `{"tasks":[{"title":"导入任务A-改","description":"新描述","status":"todo","priority":4,"tags":["y"],"assignee":"PB","depends_on":[],"children":[]}]}`
	rec = uiReq(t, srv, http.MethodPut, "/api/import/drafts/"+draftID+"/tasks", dir, edit)
	mustCode(t, rec, http.StatusOK, "update tasks")
	rec = uiReq(t, srv, http.MethodGet, "/api/import/drafts/"+draftID, dir, "")
	out = mustCode(t, rec, http.StatusOK, "draft after edit")
	if !strings.Contains(out, "导入任务A-改") || !strings.Contains(out, `"priority":4`) {
		t.Fatalf("编辑未生效: %s", out)
	}

	// 4. 非法更新（title 空）→ 422。
	bad := `{"tasks":[{"title":"","status":"todo"}]}`
	rec = uiReq(t, srv, http.MethodPut, "/api/import/drafts/"+draftID+"/tasks", dir, bad)
	out = mustCode(t, rec, http.StatusUnprocessableEntity, "invalid update")
	if !strings.Contains(out, "title") {
		t.Fatalf("非法更新应 422: %s", out)
	}

	// 5. 确认导入使用编辑后的任务树。
	rec = uiReq(t, srv, http.MethodPost, "/api/import/drafts/"+draftID+"/confirm", dir, "")
	mustCode(t, rec, http.StatusOK, "confirm after edit")
	rec = uiReq(t, srv, http.MethodGet, "/api/tasks", dir, "")
	out = mustCode(t, rec, http.StatusOK, "tasks after confirm")
	if !strings.Contains(out, "导入任务A-改") {
		t.Fatalf("确认导入应使用编辑后标题: %s", out)
	}
}
