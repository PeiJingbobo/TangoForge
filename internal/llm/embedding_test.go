package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tangoforge/internal/config"
)

// mockEmbeddingServer 构造按协议响应的 mock embedding server。
func mockEmbeddingServer(t *testing.T, kind EmbeddingKind) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/embeddings", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "test-embed" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float32{0.1, 0.2, 0.3}}},
		})
	})
	mux.HandleFunc("/api/embed", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "test-embed" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float32{{0.4, 0.5, 0.6}},
		})
	})
	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestEmbedding_OpenAI(t *testing.T) {
	srv := mockEmbeddingServer(t, EmbedOpenAI)
	cfg := EmbeddingConfig{
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Model:   "test-embed",
		Kind:    EmbedOpenAI,
	}
	vec, err := Embedding(context.Background(), cfg, "hello")
	if err != nil {
		t.Fatalf("embedding: %v", err)
	}
	if len(vec) != 3 || vec[0] != 0.1 || vec[1] != 0.2 || vec[2] != 0.3 {
		t.Fatalf("vector = %v", vec)
	}
}

func TestEmbedding_Ollama(t *testing.T) {
	srv := mockEmbeddingServer(t, EmbedOllama)
	cfg := EmbeddingConfig{BaseURL: srv.URL, Model: "test-embed", Kind: EmbedOllama}
	vec, err := Embedding(context.Background(), cfg, "hello")
	if err != nil {
		t.Fatalf("embedding: %v", err)
	}
	if len(vec) != 3 || vec[0] != 0.4 {
		t.Fatalf("vector = %v", vec)
	}
}

func TestEmbedding_NotConfigured(t *testing.T) {
	// model 空 → EMBEDDING_NOT_CONFIGURED。
	_, err := Embedding(context.Background(), EmbeddingConfig{BaseURL: "http://x", Kind: EmbedOpenAI}, "x")
	if !errors.Is(err, ErrEmbeddingNotConfigured) {
		t.Fatalf("model 空应 NOT_CONFIGURED，got %v", err)
	}
	// base_url 空 → NOT_CONFIGURED。
	_, err = Embedding(context.Background(), EmbeddingConfig{Model: "m", Kind: EmbedOpenAI}, "x")
	if !errors.Is(err, ErrEmbeddingNotConfigured) {
		t.Fatalf("base_url 空应 NOT_CONFIGURED，got %v", err)
	}
}

func TestEmbedding_APIError(t *testing.T) {
	srv := mockEmbeddingServer(t, EmbedOpenAI)
	// 错误状态（/error 端点）→ EMBEDDING_FAILED。
	cfg := EmbeddingConfig{BaseURL: srv.URL + "/error", APIKey: "test-key", Model: "test-embed", Kind: EmbedOpenAI}
	_, err := Embedding(context.Background(), cfg, "x")
	if !errors.Is(err, ErrEmbeddingFailed) {
		t.Fatalf("HTTP 500 应 FAILED，got %v", err)
	}
	// 鉴权失败（401）→ FAILED。
	cfg2 := EmbeddingConfig{BaseURL: srv.URL, APIKey: "wrong", Model: "test-embed", Kind: EmbedOpenAI}
	_, err = Embedding(context.Background(), cfg2, "x")
	if !errors.Is(err, ErrEmbeddingFailed) {
		t.Fatalf("401 应 FAILED，got %v", err)
	}
	// 网络错误（关闭的 server）→ FAILED。
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	badURL := bad.URL
	bad.Close()
	cfg3 := EmbeddingConfig{BaseURL: badURL, APIKey: "k", Model: "m", Kind: EmbedOpenAI}
	_, err = Embedding(context.Background(), cfg3, "x")
	if !errors.Is(err, ErrEmbeddingFailed) {
		t.Fatalf("网络错误应 FAILED，got %v", err)
	}
}

