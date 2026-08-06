package auth

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"tangoforge/internal/config"
	"tangoforge/internal/db"
	"tangoforge/internal/project"
)

// newStore 构造权限存储（日志丢弃）。
func newStore() *PermissionStore {
	return NewPermissionStore(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// initProject 在临时目录初始化一个项目（meta.db + 默认权限，复用 project.Service）。
func initProject(t *testing.T) (string, *sql.DB) {
	t.Helper()
	registry, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	if err := db.Migrate(context.Background(), registry, db.GlobalMigrations); err != nil {
		t.Fatalf("migrate registry: %v", err)
	}
	svc := project.NewService(registry, slog.New(slog.NewTextHandler(io.Discard, nil)))
	dir := t.TempDir()
	if _, err := svc.Import(context.Background(), dir); err != nil {
		t.Fatalf("import project: %v", err)
	}
	return dir, registry
}

func TestPermissionStore_DefaultGranted(t *testing.T) {
	dir, _ := initProject(t)
	store := newStore()
	got, err := store.Get(t.Context(), dir)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// 全量 16 项。
	if len(got) != len(project.AllActions) {
		t.Fatalf("got %d actions, want %d", len(got), len(project.AllActions))
	}
	// 默认只读 5 项 true（REQUIREMENTS.md §7.1）。
	for _, action := range []string{"task.read", "graph.read", "skill.read", "project.read", "permission.read"} {
		if !got[action] {
			t.Errorf("action %s should be granted by default", action)
		}
	}
	for _, action := range []string{"task.create", "task.update", "state_machine.write", "audit.read"} {
		if got[action] {
			t.Errorf("action %s should be denied by default", action)
		}
	}
}

func TestPermissionStore_Allowed(t *testing.T) {
	dir, _ := initProject(t)
	store := newStore()
	ctx := t.Context()

	ok, err := store.Allowed(ctx, dir, "task.read")
	if err != nil || !ok {
		t.Fatalf("task.read should be allowed, got %v err %v", ok, err)
	}
	ok, err = store.Allowed(ctx, dir, "task.create")
	if err != nil || ok {
		t.Fatalf("task.create should be denied by default, got %v err %v", ok, err)
	}
	// 未知 action（不在 v1 全集）→ 行不存在 → false。
	ok, err = store.Allowed(ctx, dir, "task.hack")
	if err != nil || ok {
		t.Fatalf("unknown action should be denied, got %v err %v", ok, err)
	}
}

func TestPermissionStore_SetFullOverwrite(t *testing.T) {
	dir, _ := initProject(t)
	store := newStore()
	ctx := t.Context()

	// 全量覆盖：只提 task.create=true，其余未提交项应重置 false。
	got, err := store.Set(ctx, dir, map[string]bool{"task.create": true})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !got["task.create"] {
		t.Error("task.create should be true after set")
	}
	if got["task.read"] {
		t.Error("task.read should be reset to false (not submitted)")
	}
	// 持久化验证。
	ok, err := store.Allowed(ctx, dir, "task.read")
	if err != nil || ok {
		t.Fatalf("task.read should be false after overwrite, got %v err %v", ok, err)
	}
	ok, _ = store.Allowed(ctx, dir, "task.create")
	if !ok {
		t.Fatal("task.create should be true after overwrite")
	}
}

func TestPermissionStore_SetInvalidAction(t *testing.T) {
	dir, _ := initProject(t)
	store := newStore()
	_, err := store.Set(t.Context(), dir, map[string]bool{"task.hack": true})
	if err == nil {
		t.Fatal("unknown action must be rejected")
	}
}

func TestPermissionStore_ProjectNotFound(t *testing.T) {
	store := newStore()
	missing := t.TempDir() // 未初始化 .taskboard/
	_, err := store.Get(t.Context(), missing)
	if err == nil {
		t.Fatal("unregistered dir must fail")
	}
	if !errorsIsProjectNotFound(err) {
		t.Fatalf("want project not found, got %v", err)
	}
}

func TestPermissionStore_RelativePathRejected(t *testing.T) {
	store := newStore()
	_, err := store.Get(t.Context(), "relative/dir")
	if err == nil {
		t.Fatal("relative path must be rejected")
	}
}

// --- 中间件测试 ---

func newHandler(t *testing.T) (http.Handler, *int) {
	t.Helper()
	hit := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit++
		w.WriteHeader(http.StatusOK)
	})
	return h, &hit
}

