package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tangoforge/internal/config"
)

func TestScanner_ConfigGetter(t *testing.T) {
	svc := newTestService(t)
	cfg := config.DefaultKnowledgeGlobalConfig()
	cfg.DebounceMS = 12345
	sc := NewScanner(svc, cfg, nil, discardLogger())
	if got := sc.Config(); got.DebounceMS != 12345 {
		t.Fatalf("config getter = %+v", got)
	}
}

func TestScanner_StopIdempotent(t *testing.T) {
	svc := newTestService(t)
	sc := newTestScanner(t, svc, nil, 0)
	sc.Stop()
	sc.Stop()
}

func TestScanner_DocumentDirs(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	// 项目内 + 外部文件。
	_ = writeFile(t, workdir, "docs/a.md", "# a")
	extDir := t.TempDir()
	ext := filepath.Join(extDir, "ext.md")
	_ = os.WriteFile(ext, []byte("x"), 0o644)
	if _, err := svc.RegisterDocument(ctx, workdir, filepath.Join(workdir, "docs", "a.md"), CopyAuto, nil); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if _, err := svc.RegisterDocument(ctx, workdir, ext, CopyNone, nil); err != nil {
		t.Fatalf("register ext: %v", err)
	}
	sc := newTestScanner(t, svc, nil, 0)
	dirs := sc.documentDirs(workdir)
	if len(dirs) < 2 {
		t.Fatalf("应含项目内+外部目录: %v", dirs)
	}
	foundInternal, foundExternal := false, false
	for _, d := range dirs {
		if d == filepath.Join(workdir, "docs") {
			foundInternal = true
		}
		if d == extDir {
			foundExternal = true
		}
	}
	if !foundInternal || !foundExternal {
		t.Fatalf("目录未收集完整: %v", dirs)
	}
}

func TestScanner_OpenProjectConn(t *testing.T) {
	workdir := initProject(t)
	conn, err := openProjectConn(workdir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = conn.Close()
	// 未初始化目录 → 错误。
	if _, err := openProjectConn(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("未初始化应报错")
	}
}

func TestScanner_HandleEventAndLocate(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "docs/a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	sc := newTestScanner(t, svc, nil, 100)
	sc.RegisterWorkdir(workdir)

	// locateDoc 命中。
	wd, id := sc.locateDoc(doc.AbsPath)
	if wd != workdir || id != doc.ID {
		t.Fatalf("locate = (%q, %q)", wd, id)
	}
	// locateDoc 未命中。
	if wd, id := sc.locateDoc(filepath.Join(t.TempDir(), "nope.md")); wd != "" || id != "" {
		t.Fatalf("未注册应空: (%q, %q)", wd, id)
	}
	// handleEvent 登记 pending。
	sc.handleEvent(ctx, doc.AbsPath)
	sc.mu.Lock()
	pending := len(sc.pending[workdir])
	sc.mu.Unlock()
	if pending != 1 {
		t.Fatalf("pending = %d, want 1", pending)
	}
	// flushPendingFor 处理（无 embedding，仅更新/缺失标记）。
	sc.flushPendingFor(ctx, workdir)
	sc.mu.Lock()
	left := len(sc.pending[workdir])
	sc.mu.Unlock()
	if left != 0 {
		t.Fatalf("flush 后 pending 应为 0，got %d", left)
	}
	// 防抖窗口内同文档多次事件 → 仍 1 pending。
	sc.handleEvent(ctx, doc.AbsPath)
	sc.handleEvent(ctx, doc.AbsPath)
	sc.mu.Lock()
	p2 := len(sc.pending[workdir])
	sc.mu.Unlock()
	if p2 != 1 {
		t.Fatalf("去重后应为 1 pending，got %d", p2)
	}
}

func TestScanner_FlushPendingAll(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	sc := newTestScanner(t, svc, nil, 0)
	sc.RegisterWorkdir(workdir)
	sc.handleEvent(ctx, doc.AbsPath)
	sc.flushPending(ctx)
}

func TestScanner_RefreshWatchDirs(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 启用 fsnotify 的 scanner。
	cfg := config.DefaultKnowledgeGlobalConfig()
	cfg.DebounceMS = 100
	sc := NewScanner(svc, cfg, nil, discardLogger())
	sc.RegisterWorkdir(workdir)
	if err := sc.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer sc.Stop()
	sc.RefreshWatchDirs()
	_ = doc
}

func TestScanner_UnregisterCleansTimers(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	sc := newTestScanner(t, svc, nil, 100)
	sc.RegisterWorkdir(workdir)
	sc.handleEvent(ctx, doc.AbsPath)
	sc.UnregisterWorkdir(workdir)
	sc.mu.Lock()
	_, hasTimer := sc.timers[workdir]
	_, hasPending := sc.pending[workdir]
	_, hasWatch := sc.watching[workdir]
	sc.mu.Unlock()
	if hasTimer || hasPending || hasWatch {
		t.Fatalf("注销应清理全部: timer=%v pending=%v watch=%v", hasTimer, hasPending, hasWatch)
	}
}

func TestScanner_InFlightGuard(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	sc := newTestScanner(t, svc, nil, 0)
	sc.RegisterWorkdir(workdir)
	sc.mu.Lock()
	sc.inFlight[workdir] = true
	sc.pending[workdir] = map[string]time.Time{doc.ID: time.Now()}
	sc.mu.Unlock()
	// inFlight 时 flushPendingFor 应直接返回（不 panic）。
	sc.flushPendingFor(ctx, workdir)
}

func TestScanner_IndexIfChanged_ContentChange(t *testing.T) {
	svc := newTestService(t)
	svc.SetEmbeddingConfig(mockEmbedFixed(t))
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# 原始"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	sc := newTestScanner(t, svc, svc.(*service).embCfg, 0)
	sc.RegisterWorkdir(workdir)
	// 首次索引。
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}
	d, _ := svc.GetDocument(ctx, workdir, doc.ID)
	if d.ContentHash == "" {
		t.Fatal("首次索引应写 hash")
	}
	// 同 mtime 但内容变化（mtime 快照一致场景）：强制改 hash 记录与文件不同。
	// 直接改数据库 content_hash 为旧值 + 改文件 → 触发内容级变化。
	_ = os.WriteFile(doc.AbsPath, []byte("# 新内容"), 0o644)
	// mtime 可能同秒 → 快速比对可能漏；直接验证慢比对路径：构造 hash 不同。
	conn := mustProjectDB(t, svc, workdir)
	_, _ = conn.Exec(`UPDATE knowledge_documents SET content_hash = 'stale-hash' WHERE id = ?`, doc.ID)
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatalf("scan2: %v", err)
	}
	d2, _ := svc.GetDocument(ctx, workdir, doc.ID)
	if d2.ContentHash == "stale-hash" {
		t.Fatal("内容变化应更新 hash")
	}
}
