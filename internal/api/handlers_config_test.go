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
	"tangoforge/internal/config"
	"testing"
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

// TF-041：POST /api/config/test 用暂存 LLM 配置测试连接（成功 → ok）。
func TestConfigTestLLM_OK(t *testing.T) {
	llmSrv := mockLLMResponse(t, "ok")
	srv := newAPIServer(t, func(cfg *config.GlobalConfig) {
		cfg.LLM = config.LLMConfig{BaseURL: llmSrv.URL, Model: "mock", APIKind: "openai", TimeoutSec: 5, Retries: 0}
	})
	defer func() { _ = srv.Close() }()

	body, _ := json.Marshal(map[string]string{
		"base_url": llmSrv.URL, "model": "mock", "api_kind": "openai",
	})
	rec := uiReq(t, srv, http.MethodPost, "/api/config/test", "", string(body))
	out := mustCode(t, rec, http.StatusOK, "config test ok")
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("响应: %s", out)
	}
}

// TF-041：POST /api/config/test 未配置 → 422 LLM_TEST_FAILED。
func TestConfigTestLLM_NotConfigured(t *testing.T) {
	srv := newAPIServer(t, func(cfg *config.GlobalConfig) {
		cfg.LLM = config.LLMConfig{} // 空配置
	})
	defer func() { _ = srv.Close() }()

	rec := uiReq(t, srv, http.MethodPost, "/api/config/test", "", `{"base_url":"","model":""}`)
	out := mustCode(t, rec, http.StatusUnprocessableEntity, "config test not configured")
	if apiCode(t, out) != "LLM_TEST_FAILED" {
		t.Fatalf("code=%s body=%s", apiCode(t, out), out)
	}
}

// TF-052：GET /api/config 返回 llm.embedding + knowledge 节（脱敏 embedding api_key）。
func TestConfigGet_EmbeddingAndKnowledge(t *testing.T) {
	srv := newConfigServer(t, &config.GlobalConfig{
		Port:    19810,
		UIToken: "ui-secret",
		LLM: config.LLMConfig{
			BaseURL: "https://api.deepseek.com",
			Model:   "deepseek-chat",
			Embedding: config.EmbeddingConfig{
				Model: "nomic-embed-text", APIKind: "ollama", TimeoutSec: 30,
			},
		},
		Knowledge: func() config.KnowledgeGlobalConfig {
			k := config.DefaultKnowledgeGlobalConfig()
			k.DebounceMS = 15000
			k.SearchTopK = 5
			return k
		}(),
	}, "")

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, uiConfigRequest(t, http.MethodGet, "/api/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/config status=%d body=%s", rec.Code, rec.Body.String())
	}
	view := decodeConfigResp(t, rec.Body.String())
	if view.LLM.Embedding.Model != "nomic-embed-text" || view.LLM.Embedding.APIKind != "ollama" {
		t.Fatalf("embedding 视图错误: %+v", view.LLM.Embedding)
	}
	if !view.Knowledge.Enabled || !view.Knowledge.FSNotify || !view.Knowledge.VectorSearch {
		t.Fatalf("knowledge 默认开关应为 true: %+v", view.Knowledge)
	}
	if view.Knowledge.DebounceMS != 15000 || view.Knowledge.SearchTopK != 5 {
		t.Fatalf("knowledge 数值错误: %+v", view.Knowledge)
	}
	if view.Knowledge.MaxIndexSize != 524288 {
		t.Fatalf("max_index_size 默认 524288: %+v", view.Knowledge)
	}
}

// TF-052：PUT /api/config 更新 llm.embedding + knowledge 节（部分更新，布尔显式 false 关闭）。
func TestConfigPut_EmbeddingAndKnowledge(t *testing.T) {
	cfg := config.DefaultGlobalConfig()
	cfg.UIToken = "ui-secret"
	cfg.LLM.BaseURL = "https://api.deepseek.com"
	cfg.LLM.Model = "deepseek-chat"
	srv := newConfigServer(t, &cfg, "")
	body, _ := json.Marshal(map[string]any{
		"llm": map[string]any{
			"embedding": map[string]any{
				"model": "qwen3-embedding:4b", "api_kind": "ollama", "timeout_sec": 45,
			},
		},
		"knowledge": map[string]any{
			"enabled": true, "fsnotify": false, "debounce_ms": 12000, "search_top_k": 8,
		},
	})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, uiConfigRequest(t, http.MethodPut, "/api/config", strings.NewReader(string(body))))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/config status=%d body=%s", rec.Code, rec.Body.String())
	}
	view := decodeConfigResp(t, rec.Body.String())
	if view.LLM.Embedding.Model != "qwen3-embedding:4b" || view.LLM.Embedding.APIKind != "ollama" ||
		view.LLM.Embedding.TimeoutSec != 45 {
		t.Fatalf("embedding 更新失败: %+v", view.LLM.Embedding)
	}
	if view.Knowledge.FSNotify {
		t.Fatal("fsnotify 显式 false 应关闭")
	}
	if !view.Knowledge.Enabled || view.Knowledge.DebounceMS != 12000 || view.Knowledge.SearchTopK != 8 {
		t.Fatalf("knowledge 更新失败: %+v", view.Knowledge)
	}
	// 未更新的字段保持默认。
	if view.Knowledge.MaxIndexSize != 524288 {
		t.Fatalf("未更新字段应保持默认: %+v", view.Knowledge)
	}
	// 持久化到文件。
	if srv.configPath != "" {
		saved, err := config.LoadGlobal(srv.configPath)
		if err != nil {
			t.Fatalf("reload saved config: %v", err)
		}
		if saved.LLM.Embedding.Model != "qwen3-embedding:4b" {
			t.Fatalf("落盘 embedding 错误: %+v", saved.LLM.Embedding)
		}
	}
}

