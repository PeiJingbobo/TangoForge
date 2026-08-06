// Package llm 提供多协议 LLM 客户端（QA P4-1 扩展：兼容 OpenAI Chat / Anthropic Messages / Responses API）。
//
// 三种协议（由全局配置 llm.api_kind 选择，QA P4-1）：
//   - openai：POST {base}/chat/completions（DeepSeek / Ollama 等 OpenAI 兼容端点）；
//   - anthropic：POST {base}/v1/messages（Anthropic Messages API；DeepSeek 兼容端点如
//     https://api.deepseek.com/anthropic）；
//   - responses：POST {base}/v1/responses（OpenAI Responses API）。
//
// 约束（AGENTS.md §3.1 / REQUIREMENTS.md §四.3）：
//   - 仅 JSON 结构化通信，供 parser / exporter 复用；
//   - 本包为基础设施，禁止引用 api / mcp / cmd；
//   - APIKey 为空时回退读取环境变量 DEEPSEEK_API_KEY（QA P4-1），便于本地模型与测试。
//
// 结构化输出策略（QA P4-1）：
//   - openai：response_format={"type":"json_object"}（DeepSeek 支持）+ prompt 内嵌 Schema；
//   - anthropic / responses：无原生 json 模式，靠 prompt 约束 + 响应后处理提取首个 JSON 块；
//   - 提取失败视为 LLM 输出不合规（ErrInvalidResponse），由调用方整次失败（不补默认值）。
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"tangoforge/internal/config"
)

// APIKind 协议类型（config.LLMConfig.APIKind 取值）。
type APIKind string

const (
	// KindOpenAI OpenAI Chat Completions（默认）。
	KindOpenAI APIKind = "openai"
	// KindAnthropic Anthropic Messages API。
	KindAnthropic APIKind = "anthropic"
	// KindResponses OpenAI Responses API。
	KindResponses APIKind = "responses"
)

// 业务错误（传输层经 errors.Is 判定，映射 HTTP）。
var (
	// ErrNotConfigured LLM 未配置（base_url / model 为空）。
	ErrNotConfigured = errors.New("llm: 未配置（base_url / model 为空）")
	// ErrTimeout 请求超时。
	ErrTimeout = errors.New("llm: 请求超时")
	// ErrAPIStatus LLM 服务返回非 2xx（携带 HTTP 状态码与响应摘要）。
	ErrAPIStatus = errors.New("llm: 服务返回错误状态")
	// ErrInvalidResponse 响应解析失败（非 JSON / 无文本内容 / JSON 提取失败）。
	ErrInvalidResponse = errors.New("llm: 响应格式不合规")
)

// APIStatusError 携带 LLM 服务端状态码的错误（errors.As 可提取）。
type APIStatusError struct {
	StatusCode int
	Body       string
}

func (e *APIStatusError) Error() string {
	return fmt.Sprintf("%s: HTTP %d: %s", ErrAPIStatus, e.StatusCode, truncate(e.Body, 200))
}

func (e *APIStatusError) Unwrap() error { return ErrAPIStatus }

// Config 客户端运行配置（由 config.LLMConfig 转换；APIKey 空时回退环境变量）。
type Config struct {
	BaseURL     string
	APIKey      string
	Model       string
	Kind        APIKind
	Timeout     time.Duration
	Retries     int
	MaxTokens   int
	Concurrency int
}

// FromConfig 由全局配置构造客户端配置（APIKey 空时由 New 统一回退 DEEPSEEK_API_KEY）。
func FromConfig(cfg config.LLMConfig) Config {
	kind := APIKind(cfg.APIKind)
	switch kind {
	case KindOpenAI, KindAnthropic, KindResponses:
	default:
		kind = KindOpenAI
	}
	return Config{
		BaseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		Kind:        kind,
		Timeout:     time.Duration(cfg.TimeoutSec) * time.Second,
		Retries:     cfg.Retries,
		MaxTokens:   cfg.MaxTokens,
		Concurrency: cfg.Concurrency,
	}
}

// Request 一次对话请求（供 parser / exporter 构造）。
type Request struct {
	// System 系统提示（可选）。
	System string
	// User 用户内容（必填）。
	User string
	// MaxTokens 单次响应上限；0 = 使用客户端配置。
	MaxTokens int
	// RequireJSON 结构化输出：openai 走 response_format；其余靠 Schema 约束 + 后处理提取。
	RequireJSON bool
	// Schema JSON Schema 描述文本（RequireJSON 时追加进 prompt）。
	Schema string
}

