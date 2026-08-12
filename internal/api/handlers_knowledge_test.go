package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAPI_Knowledge_FullLifecycle 知识库端点全链路（TF-050 验收）：
// 注册 → 关联任务 → 检索（未配置 embedding 时 422）→ 编辑 → relink → 解除。
func TestAPI_Knowledge_FullLifecycle(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// 1. 库列表（默认库存在）。
	rec := uiReq(t, srv, http.MethodGet, "/api/knowledge/bases", dir, "")
	body := mustCode(t, rec, http.StatusOK, "bases list")
	if !strings.Contains(body, "默认库") {
		t.Fatalf("默认库应存在: %s", body)
	}

	// 2. 创建库。
	rec = uiReq(t, srv, http.MethodPost, "/api/knowledge/bases", dir, `{"name":"spec","description":"规格"}`)
	body = mustCode(t, rec, http.StatusCreated, "create base")
	var kbResp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(body), &kbResp)

	// 3. 注册文档（项目内）。
	abs := writeTestFile(t, dir, "docs/spec.md", "# 规格文档\n内容")
	rec = uiReq(t, srv, http.MethodPost, "/api/knowledge/documents", dir,
		fmt.Sprintf(`{"path":%q,"copy":"auto","kb_ids":[%d]}`, abs, kbResp.Data.ID))
	body = mustCode(t, rec, http.StatusCreated, "register doc")
	var docResp struct {
		Data struct {
			ID      string `json:"id"`
			AbsPath string `json:"abs_path"`
			Type    string `json:"type"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(body), &docResp)
	if docResp.Data.ID == "" || docResp.Data.Type != "text" {
		t.Fatalf("注册文档异常: %s", body)
	}

	// 4. 建任务并关联。
	var taskID string
	rec = uiReq(t, srv, http.MethodPost, "/api/tasks", dir, `{"title":"接口改造"}`)
	body = mustCode(t, rec, http.StatusCreated, "create task")
	var taskResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(body), &taskResp)
	taskID = taskResp.Data.ID

	rec = uiReq(t, srv, http.MethodPost, "/api/knowledge/link", dir,
		fmt.Sprintf(`{"task_id":%q,"document_id":%q}`, taskID, docResp.Data.ID))
	mustCode(t, rec, http.StatusOK, "link task")

	// 5. 任务详情内嵌知识库文档（TF-050）。
	rec = uiReq(t, srv, http.MethodGet, "/api/tasks/"+taskID, dir, "")
	body = mustCode(t, rec, http.StatusOK, "task detail")
	if !strings.Contains(body, "knowledge_documents") || !strings.Contains(body, "spec.md") {
		t.Fatalf("任务详情应内嵌知识库文档: %s", body)
	}

	// 6. 检索（无 embedding 配置 → 422 EMBEDDING_NOT_CONFIGURED）。
	rec = uiReq(t, srv, http.MethodGet, "/api/knowledge/search?q=规格", dir, "")
	body = mustCode(t, rec, http.StatusUnprocessableEntity, "search no embed")
	if apiCode(t, body) != "EMBEDDING_NOT_CONFIGURED" {
		t.Fatalf("未配置 embedding 应 NOT_CONFIGURED: %s", body)
	}

	// 7. 阅读原文。
	rec = uiReq(t, srv, http.MethodGet, "/api/knowledge/documents/"+docResp.Data.ID+"/content", dir, "")
	body = mustCode(t, rec, http.StatusOK, "read content")
	if !strings.Contains(body, "规格文档") {
		t.Fatalf("阅读原文应含内容: %s", body)
	}

	// 8. 编辑原文 → 内容更新。
	rec = uiReq(t, srv, http.MethodPut, "/api/knowledge/documents/"+docResp.Data.ID+"/content", dir,
		`{"content":"# 更新后的规格文档\n新内容"}`)
	mustCode(t, rec, http.StatusOK, "edit content")
	data, _ := os.ReadFile(docResp.Data.AbsPath)
	if !strings.Contains(string(data), "更新后的规格文档") {
		t.Fatalf("写盘应更新: %s", string(data))
	}

	// 9. relink 到新路径。
	newAbs := writeTestFile(t, dir, "docs/new.md", "# 新路径文档")
	rec = uiReq(t, srv, http.MethodPost, "/api/knowledge/documents/"+docResp.Data.ID+"/relink", dir,
		fmt.Sprintf(`{"new_path":%q,"copy":"none"}`, newAbs))
	body = mustCode(t, rec, http.StatusOK, "relink")
	if !strings.Contains(body, "new.md") {
		t.Fatalf("relink 应更新路径: %s", body)
	}

	// 10. 解除任务关联 + 删除文档。
	rec = uiReq(t, srv, http.MethodPost, "/api/knowledge/unlink", dir,
		fmt.Sprintf(`{"task_id":%q,"document_id":%q}`, taskID, docResp.Data.ID))
	mustCode(t, rec, http.StatusOK, "unlink")
	rec = uiReq(t, srv, http.MethodDelete, "/api/knowledge/documents/"+docResp.Data.ID, dir, "")
	mustCode(t, rec, http.StatusOK, "delete doc")
	// 删除后详情 → 404。
	rec = uiReq(t, srv, http.MethodGet, "/api/knowledge/documents/"+docResp.Data.ID, dir, "")
	mustCode(t, rec, http.StatusNotFound, "doc gone")
}

// TestAPI_Knowledge_Permissions 知识库权限（QA-K12：read 默认只读；write/index 默认拒绝）。
func TestAPI_Knowledge_Permissions(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// agent 读（knowledge.read 默认 true）→ 200。
	rec := agentReq(t, srv, http.MethodGet, "/api/knowledge/bases", dir, "")
	mustCode(t, rec, http.StatusOK, "agent read bases")

	// agent 写（knowledge.write 默认 false）→ 403。
	rec = agentReq(t, srv, http.MethodPost, "/api/knowledge/bases", dir, `{"name":"x"}`)
	body := mustCode(t, rec, http.StatusForbidden, "agent create base")
	if apiCode(t, body) != "PERMISSION_DENIED" {
		t.Fatalf("agent 写应 403: %s", body)
	}

	// agent 扫描（knowledge.index 默认 false）→ 403。
	rec = agentReq(t, srv, http.MethodPost, "/api/knowledge/scan", dir, "")
	body = mustCode(t, rec, http.StatusForbidden, "agent scan")
	if apiCode(t, body) != "PERMISSION_DENIED" {
		t.Fatalf("agent scan 应 403: %s", body)
	}

	// UI 扫描 → 200（index 权限 UI 放行）。
	rec = uiReq(t, srv, http.MethodPost, "/api/knowledge/scan", dir, "")
	mustCode(t, rec, http.StatusOK, "ui scan")
}

// TestAPI_Knowledge_BinaryContent 二进制文档阅读仅返回路径。
func TestAPI_Knowledge_BinaryContent(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	abs := writeTestFile(t, dir, "img/logo.png", "\x89PNG\x0d\x0a\x1a\x0a")
	rec := uiReq(t, srv, http.MethodPost, "/api/knowledge/documents", dir,
		fmt.Sprintf(`{"path":%q}`, abs))
	body := mustCode(t, rec, http.StatusCreated, "register binary")
	var resp struct {
		Data struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(body), &resp)
	if resp.Data.Type != "binary" {
		t.Fatalf("应注册为 binary: %s", body)
	}
	// 阅读 → 仅路径不返回内容。
	rec = uiReq(t, srv, http.MethodGet, "/api/knowledge/documents/"+resp.Data.ID+"/content", dir, "")
	body = mustCode(t, rec, http.StatusOK, "read binary")
	if strings.Contains(body, "PNG") {
		t.Fatalf("二进制不应返回内容: %s", body)
	}
	if !strings.Contains(body, "logo.png") {
		t.Fatalf("应返回路径: %s", body)
	}
	// 编辑二进制 → 422。
	rec = uiReq(t, srv, http.MethodPut, "/api/knowledge/documents/"+resp.Data.ID+"/content", dir,
		`{"content":"x"}`)
	mustCode(t, rec, http.StatusUnprocessableEntity, "edit binary")
}

// writeTestFile 在 workdir 下写文件返回绝对路径。
func writeTestFile(t *testing.T, workdir, rel, content string) string {
	t.Helper()
	abs := filepath.Join(workdir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return abs
}
