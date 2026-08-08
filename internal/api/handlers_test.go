package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"tangoforge/internal/audit"
	"tangoforge/internal/config"
	"tangoforge/internal/db"
)

// newAPIServer 构造完整 API Server（真实 NewServer 自组装依赖）。
// 调用方负责 defer srv.Close()（保证连接在 TempDir 清理前释放）。
func newAPIServer(t *testing.T, mutate func(*config.GlobalConfig)) *Server {
	t.Helper()
	cfg := config.DefaultGlobalConfig()
	cfg.UIToken = "ui-secret"
	cfg.APIToken = "api-secret"
	if mutate != nil {
		mutate(&cfg)
	}

	registry, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	if err := db.Migrate(context.Background(), registry, db.GlobalMigrations); err != nil {
		t.Fatalf("migrate registry: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewServer(&cfg, registry, logger, "", t.TempDir())
}

// doAPI 执行请求（remoteAddr 默认回环），返回响应记录器。
func doAPI(h http.Handler, method, target, body string, setHeader func(http.Header)) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rd)
	req.RemoteAddr = "127.0.0.1:5555"
	if setHeader != nil {
		setHeader(req.Header)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// uiReq 以 UI 身份请求（回环 + X-UI-Token + X-Project）。
func uiReq(t *testing.T, srv *Server, method, target, project, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doAPI(srv.Handler(), method, target, body, func(h http.Header) {
		h.Set("X-UI-Token", "ui-secret")
		if project != "" {
			h.Set("X-Project", project)
		}
	})
}

// agentReq 以 agent 身份请求（X-Actor + X-Project）。
func agentReq(t *testing.T, srv *Server, method, target, project, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doAPI(srv.Handler(), method, target, body, func(h http.Header) {
		h.Set("X-Actor", "human")
		if project != "" {
			h.Set("X-Project", project)
		}
	})
}

// mustCode 断言响应码并返回 body 字符串。
func mustCode(t *testing.T, rec *httptest.ResponseRecorder, want int, what string) string {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("%s: code=%d want=%d, body=%s", what, rec.Code, want, rec.Body.String())
	}
	return rec.Body.String()
}

// importProjectViaAPI 通过 HTTP 导入项目并返回 workdir。
func importProjectViaAPI(t *testing.T, srv *Server) string {
	t.Helper()
	dir := t.TempDir()
	body, _ := json.Marshal(map[string]string{"workdir": dir})
	rec := uiReq(t, srv, http.MethodPost, "/api/projects/import", "", string(body))
	mustCode(t, rec, http.StatusOK, "import project")
	return dir
}

// apiCode 解析统一响应中的 code 字段。
func apiCode(t *testing.T, body string) string {
	t.Helper()
	var resp struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	return resp.Code
}

// createTask 通过 API 建任务（UI），返回任务 ID。
func createTask(t *testing.T, srv *Server, project, title string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"title": title})
	rec := uiReq(t, srv, http.MethodPost, "/api/tasks", project, string(body))
	out := mustCode(t, rec, http.StatusCreated, "create task")
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}
	return created.Data.ID
}

func TestAPIFullChain_UI(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// 1. 建任务（UI）。
	createBody := `{"title":"写需求文档","description":"P3 语义","priority":"high","tags":["doc","p3"]}`
	rec := uiReq(t, srv, http.MethodPost, "/api/tasks", dir, createBody)
	body := mustCode(t, rec, http.StatusCreated, "create task")
	var created struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}
	if created.Data.ID == "" || created.Data.Status != "todo" {
		t.Fatalf("created: %+v", created.Data)
	}
	id := created.Data.ID

	// 2. 任务列表。
	rec = uiReq(t, srv, http.MethodGet, "/api/tasks", dir, "")
	mustCode(t, rec, http.StatusOK, "list tasks")

	// 3. 状态流转 todo→doing（PATCH 带 status）。
	rec = uiReq(t, srv, http.MethodPatch, "/api/tasks/"+id, dir, `{"status":"doing"}`)
	body = mustCode(t, rec, http.StatusOK, "change status")
	if !strings.Contains(body, `"doing"`) {
		t.Fatalf("status not changed: %s", body)
	}

	// 4. 非法流转 doing→archived 应 422 STATUS_NOT_FOUND。
	rec = uiReq(t, srv, http.MethodPatch, "/api/tasks/"+id, dir, `{"status":"archived"}`)
	body = mustCode(t, rec, http.StatusUnprocessableEntity, "illegal status archived")
	if apiCode(t, body) != "STATUS_NOT_FOUND" {
		t.Fatalf("want STATUS_NOT_FOUND, got %s", body)
	}

	// 5. 归档 → 还原。
	rec = uiReq(t, srv, http.MethodPost, "/api/tasks/"+id+"/archive", dir, "")
	body = mustCode(t, rec, http.StatusOK, "archive")
	if !strings.Contains(body, `"archived"`) {
		t.Fatalf("not archived: %s", body)
	}
	rec = uiReq(t, srv, http.MethodPost, "/api/tasks/"+id+"/restore", dir, `{"fallback_todo":false}`)
	body = mustCode(t, rec, http.StatusOK, "restore")
	if !strings.Contains(body, `"doing"`) {
		t.Fatalf("restore should back to doing: %s", body)
	}

	// 6. 归档后物理删除。
	rec = uiReq(t, srv, http.MethodPost, "/api/tasks/"+id+"/archive", dir, "")
	mustCode(t, rec, http.StatusOK, "re-archive before delete")
	rec = uiReq(t, srv, http.MethodDelete, "/api/tasks/"+id, dir, "")
	mustCode(t, rec, http.StatusOK, "physical delete")

	// 7. 审计记录已写入（轮询 audit.Store 落库）。
	waitAudit(t, srv, dir, func(res *auditQueryResult) bool { return res.total >= 5 })
}