// Response 统一响应（文本内容）。
type Response struct {
	Text string
}

// Client 多协议 LLM 客户端（并发安全：信号量控制并发度）。
type Client struct {
	cfg    Config
	hc     *http.Client
	sem    chan struct{}
	logger *slog.Logger
}

// New 构造客户端；base_url / model 为空 → ErrNotConfigured；
// APIKey 为空时回退读取环境变量 DEEPSEEK_API_KEY（QA P4-1，便于本地模型与测试）。
func New(cfg Config, logger *slog.Logger) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("%w: base_url=%q model=%q", ErrNotConfigured, cfg.BaseURL, cfg.Model)
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("DEEPSEEK_API_KEY")
	}
	if logger == nil {
		logger = slog.Default()
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	conc := cfg.Concurrency
	if conc <= 0 {
		conc = 1
	}
	return &Client{
		cfg:    cfg,
		hc:     &http.Client{Timeout: timeout},
		sem:    make(chan struct{}, conc),
		logger: logger,
	}, nil
}

// Complete 发送对话请求并返回首个文本内容（重试 Retries+1 次，仅对网络/超时/5xx/429 重试）。
func (c *Client) Complete(ctx context.Context, req Request) (string, error) {
	attempts := c.cfg.Retries + 1
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			// 线性退避（100ms × 已重试次数），避免风暴。
			select {
			case <-time.After(time.Duration(i) * 100 * time.Millisecond):
			case <-ctx.Done():
				return "", fmt.Errorf("%w: %v", ErrTimeout, ctx.Err())
			}
		}
		text, err := c.doOnce(ctx, req)
		if err == nil {
			return text, nil
		}
		lastErr = err
		// 4xx 业务错误不重试（配置错误/鉴权失败等，重试无意义）。
		var apiErr *APIStatusError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && apiErr.StatusCode != 429 {
			break
		}
	}
	return "", lastErr
}

// CompleteJSON 结构化输出：Complete + 提取首个 JSON 块（对象或数组）。
// 提取失败 → ErrInvalidResponse（调用方整次失败，不补默认值）。
func (c *Client) CompleteJSON(ctx context.Context, req Request) (json.RawMessage, error) {
	req.RequireJSON = true
	text, err := c.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	raw, err := extractJSON(text)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}
	return raw, nil
}

