package knowledge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"tangoforge/internal/config"
	"tangoforge/internal/db"
	"tangoforge/internal/llm"
)

// Scanner 文件扫描与自动重索引（docs/KNOWLEDGE-BASE.md §2.7，QA-K8）。
//
// 触发时机：启动扫描（startup_scan）、fsnotify 实时监听（fsnotify，watch 已注册文档唯一父目录）、
// 手动触发（Scan）、按需懒校验（读取/检索/列表 stat）。
// 防抖：事件按文档合并到 debounce 窗口；singleflight：同一文档同时只一个索引任务；
// embedding/摘要串行（embed_concurrency=1）。
type Scanner struct {
	svc      Service
	logger   *slog.Logger
	cfg      config.KnowledgeGlobalConfig
	llmCfg   *llm.EmbeddingConfig
	mu       sync.Mutex
	watcher  *fsnotify.Watcher
	watching map[string]bool                 // workdir → 已启动监听
	pending  map[string]map[string]time.Time // workdir → docID → 最后事件时间
	timers   map[string]*time.Timer          // workdir → 防抖定时器
	inFlight map[string]bool                 // workdir → 正在索引
	stopCh   chan struct{}
	stopped  bool
}

// NewScanner 构造 scanner。
func NewScanner(svc Service, cfg config.KnowledgeGlobalConfig, embCfg *llm.EmbeddingConfig, logger *slog.Logger) *Scanner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scanner{
		svc:      svc,
		logger:   logger,
		cfg:      cfg,
		llmCfg:   embCfg,
		watching: make(map[string]bool),
		pending:  make(map[string]map[string]time.Time),
		timers:   make(map[string]*time.Timer),
		inFlight: make(map[string]bool),
		stopCh:   make(chan struct{}),
	}
}

// Config 返回当前配置（供守护进程热重载后重建 scanner）。
func (s *Scanner) Config() config.KnowledgeGlobalConfig { return s.cfg }

// Start 启动扫描与监听（按配置开关）：
//   - Enabled=false → 不扫描不监听（查询原样可用）；
//   - StartupScan=true → 启动全量扫描已注册文档；
//   - FSNotify=true → watch 已注册文档唯一父目录。
func (s *Scanner) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return nil
	}
	if !s.cfg.EnabledOn() {
		s.logger.Info("knowledge scanner disabled by config")
		return nil
	}
	if s.cfg.StartupScanOn() {
		go s.scanAll(ctx, "startup")
	}
	if s.cfg.FSNotifyOn() {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			return fmt.Errorf("knowledge: new watcher: %w", err)
		}
		s.watcher = w
		go s.watchLoop(ctx)
	}
	return nil
}

// Stop 停止监听与定时器（进程退出/测试清理）。
func (s *Scanner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	close(s.stopCh)
	if s.watcher != nil {
		_ = s.watcher.Close()
	}
	for _, t := range s.timers {
		t.Stop()
	}
	s.timers = nil
}

// Scan 手动触发全量扫描（POST /api/knowledge/scan 端点调用，权限 knowledge.index）。
func (s *Scanner) Scan(ctx context.Context) (ScanStats, error) {
	stats := s.scanAll(ctx, "manual")
	return stats, nil
}