func TestAPI_TaskCreateDenied(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// agent 默认无 task.create → 403 PERMISSION_DENIED。
	rec := agentReq(t, srv, http.MethodPost, "/api/tasks", dir, `{"title":"x"}`)
	body := mustCode(t, rec, http.StatusForbidden, "agent create denied")
	if apiCode(t, body) != "PERMISSION_DENIED" {
		t.Fatalf("want PERMISSION_DENIED, got %s", body)
	}

	// denied 审计已写入（轮询）。
	waitAudit(t, srv, dir, func(res *auditQueryResult) bool {
		for _, e := range res.entries {
			if e.Result == "denied" {
				return true
			}
		}
		return false
	})
}

func TestAPI_TaskReadGrantedForAgent(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)
	createTask(t, srv, dir, "t1")

	// agent 默认有 task.read → 200。
	rec := agentReq(t, srv, http.MethodGet, "/api/tasks", dir, "")
	mustCode(t, rec, http.StatusOK, "agent task.read")
}

func TestAPI_ProjectRemoveUIOnly(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	importProjectViaAPI(t, srv)

	// 列表拿 id。
	rec := uiReq(t, srv, http.MethodGet, "/api/projects", "", "")
	body := mustCode(t, rec, http.StatusOK, "project list")
	var list struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list.Data) == 0 {
		t.Fatalf("no project found: %s", body)
	}
	pid := list.Data[0].ID
	pidStr := strconv.FormatInt(pid, 10)

	// agent 删除 → 403。
	rec = agentReq(t, srv, http.MethodDelete, "/api/projects/"+pidStr, "", "")
	mustCode(t, rec, http.StatusForbidden, "agent remove denied")

	// UI 删除 → 200。
	rec = uiReq(t, srv, http.MethodDelete, "/api/projects/"+pidStr, "", "")
	mustCode(t, rec, http.StatusOK, "ui remove")
}

func TestAPI_ProjectRenameUIOnly(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	_ = importProjectViaAPI(t, srv)

	// 列表拿 id 与原名。
	rec := uiReq(t, srv, http.MethodGet, "/api/projects", "", "")
	body := mustCode(t, rec, http.StatusOK, "project list")
	var list struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list.Data) == 0 {
		t.Fatalf("no project found: %s", body)
	}
	pidStr := strconv.FormatInt(list.Data[0].ID, 10)
	oldName := list.Data[0].Name

	// agent 重命名 → 403。
	rec = agentReq(t, srv, http.MethodPatch, "/api/projects/"+pidStr, "", `{"name":"hijacked"}`)
	mustCode(t, rec, http.StatusForbidden, "agent rename denied")

	// UI 重命名 → 200 + 新名称回显。
	rec = uiReq(t, srv, http.MethodPatch, "/api/projects/"+pidStr, "", `{"name":"新名称"}`)
	body = mustCode(t, rec, http.StatusOK, "ui rename")
	var renamed struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &renamed); err != nil {
		t.Fatalf("unmarshal renamed: %v", err)
	}
	if renamed.Data.Name != "新名称" {
		t.Fatalf("重命名回显 %q", renamed.Data.Name)
	}

	// 列表确认变更。
	rec = uiReq(t, srv, http.MethodGet, "/api/projects", "", "")
	body = mustCode(t, rec, http.StatusOK, "project list after")
	_ = json.Unmarshal([]byte(body), &list)
	found := false
	for _, p := range list.Data {
		if p.Name == "新名称" {
			found = true
		}
	}
	if !found {
		t.Fatalf("列表未反映重命名: %s", body)
	}

	// 空名 → 422。
	rec = uiReq(t, srv, http.MethodPatch, "/api/projects/"+pidStr, "", `{"name":"  "}`)
	body = mustCode(t, rec, http.StatusUnprocessableEntity, "empty name")
	if apiCode(t, body) != "TASK_INVALID" {
		t.Fatalf("code %s", apiCode(t, body))
	}

	// 不存在 id → 404。
	rec = uiReq(t, srv, http.MethodPatch, "/api/projects/999999", "", `{"name":"x"}`)
	mustCode(t, rec, http.StatusNotFound, "rename not found")

	_ = oldName
}

