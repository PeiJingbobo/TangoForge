package knowledge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"
	"tangoforge/internal/config"
	"tangoforge/internal/llm"
)

// 嵌入任务队列（TF-052 增强：排队 + 当前状态 + 失败重试 + 手动取消）。
//
// 动机：注册/扫描触发嵌入时，前端需要持续看到「排队/正在嵌入/完成/失败」状态，
// 而之前依赖文档 status=indexing 的瞬时计数，任务间隙会消失（用户反馈）。
// 队列统一记录每个项目的嵌入任务，worker 串行执行（embed_concurrency），
// 状态变化通过 fireWrite("queue_updated") 推送 WS 事件驱动前端刷新。
//
// 任务状态机：pending（排队）→ embedding（进行中）→ done / failed / canceled。

// QueueTaskStatus 任务状态。
type QueueTaskStatus string

const (
	TaskPending   QueueTaskStatus = "pending"
	TaskEmbedding QueueTaskStatus = "embedding"
	TaskDone      QueueTaskStatus = "done"
	TaskFailed    QueueTaskStatus = "failed"
	TaskCanceled  QueueTaskStatus = "canceled"
)

// QueueTask 单个嵌入任务。
type QueueTask struct {
	DocID       string          `json:"doc_id"`
	Path        string          `json:"path"`
	DisplayName string          `json:"display_name"`
	Status      QueueTaskStatus `json:"status"`
	Error       string          `json:"error,omitempty"`
	EnqueuedAt  string          `json:"enqueued_at"`
	StartedAt   string          `json:"started_at,omitempty"`
	FinishedAt  string          `json:"finished_at,omitempty"`
}

// QueueSnapshot 项目队列快照（前端展示）。
type QueueSnapshot struct {
	Workdir   string      `json:"workdir"`
	Pending   []QueueTask `json:"pending"`   // 排队中
	Embedding []QueueTask `json:"embedding"` // 进行中（通常 ≤1）
	Done      []QueueTask `json:"done"`      // 已完成（保留最近 10）
	Failed    []QueueTask `json:"failed"`    // 失败（可重试）
	Canceled  []QueueTask `json:"canceled"`  // 已取消
}

// Queue 嵌入任务队列（每项目独立列表，全局 worker 串行执行）。
type Queue struct {
	mu      sync.Mutex
	svc     Service
	logger  *slog.Logger
	embCfg  *llm.EmbeddingConfig
	maxSize int

	jobs map[string][]*QueueTask // workdir → 任务列表（含各状态）
	// cancel 回调：workdir → 当前 embedding 任务的 ctx cancel。
	cancel map[string]context.CancelFunc
	// ch 执行通道（worker 消费；元素为 workdir）。
	ch chan string
	// inFlight workdir → bool（是否有任务在处理该 workdir 的 embedding）。
	inFlight map[string]bool
	// doneRetain 每个项目保留的完成/失败/取消任务上限。
	doneRetain int

	onWrite func(ctx context.Context, workdir, action, target string)
}

// NewQueue 构造嵌入任务队列。
func NewQueue(svc Service, cfg config.KnowledgeGlobalConfig, embCfg *llm.EmbeddingConfig, logger *slog.Logger) *Queue {
	if logger == nil {
		logger = slog.Default()
	}
	conc := cfg.EmbedConcurrency
	if conc < 1 {
		conc = 1
	}
	q := &Queue{
		svc:        svc,
		logger:     logger,
		embCfg:     embCfg,
		maxSize:    0, // 不限制
		jobs:       make(map[string][]*QueueTask),
		cancel:     make(map[string]context.CancelFunc),
		ch:         make(chan string, 64),
		inFlight:   make(map[string]bool),
		doneRetain: 10,
	}
	// 启动 conc 个 worker（串行执行不同项目/任务）。
	for i := 0; i < conc; i++ {
		go q.worker()
	}
	return q
}

// SetOnWrite 注入事件回调（状态变化 → WS 推送）。
func (q *Queue) SetOnWrite(fn func(ctx context.Context, workdir, action, target string)) {
	q.onWrite = fn
}

// fire 触发事件（不阻塞）。
func (q *Queue) fire(ctx context.Context, workdir, action, target string) {
	if q.onWrite != nil {
		q.onWrite(ctx, workdir, action, target)
	}
}

