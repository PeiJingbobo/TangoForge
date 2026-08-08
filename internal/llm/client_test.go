package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"tangoforge/internal/config"
	"testing"
	"time"
)

// mockServer 可注入响应序列的 LLM mock 服务，并记录请求（供断言协议细节）。
type mockServer struct {
	t          *testing.T
	mu         sync.Mutex
	reqs       []mockReq
	responses  []respSpec // 顺序消费；最后一个循环使用
	idx        int
	concurrent int32
	peak       int32
	kind       APIKind
}

type mockReq struct {
	path    string
	headers http.Header
	body    []byte
}

type respSpec struct {
	status int
	body   string
	delay  time.Duration
}

// newMockServer 构造 mock 服务；responses 为空时按 kind 返回默认成功响应。
func newMockServer(t *testing.T, kind APIKind, responses ...respSpec) *mockServer {
	t.Helper()
	if len(responses) == 0 {
		responses = []respSpec{{status: 200, body: defaultBody(kind)}}
	}
	return &mockServer{t: t, kind: kind, responses: responses}
}

func defaultBody(kind APIKind) string {
	switch kind {
	case KindAnthropic:
		return `{"content":[{"type":"text","text":"你好，Anthropic"}]}`
	case KindResponses:
		return `{"output":[{"type":"message","content":[{"type":"output_text","text":"你好，Responses"}]}]}`
	default:
		return `{"choices":[{"message":{"content":"你好，OpenAI"}}]}`
	}
}

func (m *mockServer) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt32(&m.concurrent, 1)
		for {
			old := atomic.LoadInt32(&m.peak)
			if cur <= old || atomic.CompareAndSwapInt32(&m.peak, old, cur) {
				break
			}
		}
		defer atomic.AddInt32(&m.concurrent, -1)

		body, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		m.reqs = append(m.reqs, mockReq{path: r.URL.Path, headers: r.Header.Clone(), body: body})
		spec := m.responses[m.idx]
		if m.idx < len(m.responses)-1 {
			m.idx++
		}
		m.mu.Unlock()

		if spec.delay > 0 {
			time.Sleep(spec.delay)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(spec.status)
		_, _ = w.Write([]byte(spec.body))
	})
}

func (m *mockServer) requests() []mockReq {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mockReq(nil), m.reqs...)
}

// bodyField 解析请求体 JSON 并返回指定字段（点路径一级）。
func bodyField(t *testing.T, req mockReq, key string) any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(req.body, &obj); err != nil {
		t.Fatalf("请求体非 JSON: %v", err)
	}
	v, ok := obj[key]
	if !ok {
		t.Fatalf("请求体缺少字段 %q: %s", key, string(req.body))
	}
	return v
}

// newTestClient 以 mock 服务 URL 构造客户端。
func newTestClient(t *testing.T, m *mockServer, cfg Config) *Client {
	t.Helper()
	cfg.BaseURL = m.HandlerServerURL(t)
	c, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func (m *mockServer) HandlerServerURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(m.Handler())
	t.Cleanup(srv.Close)
	return srv.URL
}

func baseConfig() Config {
	return Config{
		APIKey:      "sk-test",
		Model:       "deepseek-v4-flash",
		Kind:        KindOpenAI,
		Timeout:     2 * time.Second,
		Retries:     1,
		MaxTokens:   1024,
		Concurrency: 4,
	}
}

// --- 三协议请求构造与解析 ---

