package knowledge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tangoforge/internal/llm"
)

// httptest_500 返回恒定 500 的 server（嵌入失败注入；调用方负责 Close）。
func httptest_500() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
}

// mockEmbedFixed 固定向量 mock（openai /embeddings 返回 3 维向量）。
func mockEmbedFixed(t *testing.T) *llm.EmbeddingConfig {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/embeddings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &llm.EmbeddingConfig{
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Model:   "test-embed",
		Kind:    llm.EmbedOpenAI,
	}
}

func TestIndexDocument_TextAndChunks(t *testing.T) {
	svc := NewService(Options{Logger: discardLogger(), LLM: newLLMClient(t, mockChatServer(t, func(req map[string]any) string {
		return `{"summary": "测试摘要"}`
	}))})
	workdir := initProject(t)
	ctx := context.Background()

	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "docs/a.md",
		"# 标题一\n\n内容一\n\n## 标题二\n内容二"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	res, err := svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{ContentHash: "h1"})
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if res.Chunks != 2 {
		t.Fatalf("chunks = %d, want 2", res.Chunks)
	}
	if !res.Summarized {
		t.Fatal("摘要应生成")
	}
	conn := mustProjectDB(t, svc, workdir)
	var embedded int
	var status string
	if err := conn.QueryRow(`SELECT embedded, status FROM knowledge_documents WHERE id = ?`, doc.ID).
		Scan(&embedded, &status); err != nil {
		t.Fatalf("query doc: %v", err)
	}
	if embedded != 0 || status != DocStatusOK {
		t.Fatalf("无 embedding 配置：embedded=%d status=%s", embedded, status)
	}
}

