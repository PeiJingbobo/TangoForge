package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDaemonVersion 版本探测（GET /api/daemon/version，免鉴权，含 version/pid/executable）。
func TestDaemonVersion(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()

	// 本地请求（无 token 也能读版本——无敏感信息）。
	rec := doAPI(srv.Handler(), http.MethodGet, "/api/daemon/version", "", func(h http.Header) {
		h.Set("X-UI-Token", "ui-secret")
	})
	body := mustCode(t, rec, http.StatusOK, "daemon version")
	var resp struct {
		Data struct {
			Version    string `json:"version"`
			PID        int    `json:"pid"`
			Executable string `json:"executable"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Version == "" || resp.Data.PID == 0 {
		t.Fatalf("version 响应异常: %s", body)
	}
	if resp.Data.Executable == "" {
		t.Fatalf("executable 缺失: %s", body)
	}
}

// TestDaemonRestart_Permissions restart 仅 UI：agent（CLI/远程）→ 403。
func TestDaemonRestart_Permissions(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()

	// agent（human）→ 403。
	rec := doAPI(srv.Handler(), http.MethodPost, "/api/daemon/restart", "", func(h http.Header) {
		h.Set("X-Actor", "human")
		h.Set("Content-Type", "application/json")
	})
	out := mustCode(t, rec, http.StatusForbidden, "agent restart")
	if apiCode(t, out) != "PERMISSION_DENIED" {
		t.Fatalf("agent restart 应 403: %s", out)
	}
}

// TestDaemonRestart_AcceptAndCallback UI 请求重启 → 202 + 回调触发（空闲重启意图）。
func TestDaemonRestart_AcceptAndCallback(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()

	// 新二进制路径：用可执行文件（如 /bin/ls）满足存在性校验。
	bin := "/bin/ls"
	if _, err := os.Stat(bin); err != nil {
		// macOS 也有 /bin/ls；找不到则用测试二进制自身。
		bin, _ = os.Executable()
	}

	var gotBin string
	srv.SetRestartCallback(func(binPath string) { gotBin = binPath })

	body, _ := json.Marshal(map[string]string{"bin_path": bin})
	rec := doAPI(srv.Handler(), http.MethodPost, "/api/daemon/restart", string(body), func(h http.Header) {
		h.Set("X-UI-Token", "ui-secret")
		h.Set("Content-Type", "application/json")
	})
	out := mustCode(t, rec, http.StatusAccepted, "UI restart")
	if !strings.Contains(out, `"accepted":true`) {
		t.Fatalf("应返回 accepted: %s", out)
	}
	if gotBin != bin {
		t.Fatalf("回调 bin_path = %q, want %q", gotBin, bin)
	}
}

// TestDaemonRestart_BinPathInvalid bin_path 缺失/不存在 → 400。
func TestDaemonRestart_BinPathInvalid(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()

	// 缺 bin_path。
	rec := doAPI(srv.Handler(), http.MethodPost, "/api/daemon/restart", `{}`, func(h http.Header) {
		h.Set("X-UI-Token", "ui-secret")
		h.Set("Content-Type", "application/json")
	})
	out := mustCode(t, rec, http.StatusBadRequest, "empty bin_path")
	if apiCode(t, out) != "DAEMON_RESTART_INVALID" {
		t.Fatalf("code=%s body=%s", apiCode(t, out), out)
	}

	// 不存在的路径。
	body, _ := json.Marshal(map[string]string{"bin_path": filepath.Join(t.TempDir(), "nope")})
	rec = doAPI(srv.Handler(), http.MethodPost, "/api/daemon/restart", string(body), func(h http.Header) {
		h.Set("X-UI-Token", "ui-secret")
		h.Set("Content-Type", "application/json")
	})
	out = mustCode(t, rec, http.StatusBadRequest, "missing bin_path")
	if apiCode(t, out) != "DAEMON_RESTART_INVALID" {
		t.Fatalf("code=%s body=%s", apiCode(t, out), out)
	}
}

// TestDaemonRestart_RequestRestart 幂等：重复请求不覆盖意图（首次 true，后续 false）。
func TestDaemonRestart_RequestRestart(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	if !srv.RequestRestart("/bin/ls") {
		t.Fatal("首次请求应返回 true")
	}
	if srv.RequestRestart("/bin/ls") {
		t.Fatal("重复请求应返回 false（幂等）")
	}
}

// TestDaemonRestart_NoCallbackSet 未设置回调时 UI 请求仍 202（意图记录，无回调通知）。
func TestDaemonRestart_NoCallbackSet(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	bin, _ := os.Executable()
	body, _ := json.Marshal(map[string]string{"bin_path": bin})
	rec := doAPI(srv.Handler(), http.MethodPost, "/api/daemon/restart", string(body), func(h http.Header) {
		h.Set("X-UI-Token", "ui-secret")
		h.Set("Content-Type", "application/json")
	})
	mustCode(t, rec, http.StatusAccepted, "UI restart no callback")
}