func TestOpenAI_Complete(t *testing.T) {
	m := newMockServer(t, KindOpenAI)
	c := newTestClient(t, m, baseConfig())

	text, err := c.Complete(context.Background(), Request{System: "sys", User: "hello"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if text != "你好，OpenAI" {
		t.Fatalf("文本不匹配: %q", text)
	}
	reqs := m.requests()
	if len(reqs) != 1 {
		t.Fatalf("请求次数 %d != 1", len(reqs))
	}
	req := reqs[0]
	if req.path != "/chat/completions" {
		t.Fatalf("路径 %q != /chat/completions", req.path)
	}
	if got := req.headers.Get("Authorization"); got != "Bearer sk-test" {
		t.Fatalf("Authorization %q", got)
	}
	if got := bodyField(t, req, "model"); got != "deepseek-v4-flash" {
		t.Fatalf("model %v", got)
	}
	messages := bodyField(t, req, "messages").([]any)
	if len(messages) != 2 {
		t.Fatalf("messages 长度 %d != 2", len(messages))
	}
	if bodyField(t, req, "max_tokens").(float64) != 1024 {
		t.Fatalf("max_tokens 应为 1024")
	}
}

func TestAnthropic_Complete(t *testing.T) {
	m := newMockServer(t, KindAnthropic)
	cfg := baseConfig()
	cfg.Kind = KindAnthropic
	c := newTestClient(t, m, cfg)

	text, err := c.Complete(context.Background(), Request{System: "s", User: "u"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if text != "你好，Anthropic" {
		t.Fatalf("文本不匹配: %q", text)
	}
	req := m.requests()[0]
	if req.path != "/v1/messages" {
		t.Fatalf("路径 %q != /v1/messages", req.path)
	}
	if got := req.headers.Get("x-api-key"); got != "sk-test" {
		t.Fatalf("x-api-key %q", got)
	}
	if got := req.headers.Get("anthropic-version"); got != "2023-06-01" {
		t.Fatalf("anthropic-version %q", got)
	}
	if got := bodyField(t, req, "system"); got != "s" {
		t.Fatalf("system %v", got)
	}
	if got := bodyField(t, req, "max_tokens").(float64); got != 1024 {
		t.Fatalf("max_tokens %v", got)
	}
}

func TestResponses_Complete(t *testing.T) {
	m := newMockServer(t, KindResponses)
	cfg := baseConfig()
	cfg.Kind = KindResponses
	c := newTestClient(t, m, cfg)

	text, err := c.Complete(context.Background(), Request{System: "instr", User: "u"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if text != "你好，Responses" {
		t.Fatalf("文本不匹配: %q", text)
	}
	req := m.requests()[0]
	if req.path != "/v1/responses" {
		t.Fatalf("路径 %q != /v1/responses", req.path)
	}
	if got := bodyField(t, req, "instructions"); got != "instr" {
		t.Fatalf("instructions %v", got)
	}
	if got := bodyField(t, req, "input"); got != "u" {
		t.Fatalf("input %v", got)
	}
}

// --- 结构化 JSON 输出 ---

func TestCompleteJSON_OpenAI_UsesResponseFormat(t *testing.T) {
	m := newMockServer(t, KindOpenAI, respSpec{status: 200, body: `{"choices":[{"message":{"content":"{\"ok\":true,\"n\":1}"}}]}`})
	c := newTestClient(t, m, baseConfig())

	raw, err := c.CompleteJSON(context.Background(), Request{User: "u", Schema: `{"type":"object"}`})
	if err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	var out struct {
		OK bool `json:"ok"`
		N  int  `json:"n"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || !out.OK || out.N != 1 {
		t.Fatalf("解析失败: %v raw=%s", err, raw)
	}
	req := m.requests()[0]
	rf := bodyField(t, req, "response_format").(map[string]any)
	if rf["type"] != "json_object" {
		t.Fatalf("response_format %v", rf)
	}
	// Schema 应追加进 user。
	user := bodyField(t, req, "messages").([]any)
	last := user[len(user)-1].(map[string]any)
	if !strings.Contains(last["content"].(string), "JSON Schema") {
		t.Fatalf("user 未包含 Schema 约束: %v", last["content"])
	}
}

func TestCompleteJSON_ExtractFromFence(t *testing.T) {
	body := "```json\n{\"a\": 1}\n``` 解释如上"
	m := newMockServer(t, KindOpenAI, respSpec{status: 200, body: fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, body)})
	c := newTestClient(t, m, baseConfig())

	raw, err := c.CompleteJSON(context.Background(), Request{User: "u"})
	if err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if strings.TrimSpace(string(raw)) != `{"a": 1}` {
		t.Fatalf("raw=%s", raw)
	}
}

func TestCompleteJSON_ExtractArray(t *testing.T) {
	m := newMockServer(t, KindOpenAI, respSpec{status: 200, body: `{"choices":[{"message":{"content":"说明\n[1,2,3]\n完"}}]}`})
	c := newTestClient(t, m, baseConfig())

	raw, err := c.CompleteJSON(context.Background(), Request{User: "u"})
	if err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	var arr []int
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 3 {
		t.Fatalf("解析失败: %v raw=%s", err, raw)
	}
}

func TestCompleteJSON_InvalidOutput(t *testing.T) {
	m := newMockServer(t, KindOpenAI, respSpec{status: 200, body: `{"choices":[{"message":{"content":"没有 JSON"}}]}`})
	c := newTestClient(t, m, baseConfig())

	if _, err := c.CompleteJSON(context.Background(), Request{User: "u"}); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("期望 ErrInvalidResponse, got %v", err)
	}
}

func TestInvalidResponse_NoChoices(t *testing.T) {
	m := newMockServer(t, KindOpenAI, respSpec{status: 200, body: `{"choices":[]}`})
	c := newTestClient(t, m, baseConfig())
	if _, err := c.Complete(context.Background(), Request{User: "u"}); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("期望 ErrInvalidResponse, got %v", err)
	}
}

// --- 重试与错误映射 ---

func TestRetry_5xxThenOK(t *testing.T) {
	m := newMockServer(t, KindOpenAI,
		respSpec{status: 500, body: `{"error":"boom"}`},
		respSpec{status: 200, body: defaultBody(KindOpenAI)},
	)
	c := newTestClient(t, m, baseConfig())

	text, err := c.Complete(context.Background(), Request{User: "u"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if text != "你好，OpenAI" {
		t.Fatalf("text %q", text)
	}
	if n := len(m.requests()); n != 2 {
		t.Fatalf("重试后请求次数 %d != 2", n)
	}
}

func TestRetry_429ThenOK(t *testing.T) {
	m := newMockServer(t, KindOpenAI,
		respSpec{status: 429, body: `{"error":"rate limited"}`},
		respSpec{status: 200, body: defaultBody(KindOpenAI)},
	)
	c := newTestClient(t, m, baseConfig())

	if _, err := c.Complete(context.Background(), Request{User: "u"}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if n := len(m.requests()); n != 2 {
		t.Fatalf("429 应重试, 请求次数 %d", n)
	}
}

func TestNoRetry_400(t *testing.T) {
	m := newMockServer(t, KindOpenAI, respSpec{status: 400, body: `{"error":"bad request"}`})
	c := newTestClient(t, m, baseConfig())

	_, err := c.Complete(context.Background(), Request{User: "u"})
	var apiErr *APIStatusError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 {
		t.Fatalf("期望 APIStatusError(400), got %v", err)
	}
	if n := len(m.requests()); n != 1 {
		t.Fatalf("400 不应重试, 请求次数 %d", n)
	}
}

func TestTimeout(t *testing.T) {
	m := newMockServer(t, KindOpenAI, respSpec{status: 200, body: defaultBody(KindOpenAI), delay: 300 * time.Millisecond})
	cfg := baseConfig()
	cfg.Timeout = 50 * time.Millisecond
	cfg.Retries = 0
	c := newTestClient(t, m, cfg)

	if _, err := c.Complete(context.Background(), Request{User: "u"}); !errors.Is(err, ErrTimeout) {
		t.Fatalf("期望 ErrTimeout, got %v", err)
	}
}

func TestNotConfigured(t *testing.T) {
	_, err := New(Config{}, nil)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("期望 ErrNotConfigured, got %v", err)
	}
}

// --- 环境变量回退与并发 ---

func TestEnvKeyFallback(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-from-env")
	m := newMockServer(t, KindOpenAI)
	cfg := baseConfig()
	cfg.APIKey = "" // 配置空 → 回退环境变量
	c := newTestClient(t, m, cfg)

	if _, err := c.Complete(context.Background(), Request{User: "u"}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := m.requests()[0].headers.Get("Authorization"); got != "Bearer sk-from-env" {
		t.Fatalf("Authorization %q, 期望 env key", got)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	m := newMockServer(t, KindOpenAI, respSpec{status: 200, body: defaultBody(KindOpenAI), delay: 50 * time.Millisecond})
	cfg := baseConfig()
	cfg.Concurrency = 2
	c := newTestClient(t, m, cfg)

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Complete(context.Background(), Request{User: "u"}); err != nil {
				t.Errorf("Complete: %v", err)
			}
		}()
	}
	wg.Wait()
	if peak := atomic.LoadInt32(&m.peak); peak > 2 {
		t.Fatalf("并发峰值 %d > 2", peak)
	}
}

// FromConfig 转换与默认 kind。
func TestFromConfig_Defaults(t *testing.T) {
	cfg := FromConfig(config.LLMConfig{BaseURL: "https://api.deepseek.com/", Model: "m", APIKind: "unknown-kind", TimeoutSec: 30})
	if cfg.Kind != KindOpenAI {
		t.Fatalf("未知 kind 应回退 openai, got %v", cfg.Kind)
	}
	if cfg.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("BaseURL 尾部斜杠未去除: %q", cfg.BaseURL)
	}
	if cfg.Timeout != 30*time.Second {
		t.Fatalf("Timeout 转换错误: %v", cfg.Timeout)
	}
}
