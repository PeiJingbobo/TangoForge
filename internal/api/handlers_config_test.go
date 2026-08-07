package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tangoforge/internal/config"
)

// uiConfigRequest 构造 UI 身份请求（回环 + X-UI-Token）。
func uiConfigRequest(t *testing.T, method, path string, body io.Reader) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("X-UI-Token", "ui-secret")
	return req
}

// newConfigServer 构造带 configPath 的测试 Server（PUT 校验写盘用例）。
func newConfigServer(t *testing.T, cfg *config.GlobalConfig, configPath string) *Server {
	t.Helper()
	registry := openMemRegistry(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(cfg, registry, logger, configPath, t.TempDir())
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

func decodeConfigResp(t *testing.T, body string) configView {
	t.Helper()
	var resp struct {
		Code int        `json:"code"`
		Data configView `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal config resp: %v", err)
	}
	if resp.Code != 0 {
		t.Fatalf("config resp code = %d, want 0 (body=%s)", resp.Code, body)
	}
	return resp.Data
}

func TestConfigGet_MaskSecrets(t *testing.T) {
	srv := newConfigServer(t, &config.GlobalConfig{
		Port:     19810,
		UIToken:  "ui-secret",
		APIToken: "api-secret-token-xyz",
		LLM: config.LLMConfig{
			BaseURL: "https://api.deepseek.com",
			APIKey:  "sk-long-secret-key-123456",
			Model:   "deepseek-chat",
		},
	}, "")

	req := uiConfigRequest(t, http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/config status=%d body=%s", rec.Code, rec.Body.String())
	}
	view := decodeConfigResp(t, rec.Body.String())
	if !strings.Contains(view.LLM.APIKey, "****") {
		t.Errorf("api_key 未掩码: %q", view.LLM.APIKey)
	}
	if strings.Contains(view.LLM.APIKey, "secret-key") {
		t.Errorf("api_key 暴露完整值: %q", view.LLM.APIKey)
	}
	if !strings.Contains(view.APIToken, "****") {
		t.Errorf("api_token 未掩码: %q", view.APIToken)
	}
}

func TestConfigGet_NonUIForbidden(t *testing.T) {
	srv := newConfigServer(t, &config.GlobalConfig{UIToken: "ui-secret"}, "")
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.RemoteAddr = "127.0.0.1:5555" // 回环但无 token → unknown
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "PERMISSION_DENIED") {
		t.Errorf("body=%s", rec.Body.String())
	}
}

func TestConfigPut_InvalidValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := &config.GlobalConfig{
		Port:    19810,
		UIToken: "ui-secret",
		LLM: config.LLMConfig{
			BaseURL: "https://api.deepseek.com",
			Model:   "deepseek-chat",
		},
	}
	if err := config.SaveGlobal(path, *cfg); err != nil {
		t.Fatalf("save initial: %v", err)
	}
	srv := newConfigServer(t, cfg, path)

	// 非法端口 → 422，不落盘
	req := uiConfigRequest(t, http.MethodPut, "/api/config", strings.NewReader(`{"port":70000}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "CONFIG_INVALID") {
		t.Errorf("body=%s", rec.Body.String())
	}
	onDisk, err := config.LoadGlobal(path)
	if err != nil {
		t.Fatalf("reload disk: %v", err)
	}
	if onDisk.Port != 19810 {
		t.Errorf("非法值被落盘: port=%d", onDisk.Port)
	}
}

func TestConfigPut_SuccessPersistAndHotReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := &config.GlobalConfig{
		Port:    19810,
		UIToken: "ui-secret",
		LLM: config.LLMConfig{
			BaseURL:     "https://api.deepseek.com",
			APIKey:      "sk-old-secret-key-999",
			Model:       "deepseek-chat",
			APIKind:     "openai",
			TimeoutSec:  60,
			Retries:     1,
			MaxTokens:   4096,
			Concurrency: 1,
		},
	}
	if err := config.SaveGlobal(path, *cfg); err != nil {
		t.Fatalf("save initial: %v", err)
	}
	srv := newConfigServer(t, cfg, path)

	// 更新 model + timeout；api_key 空 = 保留原值
	body := `{"llm":{"model":"deepseek-v4-flash","timeout_sec":120}}`
	req := uiConfigRequest(t, http.MethodPut, "/api/config", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 热更新：GET 立即可见
	get := uiConfigRequest(t, http.MethodGet, "/api/config", nil)
	grec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(grec, get)
	view := decodeConfigResp(t, grec.Body.String())
	if view.LLM.Model != "deepseek-v4-flash" || view.LLM.TimeoutSec != 120 {
		t.Errorf("热更新未生效: %+v", view.LLM)
	}

	// 写盘：model 更新 + api_key 保留
	onDisk, err := config.LoadGlobal(path)
	if err != nil {
		t.Fatalf("reload disk: %v", err)
	}
	if onDisk.LLM.Model != "deepseek-v4-flash" {
		t.Errorf("disk model=%q", onDisk.LLM.Model)
	}
	if onDisk.LLM.APIKey != "sk-old-secret-key-999" {
		t.Errorf("api_key 空值未保留原值: %q", onDisk.LLM.APIKey)
	}
}

func TestConfigPut_UpdateAPIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := &config.GlobalConfig{
		Port:    19810,
		UIToken: "ui-secret",
		LLM: config.LLMConfig{
			BaseURL:     "https://api.deepseek.com",
			Model:       "m",
			APIKey:      "sk-old-1",
			Concurrency: 1,
		},
	}
	if err := config.SaveGlobal(path, *cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	srv := newConfigServer(t, cfg, path)

	req := uiConfigRequest(t, http.MethodPut, "/api/config", strings.NewReader(`{"llm":{"api_key":"sk-new-key-777"}}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	onDisk, err := config.LoadGlobal(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if onDisk.LLM.APIKey != "sk-new-key-777" {
		t.Errorf("api_key 更新失败: %q", onDisk.LLM.APIKey)
	}
	_ = os.Remove(path)
}
