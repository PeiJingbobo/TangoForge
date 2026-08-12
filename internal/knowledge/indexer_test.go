package knowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"tangoforge/internal/llm"
)

// mockChatServer 模拟 OpenAI chat completions（摘要生成用）。
func mockChatServer(t *testing.T, respond func(req map[string]any) string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		content := respond(req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"content": content},
				"finish_reason": "stop",
			}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newLLMClient 由 mock server 构造 llm.Client。
func newLLMClient(t *testing.T, srv *httptest.Server) *llm.Client {
	t.Helper()
	cl, err := llm.New(llm.Config{
		BaseURL: srv.URL,
		APIKey:  "test",
		Model:   "mock-model",
	}, nil)
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}
	return cl
}

func TestGenerateSummary(t *testing.T) {
	srv := mockChatServer(t, func(req map[string]any) string {
		return `{"summary": "这是文档摘要，涵盖核心内容。"}`
	})
	cl := newLLMClient(t, srv)
	got := GenerateSummary(context.Background(), cl, "文档内容：关于任务管理的说明")
	if !strings.Contains(got, "文档摘要") {
		t.Fatalf("summary = %q", got)
	}
	// 超长摘要截断到 200 字。
	len200 := strings.Repeat("长", 250)
	srv2 := mockChatServer(t, func(req map[string]any) string {
		return `{"summary": "` + len200 + `"}`
	})
	cl2 := newLLMClient(t, srv2)
	got2 := GenerateSummary(context.Background(), cl2, "x")
	if len([]rune(got2)) > maxSummaryChars {
		t.Fatalf("摘要未截断: %d", len([]rune(got2)))
	}
}

func TestGenerateSummary_Failures(t *testing.T) {
	// nil client → 空串。
	if got := GenerateSummary(context.Background(), nil, "x"); got != "" {
		t.Fatalf("nil client 应返回空: %q", got)
	}
	// 空文本 → 空串。
	cl := newLLMClient(t, mockChatServer(t, func(req map[string]any) string { return "{}" }))
	if got := GenerateSummary(context.Background(), cl, "  "); got != "" {
		t.Fatalf("空文本应返回空: %q", got)
	}
	// LLM 返回非法 JSON → 空串（不阻断）。
	cl2 := newLLMClient(t, mockChatServer(t, func(req map[string]any) string { return "not-json" }))
	if got := GenerateSummary(context.Background(), cl2, "x"); got != "" {
		t.Fatalf("非法响应应返回空: %q", got)
	}
	// LLM 返回缺 summary 字段 → 空串。
	cl3 := newLLMClient(t, mockChatServer(t, func(req map[string]any) string { return `{"other":1}` }))
	if got := GenerateSummary(context.Background(), cl3, "x"); got != "" {
		t.Fatalf("缺字段应返回空: %q", got)
	}
	// 服务端 500 → 空串。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	cl4 := newLLMClient(t, srv)
	if got := GenerateSummary(context.Background(), cl4, "x"); got != "" {
		t.Fatalf("500 应返回空: %q", got)
	}
}

func TestSummarizeAndCache(t *testing.T) {
	srv := mockChatServer(t, func(req map[string]any) string {
		return `{"summary": "缓存摘要内容"}`
	})
	svc := NewService(Options{Logger: discardLogger(), LLM: newLLMClient(t, srv)})
	workdir := initProject(t)
	ctx := context.Background()

	// 注册文档（初始 content_hash 空）。
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 生成摘要并缓存。
	summary, err := svc.SummarizeAndCache(ctx, workdir, doc.ID, "hash1", "文档文本内容")
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if summary == "" {
		t.Fatal("摘要应为非空")
	}
	// 同 hash 再次调用 → 缓存命中（返回缓存摘要，不再调 LLM）。
	// 通过关闭 LLM client 验证缓存路径：摘要已缓存则无需 LLM。
	srvCount := 0
	srv2 := mockChatServer(t, func(req map[string]any) string {
		srvCount++
		return `{"summary": "第二次摘要"}`
	})
	svc2 := NewService(Options{Logger: discardLogger(), LLM: newLLMClient(t, srv2)})
	workdir2 := initProject(t)
	doc2, err := svc2.RegisterDocument(ctx, workdir2, writeFile(t, workdir2, "b.md", "# b"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register2: %v", err)
	}
	if _, err := svc2.SummarizeAndCache(ctx, workdir2, doc2.ID, "h", "t"); err != nil {
		t.Fatalf("first summarize: %v", err)
	}
	if srvCount != 1 {
		t.Fatalf("首次应调用 LLM 1 次，got %d", srvCount)
	}
	// 缓存命中（hash 一致）→ 不再调用 LLM。
	if _, err := svc2.SummarizeAndCache(ctx, workdir2, doc2.ID, "h", "t"); err != nil {
		t.Fatalf("cached summarize: %v", err)
	}
	if srvCount != 1 {
		t.Fatalf("缓存命中不应调 LLM，got %d 次", srvCount)
	}
	// hash 变化 → 重新生成。
	if _, err := svc2.SummarizeAndCache(ctx, workdir2, doc2.ID, "h2", "t2"); err != nil {
		t.Fatalf("re-summarize: %v", err)
	}
	if srvCount != 2 {
		t.Fatalf("hash 变化应重新生成，got %d 次", srvCount)
	}
}

func TestSummarizeAndCache_NoClient(t *testing.T) {
	svc := newTestService(t) // 无 LLM client
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 无 client → 跳过摘要（nil error）。
	summary, err := svc.SummarizeAndCache(ctx, workdir, doc.ID, "h", "t")
	if err != nil {
		t.Fatalf("no-client summarize: %v", err)
	}
	if summary != "" {
		t.Fatalf("无 client 应返回空摘要: %q", summary)
	}
	// 空 hash → 跳过。
	summary, err = svc.SummarizeAndCache(ctx, workdir, doc.ID, "", "t")
	if err != nil {
		t.Fatalf("empty hash: %v", err)
	}
	if summary != "" {
		t.Fatalf("空 hash 应返回空: %q", summary)
	}
}

var _ = filepath.Join
