package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"tangoforge/internal/llm"
)

func TestIndexDocument_EmptyDoc(t *testing.T) {
	svc := NewService(Options{Logger: discardLogger()})
	svc.SetEmbeddingConfig(mockEmbedFixed(t))
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "empty.md", "   \n  "), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	res, err := svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{
		Embedding:    svc.(*service).embCfg,
		ContentHash:  "h",
		MaxIndexSize: 0,
	})
	if err != nil {
		t.Fatalf("index empty: %v", err)
	}
	if res.Embedded {
		t.Fatal("空文档不应嵌入")
	}
	conn := mustProjectDB(t, svc, workdir)
	var status string
	if err := conn.QueryRow(`SELECT status FROM knowledge_documents WHERE id = ?`, doc.ID).Scan(&status); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != DocStatusOK {
		t.Fatalf("status = %s, want ok", status)
	}
}

func TestIndexDocument_NoEmbeddingSkipped(t *testing.T) {
	// 无 embedding 配置（opts.Embedding=nil）→ 仅注册+摘要，不嵌入。
	svc := NewService(Options{Logger: discardLogger()})
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	res, err := svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{ContentHash: "h"})
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if res.Embedded {
		t.Fatal("无配置不应嵌入")
	}
}

func TestIndexDocument_ReadError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 目录替换文件 → os.ReadFile 失败（Is a directory）。
	dir := doc.AbsPath + ".d"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = os.Remove(doc.AbsPath)
	_ = os.Rename(dir, doc.AbsPath)
	_, err = svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{})
	if err == nil {
		t.Fatal("读取目录应报错")
	}
}

func TestIndexDocument_EmbeddingUnconfiguredError(t *testing.T) {
	// embedding 配置为空 base_url → EMBEDDING_NOT_CONFIGURED 错误。
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	cfg := &llm.EmbeddingConfig{Model: "m", Kind: llm.EmbedOpenAI}
	_, err = svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{Embedding: cfg})
	if err == nil {
		t.Fatal("空 base_url 嵌入应报错")
	}
}

func TestIndexDocument_ChunkInsertFail(t *testing.T) {
	svc := NewService(Options{Logger: discardLogger()})
	svc.SetEmbeddingConfig(mockEmbedFixed(t))
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 删除 knowledge_chunks 表 → INSERT chunk 失败。
	dropTable(t, svc, workdir, "knowledge_chunks")
	_, err = svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{Embedding: svc.(*service).embCfg})
	if err == nil {
		t.Fatal("chunks 表缺失插入应报错")
	}
}

func TestOptionalKB(t *testing.T) {
	if optionalKB(0) != nil {
		t.Fatal("0 应返回 nil")
	}
	if optionalKB(3) != int64(3) {
		t.Fatal("3 应返回 3")
	}
}

func TestIsSummaryFailed(t *testing.T) {
	if !IsSummaryFailed("") {
		t.Fatal("空串应视为失败")
	}
	if IsSummaryFailed("x") {
		t.Fatal("非空不应视为失败")
	}
}

func TestReadTextFile_Errors(t *testing.T) {
	// 不存在 → 错误。
	if _, err := readTextFile(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("不存在文件应报错")
	}
	// 正常读取。
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	_ = os.WriteFile(p, []byte("内容"), 0o644)
	got, err := readTextFile(p)
	if err != nil || got != "内容" {
		t.Fatalf("read = %q err=%v", got, err)
	}
}

func TestDecodeVectorF32_OddLength(t *testing.T) {
	if v := decodeVectorF32([]byte{1, 2, 3}); v != nil {
		t.Fatalf("非 4 倍数字节应返回 nil: %v", v)
	}
}

func TestSearch_KBFilterSQL(t *testing.T) {
	svc := newTestService(t)
	svc.SetEmbeddingConfig(mockEmbedFixed(t))
	workdir := initProject(t)
	ctx := context.Background()

	kb, err := svc.CreateBase(ctx, workdir, "spec", "")
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# A"), CopyAuto, []int64{kb.ID})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{Embedding: svc.(*service).embCfg}); err != nil {
		t.Fatalf("index: %v", err)
	}
	// 检索不存在的库 → 空结果（非错误）。
	res, err := svc.Search(ctx, workdir, SearchQuery{Q: "x", KBID: 9999})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.Total != 0 {
		t.Fatalf("不存在库应空结果，got %d", res.Total)
	}
}

// TestIndexDocument_DocNotFound 索引不存在文档 → DOCUMENT_NOT_FOUND。
func TestIndexDocument_DocNotFound(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	if _, err := svc.IndexDocument(context.Background(), workdir, "nope", IndexOptions{}); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("应 NOT_FOUND，got %v", err)
	}
}

// TestSummarizeAndCache_CacheHit 缓存命中（摘要与 hash 一致）→ 返回缓存摘要。
func TestSummarizeAndCache_CacheHit(t *testing.T) {
	svc := NewService(Options{Logger: discardLogger(), LLM: newLLMClient(t, mockChatServer(t, func(req map[string]any) string {
		return `{"summary": "缓存摘要"}`
	}))})
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	got, err := svc.SummarizeAndCache(ctx, workdir, doc.ID, "hash-x", "内容")
	if err != nil || got == "" {
		t.Fatalf("summarize: %q err=%v", got, err)
	}
	// 再次同 hash → 缓存命中返回相同摘要。
	got2, err := svc.SummarizeAndCache(ctx, workdir, doc.ID, "hash-x", "内容")
	if err != nil || got2 != got {
		t.Fatalf("cache hit: %q != %q err=%v", got2, got, err)
	}
}