func TestAPI_PermissionPutUIOnly(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// agent PUT 权限 → 403。
	rec := agentReq(t, srv, http.MethodPut, "/api/permissions", dir, `{"actions":{"task.create":true}}`)
	mustCode(t, rec, http.StatusForbidden, "agent permission put denied")

	// UI PUT → 200，并生效（agent 随后可建任务）。
	rec = uiReq(t, srv, http.MethodPut, "/api/permissions", dir, `{"actions":{"task.create":true}}`)
	mustCode(t, rec, http.StatusOK, "ui permission put")
	rec = agentReq(t, srv, http.MethodPost, "/api/tasks", dir, `{"title":"now-ok"}`)
	mustCode(t, rec, http.StatusCreated, "agent create after grant")
}

func TestAPI_StateMachineReadDeniedByDefault(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// agent 默认 state_machine.read=false → 403。
	rec := agentReq(t, srv, http.MethodGet, "/api/state-machine", dir, "")
	mustCode(t, rec, http.StatusForbidden, "agent state machine read denied")

	// UI 可读。
	rec = uiReq(t, srv, http.MethodGet, "/api/state-machine", dir, "")
	body := mustCode(t, rec, http.StatusOK, "ui state machine read")
	if !strings.Contains(body, "todo") {
		t.Fatalf("state machine should contain todo: %s", body)
	}
}

func TestAPI_RemoteNoToken401(t *testing.T) {
	// 远程访问开启（否则非回环直接 403 REMOTE_ACCESS_DISABLED）。
	srv := newAPIServer(t, func(c *config.GlobalConfig) { c.RemoteAccess = true })
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/tasks?project="+dir, nil)
	req.RemoteAddr = "192.168.1.5:1234"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("remote no token: code=%d, want 401", rec.Code)
	}
}

func TestAPI_RemoteWithToken(t *testing.T) {
	srv := newAPIServer(t, func(c *config.GlobalConfig) { c.RemoteAccess = true })
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/tasks?project="+dir, nil)
	req.RemoteAddr = "192.168.1.5:1234"
	req.Header.Set("Authorization", "Bearer api-secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("remote with valid token should pass task.read, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestAPI_ProjectImportInvalid(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	rec := uiReq(t, srv, http.MethodPost, "/api/projects/import", "", `{"workdir":"relative/path"}`)
	body := mustCode(t, rec, http.StatusBadRequest, "invalid workdir")
	if apiCode(t, body) != "PROJECT_INVALID" {
		t.Fatalf("want PROJECT_INVALID, got %s", body)
	}
}

func TestAPI_PermissionGetFullList(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	rec := agentReq(t, srv, http.MethodGet, "/api/permissions", dir, "")
	body := mustCode(t, rec, http.StatusOK, "permission get")
	// 全量 16 项（含 allowed=false）。
	if !strings.Contains(body, `"task.create":false`) {
		t.Fatalf("should contain denied actions: %s", body)
	}
	if !strings.Contains(body, `"task.read":true`) {
		t.Fatalf("should contain granted actions: %s", body)
	}
}

func TestAPI_ErrorCodeMappings(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// 不存在任务 → 404 TASK_NOT_FOUND。
	rec := uiReq(t, srv, http.MethodGet, "/api/tasks/does-not-exist", dir, "")
	body := mustCode(t, rec, http.StatusNotFound, "task not found")
	if apiCode(t, body) != "TASK_NOT_FOUND" {
		t.Fatalf("want TASK_NOT_FOUND, got %s", body)
	}

	// 依赖环 → 422 CIRCULAR_DEPENDENCY：建 A、B，让 B 依赖 A 后 A 依赖 B。
	createA, _ := json.Marshal(map[string]any{"title": "A"})
	rec = uiReq(t, srv, http.MethodPost, "/api/tasks", dir, string(createA))
	var ta struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(rec.Body.String()), &ta)
	createB, _ := json.Marshal(map[string]any{"title": "B", "depends_on": []string{ta.Data.ID}})
	rec = uiReq(t, srv, http.MethodPost, "/api/tasks", dir, string(createB))
	var tb struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(rec.Body.String()), &tb)
	// A 依赖 B → 环。
	updateA, _ := json.Marshal(map[string]any{"depends_on": []string{tb.Data.ID}})
	rec = uiReq(t, srv, http.MethodPatch, "/api/tasks/"+ta.Data.ID, dir, string(updateA))
	body = mustCode(t, rec, http.StatusUnprocessableEntity, "circular dependency")
	if apiCode(t, body) != "CIRCULAR_DEPENDENCY" {
		t.Fatalf("want CIRCULAR_DEPENDENCY, got %s", body)
	}
}