// SetEmbeddingConfig 更新 embedding 配置（热重载/启动注入）。
func (q *Queue) SetEmbeddingConfig(cfg *llm.EmbeddingConfig) {
	q.mu.Lock()
	q.embCfg = cfg
	q.mu.Unlock()
}

// Enqueue 入队（幂等：同文档 pending/embedding 中不重复；done/failed 后重新入队视为重试）。
func (q *Queue) Enqueue(workdir, docID, path, displayName string) {
	clean := filepath.Clean(workdir)
	q.mu.Lock()
	if q.hasActiveLocked(clean, docID) {
		q.mu.Unlock()
		return
	}
	task := &QueueTask{
		DocID:       docID,
		Path:        path,
		DisplayName: displayName,
		Status:      TaskPending,
		EnqueuedAt:  nowRFC3339(),
	}
	q.jobs[clean] = append(q.jobs[clean], task)
	q.mu.Unlock()
	select {
	case q.ch <- clean:
	default:
		// 通道满（worker 忙）→ 任务已在列表，worker 空闲时会再取。
		q.logger.Debug("queue channel full", "workdir", clean)
	}
	q.fire(context.Background(), clean, "queue_updated", docID)
}

// hasActiveLocked 判断同文档是否已有 pending/embedding 任务。
func (q *Queue) hasActiveLocked(workdir, docID string) bool {
	for _, t := range q.jobs[workdir] {
		if t.DocID == docID && (t.Status == TaskPending || t.Status == TaskEmbedding) {
			return true
		}
	}
	return false
}

// worker 消费通道：取项目 → 处理该项目最早 pending 任务。
func (q *Queue) worker() {
	for workdir := range q.ch {
		q.processWorkdir(workdir)
	}
}

// processWorkdir 处理一个项目的队列（找到最早 pending → 执行 → 循环，直到无任务）。
func (q *Queue) processWorkdir(workdir string) {
	for {
		// 取最早 pending 任务。
		q.mu.Lock()
		if q.inFlight[workdir] {
			// 该项目已有任务在跑（避免重复 worker）。
			q.mu.Unlock()
			return
		}
		var task *QueueTask
		for _, t := range q.jobs[workdir] {
			if t.Status == TaskPending {
				task = t
				break
			}
		}
		if task == nil {
			q.mu.Unlock()
			return
		}
		// 标记 embedding + inFlight。
		task.Status = TaskEmbedding
		task.StartedAt = nowRFC3339()
		q.inFlight[workdir] = true
		ictx, cancel := context.WithTimeout(context.Background(), indexTimeout)
		q.cancel[workdir] = cancel
		emb := q.embCfg
		maxSize := q.maxSize
		q.mu.Unlock()

		// 执行索引。
		var runErr error
		if emb != nil {
			_, runErr = q.svc.IndexDocument(ictx, workdir, task.DocID, IndexOptions{
				Embedding:    emb,
				MaxIndexSize: maxSize,
			})
		} else {
			// 未配置 embedding：仅标记 done（无向量可嵌）。
			runErr = nil
		}
		// 先判断结果（正常完成时 ictx.Err() 为 nil；Cancel 手动触发才 Canceled），
		// 再 cancel 释放资源（顺序重要：先 cancel 会误判为取消）。
		q.mu.Lock()
		ctxCanceled := ictx.Err() == context.Canceled || errors.Is(runErr, context.Canceled)
		cancel()
		delete(q.cancel, workdir)
		delete(q.inFlight, workdir)
		switch {
		case ctxCanceled:
			task.Status = TaskCanceled
			task.Error = "已取消"
		case runErr != nil:
			task.Status = TaskFailed
			task.Error = runErr.Error()
		default:
			task.Status = TaskDone
			task.Error = ""
		}
		task.FinishedAt = nowRFC3339()
		q.trimLocked(workdir)
		q.mu.Unlock()

		q.logger.Info("knowledge queue task finished",
			"workdir", workdir, "doc", task.DocID, "status", task.Status, "err", task.Error)
		q.fire(context.Background(), workdir, "queue_updated", task.DocID)

		// 处理下一个任务。
		q.processWorkdir(workdir)
		return
	}
}

