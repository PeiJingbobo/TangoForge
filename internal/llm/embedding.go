package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"tangoforge/internal/config"
	"time"
)

// Embedding 相关业务错误（docs/KNOWLEDGE-BASE.md §11）。
var (
	// ErrEmbeddingNotConfigured embedding 未配置（model 为空；QA-K5/K23）。
	ErrEmbeddingNotConfigured = errors.New("llm: embedding 未配置（llm.embedding.model 为空）")
	// ErrEmbeddingFailed embedding 调用/解析失败。
	ErrEmbeddingFailed = errors.New("llm: embedding 调用失败")
)

// EmbeddingKind 向量嵌入协议类型。
type EmbeddingKind string

const (
	// EmbedOpenAI OpenAI 兼容 POST {base}/embeddings。
	EmbedOpenAI EmbeddingKind = "openai"
	// EmbedOllama Ollama POST {base}/api/embed。
	EmbedOllama EmbeddingKind = "ollama"
)

// EmbeddingConfig 向量嵌入运行配置（由 config.EmbeddingConfig 转换）。
type EmbeddingConfig struct {
	BaseURL  string
	APIKey   string
	Model    string
	Kind     EmbeddingKind
	Timeout  time.Duration
	MaxToken int
}

// EmbeddingConfigured 判断全局配置中 embedding 是否可用（QA-K23：model 为空 = 未配置）。
func EmbeddingConfigured(cfg config.LLMConfig) bool {
	return strings.TrimSpace(cfg.Embedding.Model) != ""
}

// EmbeddingFromConfig 由全局 LLM 配置构造 embedding 运行配置。
// base_url 空 → 复用 llm.base_url；api_key 空 → 复用 llm.api_key（调用方再回退环境变量）。
func EmbeddingFromConfig(cfg config.LLMConfig) EmbeddingConfig {
	ec := cfg.Embedding
	base := strings.TrimRight(ec.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(cfg.BaseURL, "/")
	}
	key := ec.APIKey
	if key == "" {
		key = cfg.APIKey
	}
	kind := EmbeddingKind(ec.APIKind)
	switch kind {
	case EmbedOpenAI, EmbedOllama:
	default:
		kind = EmbedOpenAI
	}
	timeout := time.Duration(ec.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return EmbeddingConfig{
		BaseURL:  base,
		APIKey:   key,
		Model:    ec.Model,
		Kind:     kind,
		Timeout:  timeout,
		MaxToken: ec.MaxTokens,
	}
}

// Embedding 对单个文本输入生成向量（多协议：openai / ollama，QA-K5）。
//
// 约束：model 未配置 → ErrEmbeddingNotConfigured；base_url 缺失 → 同错。
// 调用失败（非 2xx / 解析失败 / 网络错误）→ ErrEmbeddingFailed。
// 纯 Go HTTP 实现，CGO_ENABLED=0 可编译（零信任依赖铁律）。
func Embedding(ctx context.Context, cfg EmbeddingConfig, input string) ([]float32, error) {
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, ErrEmbeddingNotConfigured
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, ErrEmbeddingNotConfigured
	}
	hc := &http.Client{Timeout: cfg.Timeout}
	var body []byte
	var path string
	var headers map[string]string
	var err error

	switch cfg.Kind {
	case EmbedOllama:
		// POST {base}/api/embed → {"model":..., "input": "..."} → {"embeddings": [[f32...]]}
		payload := map[string]any{"model": cfg.Model, "input": input}
		body, err = json.Marshal(payload)
		path = "/api/embed"
		headers = map[string]string{"Content-Type": "application/json"}
	default: // openai
		// POST {base}/embeddings → {"model":..., "input": "..."} → {"data":[{"embedding":[...]}]}
		payload := map[string]any{"model": cfg.Model, "input": input}
		if cfg.MaxToken > 0 {
			payload["max_tokens"] = cfg.MaxToken
		}
		body, err = json.Marshal(payload)
		path = "/embeddings"
		headers = map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + cfg.APIKey,
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%w: 构造请求: %v", ErrEmbeddingFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: 构造请求: %v", ErrEmbeddingFailed, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbeddingFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4MB 上限
	if err != nil {
		return nil, fmt.Errorf("%w: 读取响应: %v", ErrEmbeddingFailed, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrEmbeddingFailed, resp.StatusCode, truncate(string(data), 200))
	}
	return parseEmbeddingResponse(data, cfg.Kind)
}

// parseEmbeddingResponse 解析双协议响应。
func parseEmbeddingResponse(data []byte, kind EmbeddingKind) ([]float32, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("%w: 响应非 JSON: %v", ErrEmbeddingFailed, err)
	}
	switch kind {
	case EmbedOllama:
		// {"embeddings": [[...]]}（单个 input 时为一维数组的元素）。
		var embeddings [][]float32
		if err := json.Unmarshal(obj["embeddings"], &embeddings); err != nil {
			return nil, fmt.Errorf("%w: 解析 embeddings: %v", ErrEmbeddingFailed, err)
		}
		if len(embeddings) == 0 {
			return nil, fmt.Errorf("%w: embeddings 为空", ErrEmbeddingFailed)
		}
		return embeddings[0], nil
	default: // openai
		// {"data":[{"embedding":[...]}]}
		var dataArr []struct {
			Embedding []float32 `json:"embedding"`
		}
		if err := json.Unmarshal(obj["data"], &dataArr); err != nil {
			return nil, fmt.Errorf("%w: 解析 data: %v", ErrEmbeddingFailed, err)
		}
		if len(dataArr) == 0 {
			return nil, fmt.Errorf("%w: data 为空", ErrEmbeddingFailed)
		}
		return dataArr[0].Embedding, nil
	}
}

// EmbeddingErrorCode 将 embedding 错误映射为业务码（供传输层）。
func EmbeddingErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrEmbeddingNotConfigured):
		return "EMBEDDING_NOT_CONFIGURED"
	case errors.Is(err, ErrEmbeddingFailed):
		return "EMBEDDING_FAILED"
	}
	return ""
}