// doOnce 单次请求（不过重试/退避）。
func (c *Client) doOnce(ctx context.Context, req Request) (string, error) {
	// 并发控制。
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return "", fmt.Errorf("%w: %v", ErrTimeout, ctx.Err())
	}

	body, path, headers, err := c.buildRequest(req)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm: 构造请求: %w", err)
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.hc.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return "", fmt.Errorf("llm: 请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB 上限
	if err != nil {
		return "", fmt.Errorf("llm: 读取响应: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &APIStatusError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	return c.parseResponse(data)
}

// buildRequest 构造协议相关请求体与头（三协议差异集中于此）。
func (c *Client) buildRequest(req Request) ([]byte, string, map[string]string, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.cfg.MaxTokens
	}
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	// JSON Schema 约束追加进 user 提示（三协议通用）。
	user := req.User
	if req.RequireJSON && strings.TrimSpace(req.Schema) != "" {
		user += "\n\n请严格按照以下 JSON Schema 输出，只输出 JSON 本身，不要输出任何解释或代码围栏：\n" + req.Schema
	}

	switch c.cfg.Kind {
	case KindAnthropic:
		headers["x-api-key"] = c.cfg.APIKey
		headers["anthropic-version"] = "2023-06-01"
		payload := map[string]any{
			"model":      c.cfg.Model,
			"max_tokens": maxTokens,
			"messages":   []map[string]string{{"role": "user", "content": user}},
		}
		if req.System != "" {
			payload["system"] = req.System
		}
		b, err := json.Marshal(payload)
		return b, "/v1/messages", headers, err

	case KindResponses:
		headers["Authorization"] = "Bearer " + c.cfg.APIKey
		payload := map[string]any{
			"model":             c.cfg.Model,
			"max_output_tokens": maxTokens,
			"input":             user,
		}
		if req.System != "" {
			payload["instructions"] = req.System
		}
		b, err := json.Marshal(payload)
		return b, "/v1/responses", headers, err

	default: // openai
		headers["Authorization"] = "Bearer " + c.cfg.APIKey
		messages := make([]map[string]string, 0, 2)
		if req.System != "" {
			messages = append(messages, map[string]string{"role": "system", "content": req.System})
		}
		messages = append(messages, map[string]string{"role": "user", "content": user})
		payload := map[string]any{
			"model":      c.cfg.Model,
			"messages":   messages,
			"max_tokens": maxTokens,
		}
		if req.RequireJSON {
			payload["response_format"] = map[string]string{"type": "json_object"}
		}
		b, err := json.Marshal(payload)
		return b, "/chat/completions", headers, err
	}
}

// parseResponse 提取三协议响应中的文本内容。
func (c *Client) parseResponse(data []byte) (string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return "", fmt.Errorf("%w: 响应非 JSON: %v", ErrInvalidResponse, err)
	}
	switch c.cfg.Kind {
	case KindAnthropic:
		// {"content":[{"type":"text","text":"..."}]}
		var content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(obj["content"], &content); err != nil {
			return "", fmt.Errorf("%w: 解析 content: %v", ErrInvalidResponse, err)
		}
		var b strings.Builder
		for _, seg := range content {
			if seg.Type == "text" && seg.Text != "" {
				b.WriteString(seg.Text)
			}
		}
		if b.Len() == 0 {
			return "", fmt.Errorf("%w: content 无文本", ErrInvalidResponse)
		}
		return b.String(), nil

	case KindResponses:
		// {"output":[{"type":"message","content":[{"type":"output_text","text":"..."}]}]}
		var output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(obj["output"], &output); err != nil {
			return "", fmt.Errorf("%w: 解析 output: %v", ErrInvalidResponse, err)
		}
		var b strings.Builder
		for _, item := range output {
			for _, seg := range item.Content {
				if seg.Text != "" {
					b.WriteString(seg.Text)
				}
			}
		}
		if b.Len() == 0 {
			return "", fmt.Errorf("%w: output 无文本", ErrInvalidResponse)
		}
		return b.String(), nil

	default: // openai
		// {"choices":[{"message":{"content":"..."}}]}
		var choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(obj["choices"], &choices); err != nil {
			return "", fmt.Errorf("%w: 解析 choices: %v", ErrInvalidResponse, err)
		}
		if len(choices) == 0 || strings.TrimSpace(choices[0].Message.Content) == "" {
			return "", fmt.Errorf("%w: choices 无内容", ErrInvalidResponse)
		}
		return choices[0].Message.Content, nil
	}
}

// extractJSON 从文本中提取首个平衡的 JSON 对象/数组（容错代码围栏与前后缀文本）。
func extractJSON(text string) (json.RawMessage, error) {
	start := strings.IndexByte(text, '{')
	end := -1
	if start < 0 {
		// 尝试数组。
		start = strings.IndexByte(text, '[')
		if start < 0 {
			return nil, errors.New("未找到 JSON 起始字符")
		}
		open, close := byte('['), byte(']')
		depth := 0
		inStr := false
		esc := false
		for i := start; i < len(text); i++ {
			ch := text[i]
			if esc {
				esc = false
				continue
			}
			if ch == '\\' && inStr {
				esc = true
				continue
			}
			if ch == '"' {
				inStr = !inStr
				continue
			}
			if inStr {
				continue
			}
			if ch == open {
				depth++
			} else if ch == close {
				depth--
				if depth == 0 {
					end = i + 1
					break
				}
			}
		}
	} else {
		open, close := byte('{'), byte('}')
		depth := 0
		inStr := false
		esc := false
		for i := start; i < len(text); i++ {
			ch := text[i]
			if esc {
				esc = false
				continue
			}
			if ch == '\\' && inStr {
				esc = true
				continue
			}
			if ch == '"' {
				inStr = !inStr
				continue
			}
			if inStr {
				continue
			}
			if ch == open {
				depth++
			} else if ch == close {
				depth--
				if depth == 0 {
					end = i + 1
					break
				}
			}
		}
	}
	if end < 0 {
		return nil, errors.New("JSON 块未闭合")
	}
	raw := json.RawMessage(text[start:end])
	if !json.Valid(raw) {
		return nil, errors.New("提取的 JSON 块非法")
	}
	return raw, nil
}

// truncate 截断字符串（错误信息摘要用）。
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
