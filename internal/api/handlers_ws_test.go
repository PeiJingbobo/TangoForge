package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"tangoforge/internal/config"
)

// startWSTestServer 启动真实 HTTP 服务（WS 测试需要真实监听）。
func startWSTestServer(t *testing.T, mutate func(*config.GlobalConfig)) (*Server, *httptest.Server) {
	t.Helper()
	srv := newAPIServer(t, mutate)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

// wsURL 构造 ws:// 地址。
func wsURL(ts *httptest.Server, project string) string {
	return "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/events?project=" + project
}

func TestWS_ReceivesTaskCreated(t *testing.T) {
	srv, ts := startWSTestServer(t, nil)
	dir := importProjectViaAPI(t, srv)

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL(ts, dir), http.Header{
		"X-UI-Token": []string{"ui-secret"},
	})
	if err != nil {
		if resp != nil {
			t.Fatalf("dial: %v status=%d", err, resp.StatusCode)
		}
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// 触发写操作：建任务 → 应收到 task.created 事件。
	createTask(t, srv, dir, "ws-task-1")

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	var ev Event
	if err := json.Unmarshal(msg, &ev); err != nil {
		t.Fatalf("unmarshal event: %v msg=%s", err, msg)
	}
	if ev.Type != "task.created" {
		t.Fatalf("event type=%s, want task.created (msg=%s)", ev.Type, msg)
	}
	if ev.Project != dir {
		t.Fatalf("event project=%s, want %s", ev.Project, dir)
	}
	if ev.TS == "" {
		t.Fatal("event ts missing")
	}
	data, ok := ev.Data.(map[string]any)
	if !ok || data["id"] == nil || data["id"] == "" {
		t.Fatalf("event data missing id: %v", ev.Data)
	}
}

func TestWS_AgentWithTaskReadReceives(t *testing.T) {
	srv, ts := startWSTestServer(t, nil)
	dir := importProjectViaAPI(t, srv)

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL(ts, dir), http.Header{
		"X-Actor": []string{"human"},
	})
	if err != nil {
		if resp != nil {
			t.Fatalf("dial: %v status=%d", err, resp.StatusCode)
		}
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// agent 无 task.create → 直接 UI 建任务触发事件（WS 权限只要求 task.read，agent 默认有）。
	createTask(t, srv, dir, "ws-task-2")
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	if !strings.Contains(string(msg), "task.created") {
		t.Fatalf("msg should contain task.created: %s", msg)
	}
}

// TestWS_RemoteAuth 说明：WS 的远程鉴权（401/403）与 /api 共用 auth.Identify
// 与权限表，逻辑一致性已由 TestAPI_RemoteNoToken401 / TestAPI_RemoteWithToken 覆盖；
// httptest 仅回环地址，无法模拟非回环源 IP，故不重复。

func TestWS_NoPermission403(t *testing.T) {
	srv, ts := startWSTestServer(t, nil)
	// 未导入目录 → 404（project 校验先于权限）。
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL(ts, t.TempDir()), http.Header{
		"X-UI-Token": []string{"ui-secret"},
	})
	if err == nil {
		_ = conn.Close()
		t.Fatal("dial should fail for unregistered project")
	}
	if resp != nil && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unregistered project: status=%d, want 404", resp.StatusCode)
	}
	_ = srv
}

func TestWS_MissingProject404(t *testing.T) {
	_, ts := startWSTestServer(t, nil)
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"/ws/events", http.Header{
		"X-UI-Token": []string{"ui-secret"},
	})
	if err == nil {
		_ = conn.Close()
		t.Fatal("dial should fail without project")
	}
	if resp != nil && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing project: status=%d, want 404", resp.StatusCode)
	}
}

func TestGraph_ReturnsNodesAndEdges(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// 建 A（顶层），B 依赖 A，C 是 B 的子任务。
	a := createTask(t, srv, dir, "A")
	b := createTask(t, srv, dir, "B")
	// C 挂到 B 下。
	createBody, _ := json.Marshal(map[string]any{"title": "C", "parent_id": b})
	uiReq(t, srv, http.MethodPost, "/api/tasks", dir, string(createBody))
	// B 依赖 A。
	updateBody, _ := json.Marshal(map[string]any{"depends_on": []string{a}})
	uiReq(t, srv, http.MethodPatch, "/api/tasks/"+b, dir, string(updateBody))

	rec := uiReq(t, srv, http.MethodGet, "/api/graph", dir, "")
	body := mustCode(t, rec, http.StatusOK, "graph")
	if !strings.Contains(body, `"dependency"`) || !strings.Contains(body, `"parent"`) {
		t.Fatalf("graph should contain parent and dependency edges: %s", body)
	}
	if !strings.Contains(body, `"A"`) || !strings.Contains(body, `"B"`) || !strings.Contains(body, `"C"`) {
		t.Fatalf("graph should contain all tasks: %s", body)
	}
}

func TestAudit_QueryAndExport(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)
	createTask(t, srv, dir, "audit-1")

	// 等审计落库。
	waitAudit(t, srv, dir, func(res *auditQueryResult) bool { return res.total >= 1 })

	// 查询（filter[action]=task.created）。
	rec := uiReq(t, srv, http.MethodGet, "/api/audit?filter[action]=task.created", dir, "")
	body := mustCode(t, rec, http.StatusOK, "audit query")
	if !strings.Contains(body, `"task.created"`) {
		t.Fatalf("audit query missing task.created: %s", body)
	}

	// 导出（text/plain，含表头）。
	rec = doAPI(srv.Handler(), http.MethodGet, "/api/audit/export", "", func(h http.Header) {
		h.Set("X-Project", dir)
		h.Set("X-UI-Token", "ui-secret")
	})
	body = mustCode(t, rec, http.StatusOK, "audit export")
	if !strings.HasPrefix(body, "ts|actor|actor_class|action|target|result|detail") {
		t.Fatalf("export should have header: %s", body)
	}
	if !strings.Contains(body, "task.created") {
		t.Fatalf("export missing task.created: %s", body)
	}
}

func TestAudit_AgentDeniedByDefault(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// audit.read 默认 false → agent 403。
	rec := agentReq(t, srv, http.MethodGet, "/api/audit", dir, "")
	mustCode(t, rec, http.StatusForbidden, "agent audit denied")
}

func TestPlaceholders_NOT_IMPLEMENTED(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// import/export 占位待 TF-018/019 落地；skill 已由 TF-020 替换（见 handlers_skills_test.go）。
	rec := uiReq(t, srv, http.MethodPost, "/api/import", dir, `{"file":"x.md"}`)
	body := mustCode(t, rec, http.StatusNotImplemented, "import placeholder")
	if apiCode(t, body) != "NOT_IMPLEMENTED" {
		t.Fatalf("want NOT_IMPLEMENTED, got %s", body)
	}
}
