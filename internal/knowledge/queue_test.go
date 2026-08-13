package knowledge

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tangoforge/internal/config"
	"tangoforge/internal/llm"
)

func newTestQueue(t *testing.T, svc Service, emb *llm.EmbeddingConfig) *Queue {
	t.Helper()
	cfg := config.DefaultKnowledgeGlobalConfig()
	cfg.EmbedConcurrency = 1
	q := NewQueue(svc, cfg, emb, discardLogger())
	t.Cleanup(q.Close)
	return q
}

func TestQueue_EnqueueAndSnapshot(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	q := newTestQueue(t, svc, nil) // 无 embedding → done
	q.Enqueue(workdir, doc.ID, doc.Path, doc.DisplayName)

	// 等待完成。
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snap := q.Snapshot(workdir)
		if len(snap.Done) == 1 {
			if snap.Done[0].DocID != doc.ID || snap.Done[0].Status != TaskDone {
				t.Fatalf("done task wrong: %+v", snap.Done[0])
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("任务未完成")
}

func TestQueue_EnqueueIdempotent(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 慢 embedding 保持 pending 状态，验证去重。
	q := newTestQueue(t, svc, slowEmbedCfg(t))
	q.Enqueue(workdir, doc.ID, doc.Path, doc.DisplayName)
	q.Enqueue(workdir, doc.ID, doc.Path, doc.DisplayName) // 重复 → 忽略
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		snap := q.Snapshot(workdir)
		if len(snap.Pending)+len(snap.Embedding) >= 1 {
			// 只能有 1 个活跃任务（无重复）。
			if len(snap.Pending)+len(snap.Embedding) != 1 {
				t.Fatalf("重复入队应去重: %+v", snap)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 若已 done 也接受（慢 embedding 不应这么快完成，但防御）。
}

func TestQueue_CancelPending(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	q := newTestQueue(t, svc, slowEmbedCfg(t))
	q.Enqueue(workdir, doc.ID, doc.Path, doc.DisplayName)
	// 立即取消（pending 或 embedding 中）。
	if err := q.Cancel(workdir, doc.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	// 等待状态更新。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := q.Snapshot(workdir)
		if len(snap.Canceled) == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	// 可能已取消或正在取消（ctx cancel 生效）。
	snap := q.Snapshot(workdir)
	if len(snap.Canceled)+len(snap.Pending)+len(snap.Embedding) == 0 {
		t.Fatal("取消后应有 canceled 记录")
	}
}

func TestQueue_Retry(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 先制造失败（500 embedding）。
	q := newTestQueue(t, svc, failingEmbedCfg(t))
	q.Enqueue(workdir, doc.ID, doc.Path, doc.DisplayName)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snap := q.Snapshot(workdir)
		if len(snap.Failed) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	snap := q.Snapshot(workdir)
	if len(snap.Failed) != 1 {
		t.Fatalf("应失败: %+v", snap)
	}
	// 修复 embedding 后重试。
	q.SetEmbeddingConfig(mockEmbedFixed(t))
	if err := q.Retry(workdir, doc.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snap := q.Snapshot(workdir)
		if len(snap.Done) == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("重试后应成功")
}

func TestQueue_RetryUnknown(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	q := newTestQueue(t, svc, nil)
	if err := q.Retry(workdir, "nope"); err == nil {
		t.Fatal("重试未知任务应报错")
	}
}

func TestQueue_SnapshotEmpty(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	q := newTestQueue(t, svc, nil)
	snap := q.Snapshot(workdir)
	if len(snap.Pending)+len(snap.Embedding)+len(snap.Done)+len(snap.Failed)+len(snap.Canceled) != 0 {
		t.Fatalf("空队列应空: %+v", snap)
	}
}

// slowEmbedCfg 慢 embedding（延迟 300ms，便于观察 pending/embedding）。
func slowEmbedCfg(t *testing.T) *llm.EmbeddingConfig {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/embeddings", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &llm.EmbeddingConfig{BaseURL: srv.URL, Model: "m", Kind: llm.EmbedOpenAI}
}

// failingEmbedCfg 恒 500 embedding。
func failingEmbedCfg(t *testing.T) *llm.EmbeddingConfig {
	t.Helper()
	srv := httptest_500()
	t.Cleanup(srv.Close)
	return &llm.EmbeddingConfig{BaseURL: srv.URL, Model: "m", Kind: llm.EmbedOpenAI}
}

// Queue 事件与工具函数覆盖。
func TestQueue_OnWriteAndTools(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// SetOnWrite + fire 事件。
	q := newTestQueue(t, svc, nil)
	var events []string
	q.SetOnWrite(func(_ context.Context, _ string, action, _ string) {
		events = append(events, action)
	})
	q.Enqueue(workdir, doc.ID, doc.Path, doc.DisplayName)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(events) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(events) == 0 {
		t.Fatal("应有 queue_updated 事件")
	}
	// sortTasks 稳定性。
	list := []QueueTask{
		{EnqueuedAt: "2026-01-01T00:00:00+08:00"},
		{EnqueuedAt: "2025-01-01T00:00:00+08:00"},
	}
	sortTasks(list)
	if list[0].EnqueuedAt != "2025-01-01T00:00:00+08:00" {
		t.Fatal("sortTasks 应按入队时间升序")
	}
	// Close 幂等。
	q.Close()
	q.Close()
}

// Queue trimLocked 裁剪历史（超过保留上限）。
func TestQueue_TrimLocked(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	q := newTestQueue(t, svc, nil)
	// 注入 15 个 done 任务（直接操作内部）。
	for i := 0; i < 15; i++ {
		q.mu.Lock()
		q.jobs[workdir] = append(q.jobs[workdir], &QueueTask{
			DocID: fmt.Sprintf("doc-%d", i), Status: TaskDone, FinishedAt: nowRFC3339(),
		})
		q.mu.Unlock()
	}
	q.mu.Lock()
	q.trimLocked(workdir)
	n := len(q.jobs[workdir])
	q.mu.Unlock()
	if n > q.doneRetain {
		t.Fatalf("裁剪后应 ≤ %d，got %d", q.doneRetain, n)
	}
}
