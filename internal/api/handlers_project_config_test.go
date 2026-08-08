package api

import (
	"context"
	"net/http"
	"os"
	"strings"
	"tangoforge/internal/audit"
	"tangoforge/internal/config"
	"testing"
	"time"
)

// projectConfigGet 以 UI 身份 GET /api/project-config 并返回 data 原始 JSON。
func projectConfigGet(t *testing.T, srv *Server, project string) string {
	t.Helper()
	rec := uiReq(t, srv, http.MethodGet, "/api/project-config", project, "")
	return mustCode(t, rec, http.StatusOK, "GET /api/project-config")
}

func TestProjectConfigGet_DefaultsAfterImport(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	body := projectConfigGet(t, srv, dir)
	// 默认三态 + export 空。
	if !strings.Contains(body, `"StateMachine"`) || !strings.Contains(body, `"todo"`) {
		t.Errorf("missing state_machine: %s", body)
	}
	if !strings.Contains(body, `"TemplatePath":""`) {
		t.Errorf("export.template_path should default empty: %s", body)
	}
}

func TestProjectConfigGet_AgentDeniedByDefault(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// agent 默认无 state_machine.read → 403。
	rec := agentReq(t, srv, http.MethodGet, "/api/project-config", dir, "")
	mustCode(t, rec, http.StatusForbidden, "agent GET project-config")
	if !strings.Contains(rec.Body.String(), "PERMISSION_DENIED") {
		t.Errorf("body=%s", rec.Body.String())
	}
}

func TestProjectConfigPut_NonUIForbidden(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	body := `{"StateMachine":{"States":[{"Key":"todo","Label":"待办","Color":"#999"}],"Transitions":[]},"Export":{"TemplatePath":""}}`
	rec := agentReq(t, srv, http.MethodPut, "/api/project-config", dir, body)
	mustCode(t, rec, http.StatusForbidden, "agent PUT project-config")
	if !strings.Contains(rec.Body.String(), "PERMISSION_DENIED") {
		t.Errorf("body=%s", rec.Body.String())
	}
}

func TestProjectConfigPut_SuccessPersist(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	body := `{"StateMachine":{"States":[
		{"Key":"backlog","Label":"待排期","Color":"#666"},
		{"Key":"done","Label":"完成","Color":"#0a0"}
	],"Transitions":[{"From":"backlog","To":["done"]}]},"Export":{"TemplatePath":"custom.tmpl"}}`
	rec := uiReq(t, srv, http.MethodPut, "/api/project-config", dir, body)
	out := mustCode(t, rec, http.StatusOK, "UI PUT project-config")
	if !strings.Contains(out, `"backlog"`) || !strings.Contains(out, `"custom.tmpl"`) {
		t.Errorf("响应未含更新值: %s", out)
	}

	// 落盘验证（config.LoadProject）。
	cfg, err := config.LoadProject(dir)
	if err != nil {
		t.Fatalf("load disk: %v", err)
	}
	if len(cfg.StateMachine.States) != 2 || cfg.StateMachine.States[0].Key != "backlog" {
		t.Errorf("states = %+v", cfg.StateMachine.States)
	}
	if cfg.Export.TemplatePath != "custom.tmpl" {
		t.Errorf("template_path = %q", cfg.Export.TemplatePath)
	}

	// 审计记录存在（异步写入 → 轮询等待，避免慢环境时序竞争）。
	deadline := time.Now().Add(3 * time.Second)
	for {
		recs, err := srv.audit.Query(context.Background(), dir, audit.Filter{Action: "project_config.updated"})
		if err != nil {
			t.Fatalf("audit query: %v", err)
		}
		if len(recs.Items) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Error("缺少 project_config.updated 审计记录")
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestProjectConfigPut_InvalidTransition(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// to 引用不存在的状态 → 编辑校验失败 400 TASK_INVALID，不落盘。
	body := `{"StateMachine":{"States":[{"Key":"todo","Label":"待办","Color":"#999"}],
		"Transitions":[{"From":"todo","To":["nope"]}]},"Export":{}}`
	rec := uiReq(t, srv, http.MethodPut, "/api/project-config", dir, body)
	out := mustCode(t, rec, http.StatusBadRequest, "invalid transition")
	if !strings.Contains(out, "TASK_INVALID") {
		t.Errorf("body=%s", out)
	}
	cfg, err := config.LoadProject(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.StateMachine.States) != 3 {
		t.Errorf("非法请求不应落盘: %+v", cfg.StateMachine)
	}
}

func TestProjectConfigPut_StatusInUse(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// 建一个 todo 任务 → 删除 todo 状态 → 422 STATUS_IN_USE。
	createTask(t, srv, dir, "占位任务")
	body := `{"StateMachine":{"States":[{"Key":"done","Label":"完成","Color":"#0a0"}],
		"Transitions":[]},"Export":{}}`
	rec := uiReq(t, srv, http.MethodPut, "/api/project-config", dir, body)
	out := mustCode(t, rec, http.StatusUnprocessableEntity, "status in use")
	if !strings.Contains(out, "STATUS_IN_USE") {
		t.Errorf("body=%s", out)
	}
}

func TestProjectConfigPut_PreservesUnknownSections(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// 预写未知节到 config.yaml（默认配置 + 追加未知节，模拟未来扩展）。
	raw0, err := os.ReadFile(config.ProjectConfigPath(dir))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	extra := "future_feature:\n  enabled: true\n"
	if err := os.WriteFile(config.ProjectConfigPath(dir), []byte(string(raw0)+extra), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// PUT 更新状态机 + export → 未知节保留。
	body := `{"StateMachine":{"States":[{"Key":"todo","Label":"待办","Color":"#9aa0a6"}],
		"Transitions":[]},"Export":{"TemplatePath":"x.tmpl"}}`
	rec := uiReq(t, srv, http.MethodPut, "/api/project-config", dir, body)
	mustCode(t, rec, http.StatusOK, "UI PUT")

	raw, err := os.ReadFile(config.ProjectConfigPath(dir))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(raw), "future_feature:") {
		t.Errorf("未知节丢失:\n%s", raw)
	}
}