// ScanStats 扫描统计。
type ScanStats struct {
	Total   int `json:"total"`
	Indexed int `json:"indexed"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
	Missing int `json:"missing"`
}

// scanAll 扫描当前项目全部已注册文档（变化检测 + 索引 + 漂移重嵌）。
func (s *Scanner) scanAll(ctx context.Context, trigger string) ScanStats {
	if !s.cfg.EnabledOn() {
		return ScanStats{}
	}
	workdirs := s.listWorkdirs(ctx)
	stats := ScanStats{}
	for _, wd := range workdirs {
		docs, err := s.svc.ListDocuments(ctx, wd, DocumentFilter{Page: 0, Size: maxPageSize})
		if err != nil {
			s.logger.Warn("knowledge: scan list documents failed", "workdir", wd, "err", err)
			continue
		}
		for _, d := range docs.Items {
			stats.Total++
			res, err := s.indexIfChanged(ctx, wd, d, trigger)
			if err != nil {
				stats.Failed++
				s.logger.Warn("knowledge: scan index failed", "doc", d.ID, "err", err)
				continue
			}
			// 无变化（Chunks=0 且未跳过）→ 不计数（已是最新）。
			if res.Chunks == 0 && !res.Skipped && !res.Embedded && !res.Summarized {
				continue
			}
			if res.Skipped {
				stats.Skipped++
			} else {
				stats.Indexed++
			}
		}
	}
	s.logger.Info("knowledge scan complete", "trigger", trigger, "total", stats.Total,
		"indexed", stats.Indexed, "skipped", stats.Skipped, "failed", stats.Failed)
	return stats
}

// listWorkdirs 枚举项目库目录（scanner 独立于全局注册表：遍历扫描期间触发过的 workdir 集合）。
//
// 由于 knowledge Service 不维护项目注册表（分层：注册表在 project 包），scanner 采用
// 「被动注册」：IndexDocument 时登记 workdir，扫描/监听仅针对已登记项目。守护进程启动时
// 由 api/daemon 层调用 RegisterWorkdir 登记全部已导入项目。
func (s *Scanner) listWorkdirs(ctx context.Context) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.watching))
	for wd := range s.watching {
		out = append(out, wd)
	}
	return out
}

// RegisterWorkdir 登记项目（守护进程启动时对所有已导入项目调用；
// 扫描/监听以此为准，避免 scanner 依赖 project 包）。
func (s *Scanner) RegisterWorkdir(workdir string) {
	clean := filepath.Clean(workdir)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.watching[clean] {
		s.watching[clean] = true
		s.logger.Info("knowledge scanner registered workdir", "workdir", clean)
	}
	// fsnotify：watch 该项目内全部已注册文档的父目录。
	s.updateWatchDirs(clean)
}

// UnregisterWorkdir 注销项目（移除项目记录时调用；监听目录自动清理）。
func (s *Scanner) UnregisterWorkdir(workdir string) {
	clean := filepath.Clean(workdir)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.watching, clean)
	delete(s.pending, clean)
	if t, ok := s.timers[clean]; ok {
		t.Stop()
		delete(s.timers, clean)
	}
	delete(s.inFlight, clean)
}

// updateWatchDirs 为指定项目维护 fsnotify watch 集合（仅文档父目录，防止事件风暴）。
func (s *Scanner) updateWatchDirs(workdir string) {
	if s.watcher == nil || !s.cfg.FSNotifyOn() {
		return
	}
	dirs := s.documentDirs(workdir)
	for _, dir := range dirs {
		_ = s.watcher.Add(dir)
	}
}

// documentDirs 列出项目已注册文档的唯一父目录（外部文件目录也纳入；项目内去重）。
func (s *Scanner) documentDirs(workdir string) []string {
	conn, err := openProjectConn(workdir)
	if err != nil {
		return nil
	}
	defer func() { _ = conn.Close() }()
	rows, err := conn.Query(`SELECT abs_path FROM knowledge_documents`)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	set := map[string]bool{}
	for rows.Next() {
		var abs string
		if err := rows.Scan(&abs); err != nil {
			continue
		}
		dir := filepath.Dir(abs)
		set[dir] = true
	}
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	return out
}

// openProjectConn 打开项目库（scanner 内部辅助；不缓存连接）。
func openProjectConn(workdir string) (*sql.DB, error) {
	return db.Open(db.MetaDBPath(workdir))
}

// watchLoop fsnotify 事件循环：按文档合并到防抖窗口，到期批量索引。
func (s *Scanner) watchLoop(ctx context.Context) {
	defer func() { _ = s.watcher.Close() }()
	debounce := time.Duration(s.cfg.DebounceMS) * time.Millisecond
	if debounce <= 0 {
		debounce = 30 * time.Second
	}
	for {
		select {
		case <-s.stopCh:
			return
		case ev, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			s.handleEvent(ctx, ev.Name)
		case _, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
		case <-time.After(debounce):
			s.flushPending(ctx)
		}
	}
}

// handleEvent 记录事件 → 定时防抖。
func (s *Scanner) handleEvent(ctx context.Context, path string) {
	wd, docID := s.locateDoc(path)
	if wd == "" || docID == "" {
		return // 非已注册文档
	}
	debounce := time.Duration(s.cfg.DebounceMS) * time.Millisecond
	if debounce <= 0 {
		debounce = 30 * time.Second
	}
	s.mu.Lock()
	if s.pending[wd] == nil {
		s.pending[wd] = map[string]time.Time{}
	}
	s.pending[wd][docID] = time.Now()
	s.mu.Unlock()
	// 启动/重置定时器（单项目定时器，到期处理该项目全部 pending）。
	s.mu.Lock()
	if t, ok := s.timers[wd]; ok {
		t.Stop()
	}
	s.timers[wd] = time.AfterFunc(debounce, func() {
		s.flushPendingFor(ctx, wd)
	})
	s.mu.Unlock()
}

// locateDoc 通过路径定位 workdir + docID（全表扫描匹配 abs_path）。
func (s *Scanner) locateDoc(path string) (string, string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", ""
	}
	abs = filepath.Clean(abs)
	s.mu.Lock()
	workdirs := make([]string, 0, len(s.watching))
	for wd := range s.watching {
		workdirs = append(workdirs, wd)
	}
	s.mu.Unlock()
	for _, wd := range workdirs {
		conn, err := openProjectConn(wd)
		if err != nil {
			continue
		}
		var id string
		err = conn.QueryRow(`SELECT id FROM knowledge_documents WHERE abs_path = ?`, abs).Scan(&id)
		_ = conn.Close()
		if err == nil && id != "" {
			return wd, id
		}
	}
	return "", ""
}

// flushPending 处理全部项目的 pending 文档（定时兜底）。
func (s *Scanner) flushPending(ctx context.Context) {
	s.mu.Lock()
	workdirs := make([]string, 0, len(s.pending))
	for wd := range s.pending {
		workdirs = append(workdirs, wd)
	}
	s.mu.Unlock()
	for _, wd := range workdirs {
		s.flushPendingFor(ctx, wd)
	}
}

// flushPendingFor 处理单项目 pending 文档（singleflight：同一项目同时只一个索引任务）。
func (s *Scanner) flushPendingFor(ctx context.Context, workdir string) {
	s.mu.Lock()
	if s.inFlight[workdir] {
		s.mu.Unlock()
		return
	}
	s.inFlight[workdir] = true
	docs := make([]string, 0, len(s.pending[workdir]))
	for id := range s.pending[workdir] {
		docs = append(docs, id)
	}
	delete(s.pending, workdir)
	if t, ok := s.timers[workdir]; ok {
		t.Stop()
		delete(s.timers, workdir)
	}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.inFlight, workdir)
		s.mu.Unlock()
	}()
	sort.Strings(docs)
	for _, docID := range docs {
		// 重新读取文档（事件后状态可能已变化）。
		d, err := s.svc.GetDocument(ctx, workdir, docID)
		if err != nil {
			continue
		}
		if _, err := s.indexIfChanged(ctx, workdir, d, "fsnotify"); err != nil {
			s.logger.Warn("knowledge: fsnotify index failed", "doc", docID, "err", err)
		}
	}
}

// indexIfChanged 变化检测（mtime+size 快速比对 → sha256 确认）→ 重建摘要与向量。
func (s *Scanner) indexIfChanged(ctx context.Context, workdir string, d Document, trigger string) (IndexResult, error) {
	fi, err := os.Stat(d.AbsPath)
	if err != nil {
		// 文件缺失：懒校验 → missing（内容/向量保留）。
		s.markMissing(workdir, d.ID)
		return IndexResult{}, nil
	}
	newSize := fi.Size()
	newMTime := fi.ModTime().Format(timeRFC3339)
	// 快速比对：mtime + size。
	if !s.changedByMeta(d, newSize, newMTime) {
		// 从未索引（hash 空）→ 仍需索引（注册时仅记录元数据，未建向量/摘要）。
		if d.ContentHash == "" {
			data, err := os.ReadFile(d.AbsPath)
			if err != nil {
				return IndexResult{}, fmt.Errorf("knowledge: read %s: %w", d.AbsPath, err)
			}
			return s.indexDocument(workdir, d, sha256Hex(data), false)
		}
		// 无变化 → 模型漂移检测（embedding_model 变更 → 全量重嵌）。
		if s.modelDrifted(d) {
			s.logger.Info("knowledge: embedding model drift, re-embedding", "doc", d.ID)
			return s.indexDocument(workdir, d, d.ContentHash, true)
		}
		return IndexResult{Chunks: 0}, nil
	}
	// 慢比对：sha256。
	data, err := os.ReadFile(d.AbsPath)
	if err != nil {
		return IndexResult{}, fmt.Errorf("knowledge: read %s: %w", d.AbsPath, err)
	}
	sum := sha256Hex(data)
	if sum == d.ContentHash && d.Status == DocStatusOK {
		// 内容未变（mtime 变化但 hash 相同）。
		return IndexResult{Chunks: 0}, nil
	}
	return s.indexDocument(workdir, d, sum, false)
}

// changedByMeta mtime + size 快速比对。
func (s *Scanner) changedByMeta(d Document, size int64, mtime string) bool {
	return d.Size != size || d.MTime != mtime
}

// modelDrifted 检测 embedding 模型漂移（已嵌入且模型变更 → 重嵌）。
func (s *Scanner) modelDrifted(d Document) bool {
	if s.llmCfg == nil {
		return false
	}
	return d.Embedded == EmbedYes && d.EmbeddingModel != "" && d.EmbeddingModel != s.llmCfg.Model
}

// indexDocument 执行索引（带 singleflight 语义由调用方保证串行）。
func (s *Scanner) indexDocument(workdir string, d Document, contentHash string, force bool) (IndexResult, error) {
	opts := IndexOptions{
		ContentHash:  contentHash,
		Embedding:    s.llmCfg,
		MaxIndexSize: s.cfg.MaxIndexSize,
		ForceReembed: force,
	}
	return s.svc.IndexDocument(context.Background(), workdir, d.ID, opts)
}

// markMissing 标记文档缺失（懒校验：读取时 stat 失败 → missing 状态，内容/向量保留）。
func (s *Scanner) markMissing(workdir, docID string) {
	_, _ = s.svc.IndexDocument(context.Background(), workdir, docID, IndexOptions{})
}

// sha256Hex 计算内容哈希。
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// RefreshWatchDirs 重新同步全部项目的 watch 目录（注册新文档/relink 后调用）。
func (s *Scanner) RefreshWatchDirs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watcher == nil || !s.cfg.FSNotifyOn() {
		return
	}
	for wd := range s.watching {
		for _, dir := range s.documentDirs(wd) {
			_ = s.watcher.Add(dir)
		}
	}
}