func TestEmbeddingFromConfig(t *testing.T) {
	// base_url 复用 llm.base_url；api_key 复用。
	cfg := config.LLMConfig{
		BaseURL: "https://api.example.com/",
		APIKey:  "chat-key",
		Embedding: config.EmbeddingConfig{
			Model:      "emb-model",
			APIKind:    "openai",
			TimeoutSec: 30,
		},
	}
	ec := EmbeddingFromConfig(cfg)
	if ec.BaseURL != "https://api.example.com" || ec.APIKey != "chat-key" || ec.Model != "emb-model" {
		t.Fatalf("from config = %+v", ec)
	}
	if ec.Kind != EmbedOpenAI || ec.Timeout != 30e9 {
		t.Fatalf("from config = %+v", ec)
	}
	// embedding 节覆盖 + ollama。
	cfg2 := config.LLMConfig{
		BaseURL: "https://chat.example.com",
		Embedding: config.EmbeddingConfig{
			BaseURL:   "http://localhost:11434",
			APIKind:   "ollama",
			Model:     "nomic-embed-text",
			MaxTokens: 1024,
		},
	}
	ec2 := EmbeddingFromConfig(cfg2)
	if ec2.BaseURL != "http://localhost:11434" || ec2.Kind != EmbedOllama || ec2.MaxToken != 1024 {
		t.Fatalf("from config 2 = %+v", ec2)
	}
	// 非法 api_kind → 默认 openai。
	cfg3 := config.LLMConfig{Embedding: config.EmbeddingConfig{APIKind: "weird", Model: "m"}}
	ec3 := EmbeddingFromConfig(cfg3)
	if ec3.Kind != EmbedOpenAI {
		t.Fatalf("非法 kind 应回退 openai: %+v", ec3)
	}
	// 默认超时 60s。
	cfg4 := config.LLMConfig{Embedding: config.EmbeddingConfig{Model: "m"}}
	ec4 := EmbeddingFromConfig(cfg4)
	if ec4.Timeout != 60e9 {
		t.Fatalf("默认 timeout = %v", ec4.Timeout)
	}
}

func TestEmbeddingConfigured(t *testing.T) {
	if EmbeddingConfigured(config.LLMConfig{}) {
		t.Error("model 空应视为未配置")
	}
	cfg := config.LLMConfig{Embedding: config.EmbeddingConfig{Model: "  m  "}}
	if !EmbeddingConfigured(cfg) {
		t.Error("model 非空应视为已配置")
	}
}

func TestEmbeddingErrorCode(t *testing.T) {
	if EmbeddingErrorCode(ErrEmbeddingNotConfigured) != "EMBEDDING_NOT_CONFIGURED" {
		t.Error("NOT_CONFIGURED 映射错误")
	}
	if EmbeddingErrorCode(ErrEmbeddingFailed) != "EMBEDDING_FAILED" {
		t.Error("FAILED 映射错误")
	}
	if EmbeddingErrorCode(errors.New("other")) != "" {
		t.Error("其它错误应返回空")
	}
}

func TestParseEmbeddingResponse_Errors(t *testing.T) {
	// 非 JSON。
	if _, err := parseEmbeddingResponse([]byte("not-json"), EmbedOpenAI); !errors.Is(err, ErrEmbeddingFailed) {
		t.Fatalf("非 JSON 应 FAILED，got %v", err)
	}
	// openai 缺 data。
	if _, err := parseEmbeddingResponse([]byte(`{}`), EmbedOpenAI); !errors.Is(err, ErrEmbeddingFailed) {
		t.Fatalf("缺 data 应 FAILED，got %v", err)
	}
	// openai data 空。
	if _, err := parseEmbeddingResponse([]byte(`{"data":[]}`), EmbedOpenAI); !errors.Is(err, ErrEmbeddingFailed) {
		t.Fatalf("空 data 应 FAILED，got %v", err)
	}
	// ollama 缺 embeddings。
	if _, err := parseEmbeddingResponse([]byte(`{}`), EmbedOllama); !errors.Is(err, ErrEmbeddingFailed) {
		t.Fatalf("ollama 缺 embeddings 应 FAILED，got %v", err)
	}
	// ollama 空。
	if _, err := parseEmbeddingResponse([]byte(`{"embeddings":[]}`), EmbedOllama); !errors.Is(err, ErrEmbeddingFailed) {
		t.Fatalf("ollama 空应 FAILED，got %v", err)
	}
}

var _ = strings.TrimSpace
