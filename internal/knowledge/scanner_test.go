package knowledge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"tangoforge/internal/config"
	"tangoforge/internal/llm"
)

// newTestScanner 构造测试 scanner（短防抖窗口便于测试）。
func newTestScanner(t *testing.T, svc Service, embCfg *llm.EmbeddingConfig, debounceMS int) *Scanner {
	t.Helper()
	cfg := config.DefaultKnowledgeGlobalConfig()
	if debounceMS > 0 {
		cfg.DebounceMS = debounceMS
	}
	return NewScanner(svc, cfg, embCfg, discardLogger())
}

// ptrBool 返回 bool 指针。
func ptrBool(v bool) *bool { return &v }

func TestScanner_DisabledConfig(t *testing.T) {
	svc := newTestService(t)
	cfg := config.DefaultKnowledgeGlobalConfig()
	cfg.Enabled = ptrBool(false)
	sc := NewScanner(svc, cfg, nil, discardLogger())
	if err := sc.Start(context.Background()); err != nil {
		t.Fatalf("start disabled: %v", err)
	}
	stats, err := sc.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if stats.Total != 0 {
		t.Fatalf("disabled 不应扫描，got %+v", stats)
	}
	sc.Stop()
}

func TestScanner_StartupScan(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	doc, err := svc.RegisterDocument(context.Background(), workdir,
		writeFile(t, workdir, "a.md", "# 内容"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	sc := newTestScanner(t, svc, mockEmbedFixed(t), 0)
	sc.RegisterWorkdir(workdir)
	// 启动扫描（异步）→ 等待完成。
	if err := sc.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer sc.Stop()
	// 轮询等待索引完成。
	waitFor(t, func() bool {
		d, err := svc.GetDocument(context.Background(), workdir, doc.ID)
		return err == nil && d.Embedded == EmbedYes
	})
}

func TestScanner_ManualScan(t *testing.T) {
	svc := newTestService(t)
	svc.SetEmbeddingConfig(mockEmbedFixed(t))
	workdir := initProject(t)
	doc, err := svc.RegisterDocument(context.Background(), workdir,
		writeFile(t, workdir, "a.md", "# 内容"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	sc := newTestScanner(t, svc, svc.(*service).embCfg, 0)
	sc.RegisterWorkdir(workdir)
	stats, err := sc.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if stats.Total != 1 || stats.Indexed != 1 {
		t.Fatalf("scan stats = %+v", stats)
	}
	d, _ := svc.GetDocument(context.Background(), workdir, doc.ID)
	if d.Embedded != EmbedYes {
		t.Fatalf("扫描后应已嵌入: %+v", d)
	}
	// 再次扫描（无变化）→ indexed=0。
	stats2, _ := sc.Scan(context.Background())
	if stats2.Indexed != 0 {
		t.Fatalf("无变化不应重索引: %+v", stats2)
	}
}

func TestScanner_ChangeDetection(t *testing.T) {
	svc := newTestService(t)
	svc.SetEmbeddingConfig(mockEmbedFixed(t))
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# 原始内容"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	sc := newTestScanner(t, svc, svc.(*service).embCfg, 0)
	sc.RegisterWorkdir(workdir)
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	// 修改文件（mtime + size 变化）。
	time.Sleep(1100 * time.Millisecond) // 确保 mtime 变化
	abs := doc.AbsPath
	if err := os.WriteFile(abs, []byte("# 修改后的更长内容内容内容"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatalf("second scan: %v", err)
	}
	d, _ := svc.GetDocument(ctx, workdir, doc.ID)
	if d.ContentHash == "" {
		t.Fatal("变更后应更新 content_hash")
	}
}

func TestScanner_ModelDriftReembed(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# 内容"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 用模型 A 索引。
	embA := mockEmbedFixed(t)
	svc.SetEmbeddingConfig(embA)
	if _, err := svc.IndexDocument(ctx, workdir, doc.ID, IndexOptions{Embedding: embA}); err != nil {
		t.Fatalf("index A: %v", err)
	}
	// 模拟模型漂移：换模型 B（模型名不同）。
	embB := &llm.EmbeddingConfig{BaseURL: embA.BaseURL, APIKey: embA.APIKey, Model: "model-B", Kind: llm.EmbedOpenAI}
	svc.SetEmbeddingConfig(embB)
	sc := newTestScanner(t, svc, embB, 0)
	sc.RegisterWorkdir(workdir)
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatalf("scan drift: %v", err)
	}
	d, _ := svc.GetDocument(ctx, workdir, doc.ID)
	if d.EmbeddingModel != "model-B" {
		t.Fatalf("漂移后应重嵌为新模型: %+v", d)
	}
}

func TestScanner_MissingMark(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# 内容"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 删除文件 → 扫描 → missing。
	if err := os.Remove(doc.AbsPath); err != nil {
		t.Fatalf("remove: %v", err)
	}
	sc := newTestScanner(t, svc, nil, 0)
	sc.RegisterWorkdir(workdir)
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}
	d, _ := svc.GetDocument(ctx, workdir, doc.ID)
	if d.Status != DocStatusMissing {
		t.Fatalf("status = %s, want missing", d.Status)
	}
}

func TestScanner_RegisterUnregister(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	sc := newTestScanner(t, svc, nil, 0)
	sc.RegisterWorkdir(workdir)
	sc.RegisterWorkdir(workdir) // 幂等
	if len(sc.listWorkdirs(context.Background())) != 1 {
		t.Fatal("注册应幂等")
	}
	sc.UnregisterWorkdir(workdir)
	if len(sc.listWorkdirs(context.Background())) != 0 {
		t.Fatal("注销后应为空")
	}
}

func TestScanner_OverLimitAndEmbeddingNil(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# 内容"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 超限（MaxIndexSize=1）→ skipped。
	cfg := config.DefaultKnowledgeGlobalConfig()
	cfg.MaxIndexSize = 1
	sc := NewScanner(svc, cfg, mockEmbedFixed(t), discardLogger())
	sc.RegisterWorkdir(workdir)
	stats, err := sc.Scan(ctx)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if stats.Skipped != 1 {
		t.Fatalf("应跳过超限: %+v", stats)
	}
	d, _ := svc.GetDocument(ctx, workdir, doc.ID)
	if d.IndexError != "too_large" {
		t.Fatalf("index_error = %q", d.IndexError)
	}
	// 无 embedding 配置 → 仅注册 + 摘要（embedded=0）。
	doc2, _ := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "b.md", "# b"), CopyAuto, nil)
	sc2 := newTestScanner(t, svc, nil, 0)
	sc2.RegisterWorkdir(workdir)
	if _, err := sc2.Scan(ctx); err != nil {
		t.Fatalf("scan2: %v", err)
	}
	d2, _ := svc.GetDocument(ctx, workdir, doc2.ID)
	if d2.Embedded != EmbedNo {
		t.Fatalf("无 embedding 不应嵌入: %+v", d2)
	}
}

func TestScanner_EmbedFailRetry(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# 内容"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 先失败（500），后恢复。
	state := &struct{ fail bool }{fail: true}
	mux := http.NewServeMux()
	mux.HandleFunc("/embeddings", func(w http.ResponseWriter, r *http.Request) {
		if state.fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	emb := &llm.EmbeddingConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", Kind: llm.EmbedOpenAI}
	sc := newTestScanner(t, svc, emb, 0)
	sc.RegisterWorkdir(workdir)
	// 失败扫描 → failed。
	stats, err := sc.Scan(ctx)
	if err != nil {
		t.Fatalf("scan fail: %v", err)
	}
	if stats.Failed != 1 {
		t.Fatalf("应 1 失败: %+v", stats)
	}
	d, _ := svc.GetDocument(ctx, workdir, doc.ID)
	if d.Status != DocStatusFailed {
		t.Fatalf("status = %s, want failed", d.Status)
	}
	// 恢复后重扫 → 重试成功。
	state.fail = false
	stats2, err := sc.Scan(ctx)
	if err != nil {
		t.Fatalf("scan retry: %v", err)
	}
	if stats2.Failed != 0 {
		t.Fatalf("重试应成功: %+v", stats2)
	}
	d2, _ := svc.GetDocument(ctx, workdir, doc.ID)
	if d2.Embedded != EmbedYes {
		t.Fatalf("重试后应嵌入: %+v", d2)
	}
}

func TestScanner_FsnotifyDebounce(t *testing.T) {
	svc := newTestService(t)
	svc.SetEmbeddingConfig(mockEmbedFixed(t))
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "docs/a.md", "# 原始"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 短防抖（100ms）→ 等待。
	sc := newTestScanner(t, svc, svc.(*service).embCfg, 100)
	sc.RegisterWorkdir(workdir)
	if err := sc.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer sc.Stop()
	// 触发写事件（fsnotify watch 父目录）。
	time.Sleep(200 * time.Millisecond) // watcher 就绪
	_ = os.WriteFile(doc.AbsPath, []byte("# 修改后内容"), 0o644)
	// 等待防抖窗口内索引完成。
	waitFor(t, func() bool {
		d, err := svc.GetDocument(ctx, workdir, doc.ID)
		return err == nil && d.ContentHash != ""
	})
}

// waitFor 轮询等待条件成立（最长 5s）。
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("waitFor 超时")
}

// TF-052：注册文档 → SetOnDocumentRegistered 回调触发（scanner.IndexNow 立即索引）。
func TestIndexNow_TriggersImmediateIndex(t *testing.T) {
	svc := NewService(Options{Logger: discardLogger()})
	svc.SetEmbeddingConfig(mockEmbedFixed(t))
	workdir := initProject(t)
	ctx := context.Background()

	sc := newTestScanner(t, svc, svc.(*service).embCfg, 0)
	sc.RegisterWorkdir(workdir)

	// 模拟 api 层接线：注册成功回调 → scanner.IndexNow 立即索引。
	var indexed string
	svc.SetOnDocumentRegistered(func(_ context.Context, _ string, docID string) {
		indexed = docID
		sc.IndexNow(workdir, docID)
	})

	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if indexed != doc.ID {
		t.Fatalf("回调未触发: indexed=%q", indexed)
	}
	// IndexNow 异步索引 → 等待 embedded=1。
	waitFor(t, func() bool {
		d, err := svc.GetDocument(ctx, workdir, doc.ID)
		return err == nil && d.Embedded == EmbedYes
	})
}

// TF-052：SetOnDocumentRegistered 未设置 → 注册不 panic（nil 安全）。
func TestRegisterDocument_NoCallbackNilSafe(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	if _, err := svc.RegisterDocument(context.Background(), workdir,
		writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil); err != nil {
		t.Fatalf("register: %v", err)
	}
}