func TestIndexDocument_Embed(t *testing.T) {
	svc := NewService(Options{Logger: discardLogger(), LLM: newLLMClient(t, mockChatServer(t, func(req map[string]any) string {
		return `{"summary": "x"}`
	}))})
	svc.SetEmbeddingConfig(mockEmbedFixed(t))
	workdir := initProject(t)
	ctx := context.Background()

	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# 标题\n内容"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	res, err := svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{ContentHash: "h1", Embedding: svc.(*service).embCfg})
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if !res.Embedded {
		t.Fatal("应嵌入成功")
	}
	conn := mustProjectDB(t, svc, workdir)
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM knowledge_chunks WHERE document_id = ?`, doc.ID).Scan(&n); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if n != 1 {
		t.Fatalf("chunks = %d, want 1", n)
	}
	var embedded, dim int
	var model string
	if err := conn.QueryRow(`SELECT embedded, embedding_model, dim FROM knowledge_documents d
		JOIN knowledge_chunks c ON c.document_id = d.id WHERE d.id = ?`, doc.ID).
		Scan(&embedded, &model, &dim); err != nil {
		t.Fatalf("query embed: %v", err)
	}
	if embedded != 1 || model != "test-embed" || dim != 3 {
		t.Fatalf("embedded=%d model=%s dim=%d", embedded, model, dim)
	}
}

func TestIndexDocument_Binary(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "x.png", "\x89PNG"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	res, err := svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{})
	if err != nil {
		t.Fatalf("index binary: %v", err)
	}
	if res.Embedded {
		t.Fatal("二进制不应嵌入")
	}
}

func TestIndexDocument_MissingFile(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 删除文件 → missing 状态。
	if err := os.Remove(doc.AbsPath); err != nil {
		t.Fatalf("remove: %v", err)
	}
	res, err := svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{})
	if err != nil {
		t.Fatalf("index missing: %v", err)
	}
	if res.Embedded {
		t.Fatal("缺失文件不应嵌入")
	}
	conn := mustProjectDB(t, svc, workdir)
	var status string
	if err := conn.QueryRow(`SELECT status FROM knowledge_documents WHERE id = ?`, doc.ID).Scan(&status); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != DocStatusMissing {
		t.Fatalf("status = %s, want missing", status)
	}
}

func TestIndexDocument_OverLimit(t *testing.T) {
	svc := NewService(Options{Logger: discardLogger()})
	svc.SetEmbeddingConfig(mockEmbedFixed(t))
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	res, err := svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{
		Embedding:    svc.(*service).embCfg,
		MaxIndexSize: 1, // 文件 > 1 字节 → 超限
	})
	if err != nil {
		t.Fatalf("index overlimit: %v", err)
	}
	if !res.Skipped {
		t.Fatal("应标记超限跳过")
	}
	conn := mustProjectDB(t, svc, workdir)
	var idxErr string
	if err := conn.QueryRow(`SELECT index_error FROM knowledge_documents WHERE id = ?`, doc.ID).Scan(&idxErr); err != nil {
		t.Fatalf("query: %v", err)
	}
	if idxErr != "too_large" {
		t.Fatalf("index_error = %q, want too_large", idxErr)
	}
}

func TestIndexDocument_EmbedFail(t *testing.T) {
	// 返回错误的 embedding server → 文档 failed + index_error。
	srv := httptest_500()
	t.Cleanup(srv.Close)
	svc := newTestService(t)
	svc.SetEmbeddingConfig(&llm.EmbeddingConfig{BaseURL: srv.URL, Model: "m", Kind: llm.EmbedOpenAI})
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	res, err := svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{Embedding: svc.(*service).embCfg})
	if err == nil {
		t.Fatalf("嵌入失败应报错，res=%+v", res)
	}
	conn := mustProjectDB(t, svc, workdir)
	var status, idxErr string
	if err := conn.QueryRow(`SELECT status, index_error FROM knowledge_documents WHERE id = ?`, doc.ID).
		Scan(&status, &idxErr); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != DocStatusFailed || idxErr == "" {
		t.Fatalf("status=%s index_error=%q", status, idxErr)
	}
}

func TestSearch_Full(t *testing.T) {
	svc := newTestService(t)
	svc.SetEmbeddingConfig(mockEmbedFixed(t))
	workdir := initProject(t)
	ctx := context.Background()

	// 两个文档。
	docA, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# Alpha\n内容"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register a: %v", err)
	}
	docB, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "b.md", "# Beta\n内容"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register b: %v", err)
	}
	// 指定库（仅 a 加入 spec 库）。
	kb, _ := svc.CreateBase(ctx, workdir, "spec", "")
	if _, err := svc.RegisterDocument(ctx, workdir, filepath.Join(workdir, "a.md"), CopyAuto, []int64{kb.ID}); err != nil {
		t.Fatalf("re-register a with kb: %v", err)
	}
	for _, id := range []string{docA.ID, docB.ID} {
		if _, err := svc.IndexDocument(ctx, workdir, id, IndexOptions{ContentHash: "h", Embedding: svc.(*service).embCfg}); err != nil {
			t.Fatalf("index %s: %v", id, err)
		}
	}

	// 检索（mock 向量全相同 → 都命中）。
	res, err := svc.Search(ctx, workdir, SearchQuery{Q: "查询", TopK: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("total = %d, want 2", res.Total)
	}
	for _, hit := range res.Items {
		if hit.Score <= 0 {
			t.Fatalf("score 应为正: %+v", hit)
		}
		if len(hit.Chunks) == 0 {
			t.Fatalf("应含命中片段: %+v", hit)
		}
	}

	// kb 过滤 → 仅 a。
	res, _ = svc.Search(ctx, workdir, SearchQuery{Q: "查询", KBID: kb.ID, TopK: 10})
	if res.Total != 1 || res.Items[0].Document.ID != docA.ID {
		t.Fatalf("kb 过滤错误: %+v", res)
	}

	// topK 限制。
	res, _ = svc.Search(ctx, workdir, SearchQuery{Q: "查询", TopK: 1})
	if len(res.Items) != 1 {
		t.Fatalf("topK 应 1，got %d", len(res.Items))
	}

	// 高阈值（>1 不可能达到）→ 无结果。
	res, _ = svc.Search(ctx, workdir, SearchQuery{Q: "查询", Threshold: 1.01})
	if res.Total != 0 {
		t.Fatalf("高阈值应无结果，got %d", res.Total)
	}
}

func TestSearch_NotConfigured(t *testing.T) {
	svc := newTestService(t) // 无 embedding 配置
	workdir := initProject(t)
	if _, err := svc.Search(context.Background(), workdir, SearchQuery{Q: "x"}); !errors.Is(err, ErrEmbeddingNotConfigured) {
		t.Fatalf("未配置应 NOT_CONFIGURED，got %v", err)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	if _, err := svc.Search(context.Background(), workdir, SearchQuery{Q: "  "}); !errors.Is(err, ErrDocumentInvalid) {
		t.Fatalf("空查询应 INVALID，got %v", err)
	}
}

func TestSearch_MissingMark(t *testing.T) {
	svc := newTestService(t)
	svc.SetEmbeddingConfig(mockEmbedFixed(t))
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# A"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{Embedding: svc.(*service).embCfg}); err != nil {
		t.Fatalf("index: %v", err)
	}
	// 删除文件 → 检索命中标注 missing。
	_ = os.Remove(doc.AbsPath)
	res, err := svc.Search(ctx, workdir, SearchQuery{Q: "x"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.Total != 1 || !res.Items[0].Missing {
		t.Fatalf("应标注 missing: %+v", res)
	}
}