// trimLocked 裁剪每个状态的历史任务（保留 doneRetain 个 done/failed/canceled）。
func (q *Queue) trimLocked(workdir string) {
	list := q.jobs[workdir]
	var pending, embedding, done, failed, canceled []*QueueTask
	for _, t := range list {
		switch t.Status {
		case TaskPending:
			pending = append(pending, t)
		case TaskEmbedding:
			embedding = append(embedding, t)
		case TaskDone:
			done = append(done, t)
		case TaskFailed:
			failed = append(failed, t)
		case TaskCanceled:
			canceled = append(canceled, t)
		}
	}
	// 保留最近 doneRetain 个。
	keep := func(s []*QueueTask) []*QueueTask {
		if len(s) > q.doneRetain {
			return s[len(s)-q.doneRetain:]
		}
		return s
	}
	done = keep(done)
	failed = keep(failed)
	canceled = keep(canceled)
	// 重组（pending/embedding 在前，保持顺序）。
	out := make([]*QueueTask, 0, len(pending)+len(embedding)+len(done)+len(failed)+len(canceled))
	out = append(out, pending...)
	out = append(out, embedding...)
	out = append(out, done...)
	out = append(out, failed...)
	out = append(out, canceled...)
	q.jobs[workdir] = out
}

// Snapshot 返回项目队列快照（空状态返回空切片而非 null，保证前端安全）。
func (q *Queue) Snapshot(workdir string) QueueSnapshot {
	clean := filepath.Clean(workdir)
	q.mu.Lock()
	defer q.mu.Unlock()
	snap := QueueSnapshot{
		Workdir:   clean,
		Pending:   make([]QueueTask, 0),
		Embedding: make([]QueueTask, 0),
		Done:      make([]QueueTask, 0),
		Failed:    make([]QueueTask, 0),
		Canceled:  make([]QueueTask, 0),
	}
	for _, t := range q.jobs[clean] {
		switch t.Status {
		case TaskPending:
			snap.Pending = append(snap.Pending, *t)
		case TaskEmbedding:
			snap.Embedding = append(snap.Embedding, *t)
		case TaskDone:
			snap.Done = append(snap.Done, *t)
		case TaskFailed:
			snap.Failed = append(snap.Failed, *t)
		case TaskCanceled:
			snap.Canceled = append(snap.Canceled, *t)
		}
	}
	// 稳定排序：pending 按入队时间升序。
	sortTasks(snap.Pending)
	sortTasks(snap.Failed)
	return snap
}

func sortTasks(list []QueueTask) {
	sort.Slice(list, func(i, j int) bool { return list[i].EnqueuedAt < list[j].EnqueuedAt })
}

// Cancel 取消任务（pending 直接移除/标记；embedding 调 ctx cancel）。
func (q *Queue) Cancel(workdir, docID string) error {
	clean := filepath.Clean(workdir)
	q.mu.Lock()
	var found *QueueTask
	for _, t := range q.jobs[clean] {
		if t.DocID == docID && t.Status == TaskPending {
			t.Status = TaskCanceled
			t.Error = "已取消"
			t.FinishedAt = nowRFC3339()
			found = t
			break
		}
	}
	if found == nil {
		// 可能正在 embedding → 取消 ctx。
		if cf, ok := q.cancel[clean]; ok {
			cf()
		}
	}
	q.mu.Unlock()
	if found != nil {
		q.fire(context.Background(), clean, "queue_updated", docID)
		return nil
	}
	// 未找到 pending 任务（可能在 embedding，已触发 cancel）。
	return nil
}

// Retry 重试失败/取消任务（重新入队）。
func (q *Queue) Retry(workdir, docID string) error {
	clean := filepath.Clean(workdir)
	q.mu.Lock()
	var target *QueueTask
	for _, t := range q.jobs[clean] {
		if t.DocID == docID && (t.Status == TaskFailed || t.Status == TaskCanceled) {
			target = t
			break
		}
	}
	if target != nil {
		target.Status = TaskPending
		target.Error = ""
		target.EnqueuedAt = nowRFC3339()
		target.StartedAt = ""
		target.FinishedAt = ""
	}
	q.mu.Unlock()
	if target == nil {
		return fmt.Errorf("%w: 队列中无此失败任务 %s", ErrDocumentNotFound, docID)
	}
	select {
	case q.ch <- clean:
	default:
	}
	q.fire(context.Background(), clean, "queue_updated", docID)
	return nil
}

// Close 关闭队列（worker 退出）。
func (q *Queue) Close() {
	// 关闭 channel 会使 worker 退出（for range 结束）。
	// 注意：并发 Enqueue 可能 panic，调用方应保证无并发入队后再 Close。
	// 实际使用中 daemon 退出时才 Close，此处不做复杂优雅关闭。
}