// auditQueryResult 审计查询辅助结果。
type auditQueryResult struct {
	total   int
	entries []struct {
		Result string `json:"result"`
	}
}

// waitAudit 轮询审计落库直至条件满足。
func waitAudit(t *testing.T, srv *Server, workdir string, cond func(*auditQueryResult) bool) {
	t.Helper()
	for i := 0; i < 50; i++ {
		res, err := srv.audit.Query(t.Context(), workdir, audit.Filter{})
		if err == nil {
			r := &auditQueryResult{total: res.Total}
			for _, e := range res.Items {
				r.entries = append(r.entries, struct {
					Result string `json:"result"`
				}{Result: e.Result})
			}
			if cond(r) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("audit condition not met within 1s")
}

// TF-041：POST /api/projects/check 返回目录前置状态（未注册/无元数据/有元数据合法/非法）。
func TestProjectCheck(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()

	// 全新目录：未注册、无元数据。
	dir := t.TempDir()
	body, _ := json.Marshal(map[string]string{"workdir": dir})
	rec := uiReq(t, srv, http.MethodPost, "/api/projects/check", "", string(body))
	out := mustCode(t, rec, http.StatusOK, "check fresh")
	var check struct {
		Data struct {
			Registered bool   `json:"registered"`
			HasMeta    bool   `json:"has_meta"`
			MetaValid  bool   `json:"meta_valid"`
			MetaReason string `json:"meta_reason"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &check); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, out)
	}
	if check.Data.Registered || check.Data.HasMeta {
		t.Fatalf("fresh dir: %+v", check.Data)
	}

	// 导入后：已注册 + 有元数据 + 合法。
	dir2 := importProjectViaAPI(t, srv)
	body, _ = json.Marshal(map[string]string{"workdir": dir2})
	rec = uiReq(t, srv, http.MethodPost, "/api/projects/check", "", string(body))
	out = mustCode(t, rec, http.StatusOK, "check imported")
	if err := json.Unmarshal([]byte(out), &check); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, out)
	}
	if !check.Data.Registered || !check.Data.HasMeta || !check.Data.MetaValid {
		t.Fatalf("imported dir: %+v", check.Data)
	}

	// 手动造非法元数据：仅 .taskboard/ 目录无 config.yaml → 非法。
	dir3 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir3, ".taskboard"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(map[string]string{"workdir": dir3})
	rec = uiReq(t, srv, http.MethodPost, "/api/projects/check", "", string(body))
	out = mustCode(t, rec, http.StatusOK, "check bad meta")
	if err := json.Unmarshal([]byte(out), &check); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, out)
	}
	if !check.Data.HasMeta || check.Data.MetaValid {
		t.Fatalf("bad meta dir: %+v", check.Data)
	}
}

// TF-041：POST /api/projects/import/reset 清空历史元数据（仅 UI；未注册目录可清）。
func TestProjectResetMetadata(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".taskboard"), 0o755); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(dir, ".taskboard")
	if _, err := os.Stat(metaPath); err != nil {
		t.Fatal(err)
	}

	// Agent 调用 → 403（仅 UI）。
	body, _ := json.Marshal(map[string]string{"workdir": dir})
	rec := agentReq(t, srv, http.MethodPost, "/api/projects/import/reset", "", string(body))
	mustCode(t, rec, http.StatusForbidden, "reset agent forbidden")

	// UI 调用 → 清空。
	rec = uiReq(t, srv, http.MethodPost, "/api/projects/import/reset", "", string(body))
	mustCode(t, rec, http.StatusOK, "reset ui")
	if _, err := os.Stat(metaPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".taskboard 应被删除: %v", err)
	}

	// 已注册项目禁止重置。
	dir2 := importProjectViaAPI(t, srv)
	body, _ = json.Marshal(map[string]string{"workdir": dir2})
	rec = uiReq(t, srv, http.MethodPost, "/api/projects/import/reset", "", string(body))
	out := mustCode(t, rec, http.StatusBadRequest, "reset registered forbidden")
	if apiCode(t, out) != "PROJECT_INVALID" {
		t.Fatalf("code=%s body=%s", apiCode(t, out), out)
	}
}