// TF-052：PUT /api/config embedding 非法 api_kind → 422 CONFIG_INVALID。
func TestConfigPut_EmbeddingInvalidKind(t *testing.T) {
	dc := config.DefaultGlobalConfig()
	dc.UIToken = "ui-secret"
	srv := newConfigServer(t, &dc, "")
	body, _ := json.Marshal(map[string]any{
		"llm": map[string]any{
			"embedding": map[string]any{"model": "m", "api_kind": "weird"},
		},
	})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, uiConfigRequest(t, http.MethodPut, "/api/config", strings.NewReader(string(body))))
	out := mustCode(t, rec, http.StatusUnprocessableEntity, "invalid embedding kind")
	if apiCode(t, out) != "CONFIG_INVALID" {
		t.Fatalf("code=%s body=%s", apiCode(t, out), out)
	}
}

// TF-052：PUT /api/config knowledge 阈值越界 → 422 CONFIG_INVALID。
func TestConfigPut_KnowledgeThresholdOutOfRange(t *testing.T) {
	dc := config.DefaultGlobalConfig()
	dc.UIToken = "ui-secret"
	srv := newConfigServer(t, &dc, "")
	body, _ := json.Marshal(map[string]any{
		"knowledge": map[string]any{"search_threshold": 1.5},
	})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, uiConfigRequest(t, http.MethodPut, "/api/config", strings.NewReader(string(body))))
	out := mustCode(t, rec, http.StatusUnprocessableEntity, "threshold out of range")
	if apiCode(t, out) != "CONFIG_INVALID" {
		t.Fatalf("code=%s body=%s", apiCode(t, out), out)
	}
}

// TF-053：POST /api/config/test-embedding 测试向量嵌入连接（成功 → ok + dim）。
func TestConfigTestEmbedding_OK(t *testing.T) {
	// mock openai embeddings 端点。
	mux := http.NewServeMux()
	mux.HandleFunc("/embeddings", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := config.DefaultGlobalConfig()
	cfg.UIToken = "ui-secret"
	srv2 := newConfigServer(t, &cfg, "")
	body, _ := json.Marshal(map[string]string{
		"base_url": srv.URL, "model": "test-embed", "api_kind": "openai",
	})
	rec := uiReq(t, srv2, http.MethodPost, "/api/config/test-embedding", "", string(body))
	out := mustCode(t, rec, http.StatusOK, "embedding test ok")
	if !strings.Contains(out, `"ok":true`) || !strings.Contains(out, `"dim":3`) {
		t.Fatalf("响应应含 ok+dim: %s", out)
	}
}

// TF-053：POST /api/config/test-embedding 未配置 → 422 EMBEDDING_TEST_FAILED。
func TestConfigTestEmbedding_NotConfigured(t *testing.T) {
	cfg := config.DefaultGlobalConfig()
	cfg.UIToken = "ui-secret"
	srv := newConfigServer(t, &cfg, "")
	// 空 model + 空 base_url → 未配置。
	rec := uiReq(t, srv, http.MethodPost, "/api/config/test-embedding", "", `{"model":""}`)
	out := mustCode(t, rec, http.StatusUnprocessableEntity, "embedding test not configured")
	if apiCode(t, out) != "EMBEDDING_TEST_FAILED" {
		t.Fatalf("code=%s body=%s", apiCode(t, out), out)
	}
}

// TF-053：POST /api/config/test-embedding agent → 403。
func TestConfigTestEmbedding_AgentDenied(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	rec := doAPI(srv.Handler(), http.MethodPost, "/api/config/test-embedding", `{"model":"m"}`, func(h http.Header) {
		h.Set("X-Actor", "human")
		h.Set("Content-Type", "application/json")
	})
	out := mustCode(t, rec, http.StatusForbidden, "agent embedding test")
	if apiCode(t, out) != "PERMISSION_DENIED" {
		t.Fatalf("code=%s body=%s", apiCode(t, out), out)
	}
}