func doReq(h http.Handler, method, target, remoteAddr string, setHeader func(http.Header)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	r.RemoteAddr = remoteAddr
	if setHeader != nil {
		setHeader(r.Header)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// chain 组装中间件链（顺序与 api 层一致）：workdir 先写入 → 来源识别 → 权限判断 → handler。
func chain(cfg *config.GlobalConfig, store *PermissionStore, action, workdir string, h http.Handler) http.Handler {
	return WithWorkdirHandler(workdir, IdentifyMiddleware(cfg)(store.RequirePermission(action)(h)))
}

func TestMiddleware_UIAllowedWithoutTable(t *testing.T) {
	// UI：即使 action 默认 denied，ui 也不查表直接放行。
	dir, _ := initProject(t)
	cfg := config.DefaultGlobalConfig()
	cfg.UIToken = "ui-secret"
	store := newStore()
	// 用无权限动作 task.create 验证 ui 豁免。
	target, hit := newHandler(t)
	h := chain(&cfg, store, "task.create", dir, target)

	rec := doReq(h, http.MethodGet, "/api/tasks", "127.0.0.1:5555", func(hd http.Header) {
		hd.Set("X-UI-Token", "ui-secret")
	})
	if rec.Code != http.StatusOK || *hit != 1 {
		t.Fatalf("ui should pass without permission table, code=%d hit=%d", rec.Code, *hit)
	}
}

func TestMiddleware_AgentAllowed(t *testing.T) {
	dir, _ := initProject(t)
	cfg := config.DefaultGlobalConfig()
	cfg.UIToken = "ui-secret"
	store := newStore()
	target, hit := newHandler(t)
	h := chain(&cfg, store, "task.read", dir, target)

	rec := doReq(h, http.MethodGet, "/api/tasks", "127.0.0.1:5555", func(hd http.Header) {
		hd.Set("X-Actor", "human")
	})
	if rec.Code != http.StatusOK || *hit != 1 {
		t.Fatalf("agent with granted action should pass, code=%d hit=%d", rec.Code, *hit)
	}
}

func TestMiddleware_AgentDenied(t *testing.T) {
	dir, _ := initProject(t)
	cfg := config.DefaultGlobalConfig()
	cfg.UIToken = "ui-secret"
	store := newStore()
	denied := 0
	store.OnDenied = func(_ context.Context, workdir, action string) {
		denied++
		if workdir != dir || action != "task.create" {
			t.Errorf("denied callback got workdir=%q action=%q", workdir, action)
		}
	}
	target, hit := newHandler(t)
	h := chain(&cfg, store, "task.create", dir, target)

	rec := doReq(h, http.MethodGet, "/api/tasks", "127.0.0.1:5555", func(hd http.Header) {
		hd.Set("X-Actor", "human")
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("agent with denied action should get 403, got %d", rec.Code)
	}
	if *hit != 0 {
		t.Error("handler must not be reached")
	}
	if denied != 1 {
		t.Errorf("denied callback should fire once, got %d", denied)
	}
	if !contains(rec.Body.String(), "PERMISSION_DENIED") {
		t.Errorf("body should contain PERMISSION_DENIED: %s", rec.Body.String())
	}
}

func TestMiddleware_UnknownQueriesTable(t *testing.T) {
	dir, _ := initProject(t)
	cfg := config.DefaultGlobalConfig()
	cfg.UIToken = "ui-secret"
	store := newStore()
	target, hit := newHandler(t)
	h := chain(&cfg, store, "task.read", dir, target)

	// 无凭据回环 → unknown → 查表（task.read 默认 true → 放行）。
	rec := doReq(h, http.MethodGet, "/api/tasks", "127.0.0.1:5555", nil)
	if rec.Code != http.StatusOK || *hit != 1 {
		t.Fatalf("unknown with granted action should pass, code=%d hit=%d", rec.Code, *hit)
	}
}

func TestMiddleware_RemoteNoBearer401(t *testing.T) {
	dir, _ := initProject(t)
	cfg := config.DefaultGlobalConfig()
	cfg.UIToken = "ui-secret"
	cfg.APIToken = "api-secret"
	store := newStore()
	target, hit := newHandler(t)
	h := chain(&cfg, store, "task.read", dir, target)

	rec := doReq(h, http.MethodGet, "/api/tasks", "192.168.1.5:1234", nil)
	if rec.Code != http.StatusUnauthorized || *hit != 0 {
		t.Fatalf("remote without bearer should get 401, code=%d hit=%d", rec.Code, *hit)
	}
}

func TestMiddleware_RemoteWithBearer(t *testing.T) {
	dir, _ := initProject(t)
	cfg := config.DefaultGlobalConfig()
	cfg.UIToken = "ui-secret"
	cfg.APIToken = "api-secret"
	store := newStore()
	target, hit := newHandler(t)
	h := chain(&cfg, store, "task.read", dir, target)

	rec := doReq(h, http.MethodGet, "/api/tasks", "192.168.1.5:1234", func(hd http.Header) {
		hd.Set("Authorization", "Bearer api-secret")
	})
	if rec.Code != http.StatusOK || *hit != 1 {
		t.Fatalf("remote with valid bearer should pass, code=%d hit=%d", rec.Code, *hit)
	}
}

func TestMiddleware_MissingWorkdir(t *testing.T) {
	// workdir 未写入 ctx（projectMiddleware 未执行）→ Allowed 报项目不存在 → 500。
	cfg := config.DefaultGlobalConfig()
	store := newStore()
	target, _ := newHandler(t)
	h := chain(&cfg, store, "task.read", "", target)

	rec := doReq(h, http.MethodGet, "/api/tasks", "127.0.0.1:5555", func(hd http.Header) {
		hd.Set("X-Actor", "human")
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("missing workdir should yield 500, got %d", rec.Code)
	}
}

// WithWorkdirHandler 包装 handler：先写入 workdir ctx（模拟 api 层 projectMiddleware）。
func WithWorkdirHandler(workdir string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithWorkdir(r.Context(), workdir)))
	})
}

// contains 简单子串判断（测试辅助）。
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// errorsIsProjectNotFound 判断错误是否包装了 ErrProjectNotFound。
func errorsIsProjectNotFound(err error) bool {
	return errors.Is(err, ErrProjectNotFound)
}
