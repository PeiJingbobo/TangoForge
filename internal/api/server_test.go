package api

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"tangoforge/internal/config"
	"tangoforge/internal/db"
)

// newTestServer 构造测试 Server（注册表用内存库，日志丢弃）。
func newTestServer(t *testing.T, cfg *config.GlobalConfig, registry *sql.DB) *Server {
	t.Helper()
	if cfg == nil {
		c := config.DefaultGlobalConfig()
		cfg = &c
	}
	if registry == nil {
		registry = openMemRegistry(t)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewServer(cfg, registry, logger)
}

// openMemRegistry 打开并迁移内存全局注册表库。
func openMemRegistry(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open memory registry: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(context.Background(), conn, db.GlobalMigrations); err != nil {
		t.Fatalf("migrate registry: %v", err)
	}
	return conn
}

// registerProject 向注册表插入一条项目记录。
func registerProject(t *testing.T, reg *sql.DB, workdir string) {
	t.Helper()
	_, err := reg.Exec(`INSERT INTO projects (name, workdir, created_at) VALUES (?, ?, ?)`,
		"test-project", workdir, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("register project: %v", err)
	}
}

// doRequest 执行请求并返回响应记录器。
func doRequest(t *testing.T, h http.Handler, method, target string, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPing(t *testing.T) {
	srv := newTestServer(t, nil, nil)
	rec := doRequest(t, srv.Handler(), http.MethodGet, "/ping", "127.0.0.1:5555")
	if rec.Code != http.StatusOK {
		t.Fatalf("ping status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code":0`) {
		t.Errorf("ping body = %s", rec.Body.String())
	}
}

func TestRemoteAccess_NonLoopbackDenied(t *testing.T) {
	cfg := config.DefaultGlobalConfig() // RemoteAccess=false
	srv := newTestServer(t, &cfg, nil)
	rec := doRequest(t, srv.Handler(), http.MethodGet, "/api/tasks", "192.168.1.5:1234")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "REMOTE_ACCESS_DISABLED") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestRemoteAccess_LoopbackAllowed(t *testing.T) {
	cfg := config.DefaultGlobalConfig()
	srv := newTestServer(t, &cfg, nil)
	// 回环来源通过 403 层，进入项目校验（无 X-Project → PROJECT_NOT_FOUND）。
	rec := doRequest(t, srv.Handler(), http.MethodGet, "/api/tasks", "127.0.0.1:5555")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "PROJECT_NOT_FOUND") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestProject_MissingIdentifier(t *testing.T) {
	srv := newTestServer(t, nil, nil)
	rec := doRequest(t, srv.Handler(), http.MethodGet, "/api/tasks", "127.0.0.1:5555")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "PROJECT_NOT_FOUND") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestProject_Unregistered(t *testing.T) {
	srv := newTestServer(t, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("X-Project", `C:\not\registered`)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "PROJECT_NOT_FOUND") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestProject_RegisteredPasses(t *testing.T) {
	reg := openMemRegistry(t)
	cfg := config.DefaultGlobalConfig()
	cfg.UIToken = "ui-secret"
	// 真实临时目录 + 初始化项目库（handler 可正常工作）。
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".taskboard"), 0o755); err != nil {
		t.Fatalf("mkdir .taskboard: %v", err)
	}
	if _, err := db.EnsureProject(context.Background(), db.MetaDBPath(workdir)); err != nil {
		t.Fatalf("ensure project db: %v", err)
	}
	registerProject(t, reg, workdir)

	srv := newTestServer(t, &cfg, reg)
	defer func() { _ = srv.Close() }()
	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("X-Project", workdir)
	req.Header.Set("X-UI-Token", "ui-secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	// TF-013 起 /api/tasks 已定义：注册通过中间件 + UI 放行 → 200 空列表。
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (registered project passes middleware)", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "PROJECT_NOT_FOUND") {
		t.Errorf("should pass project check, body = %s", rec.Body.String())
	}
}

func TestProject_QueryParam(t *testing.T) {
	reg := openMemRegistry(t)
	cfg := config.DefaultGlobalConfig()
	cfg.UIToken = "ui-secret"
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".taskboard"), 0o755); err != nil {
		t.Fatalf("mkdir .taskboard: %v", err)
	}
	if _, err := db.EnsureProject(context.Background(), db.MetaDBPath(workdir)); err != nil {
		t.Fatalf("ensure project db: %v", err)
	}
	registerProject(t, reg, workdir)

	srv := newTestServer(t, &cfg, reg)
	defer func() { _ = srv.Close() }()
	req := httptest.NewRequest(http.MethodGet, "/api/tasks?project="+workdir, nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("X-UI-Token", "ui-secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusNotFound && strings.Contains(rec.Body.String(), "PROJECT_NOT_FOUND") {
		t.Fatalf("query param should pass project check, body = %s", rec.Body.String())
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (query param passes middleware)", rec.Code)
	}
}

func TestRemoteAccess_EnabledAllowsNonLoopback(t *testing.T) {
	cfg := config.DefaultGlobalConfig()
	cfg.RemoteAccess = true
	srv := newTestServer(t, &cfg, nil)
	// 非回环 + remote_access=true → 通过 403 层，进入项目校验。
	rec := doRequest(t, srv.Handler(), http.MethodGet, "/api/tasks", "192.168.1.5:1234")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (project check)", rec.Code)
	}
}

// TestServeAndReloadPort 验证真实监听 + 端口热重载（QA Q8 完整热重载）。
func TestServeAndReloadPort(t *testing.T) {
	cfg := config.DefaultGlobalConfig()
	cfg.Port = 0 // 随机端口
	srv := newTestServer(t, &cfg, nil)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	// 等待 Serve 就绪并取得实际端口 p1。
	var p1 int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		srv.lnMu.Lock()
		ln := srv.listener
		srv.lnMu.Unlock()
		if ln != nil {
			p1 = ln.Addr().(*net.TCPAddr).Port
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if p1 == 0 {
		t.Fatal("server did not start listening within 5s")
	}
	waitPing(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(p1)))

	// 获取一个可用随机端口 p2，热重载到 p2。
	p2 := freePort(t)
	if err := srv.ReloadPort(p2); err != nil {
		t.Fatalf("reload port: %v", err)
	}
	waitPing(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(p2)))

	// 选择 errCh 里 Serve 的返回（Shutdown 后应为 nil）。
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("serve returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
	}
}

// waitPing 轮询等待指定地址 /ping 返回 200。
func waitPing(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr + "/ping")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("ping %s not ok within 5s", addr)
}

// freePort 获取一个当前可用的随机端口（测试用，存在竞态但可接受）。
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}
