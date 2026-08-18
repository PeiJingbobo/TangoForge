package parser

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"tangoforge/internal/config"
	"tangoforge/internal/db"
	"tangoforge/internal/knowledge"
	"tangoforge/internal/llm"
	"tangoforge/internal/task"
	"time"

	"github.com/google/uuid"
)

// 业务错误码（docs/TASK-SEMANTICS.md §17.4）。
const (
	CodeImportFailed  = "IMPORT_FAILED"
	CodeDraftNotFound = "DRAFT_NOT_FOUND"
)

// ErrImportFailed LLM 解析失败 / 校验失败（整次失败不落库，Message 含 LLM 原始输出）。
var ErrImportFailed = errors.New("parser: import failed")

// ErrDraftNotFound 草稿不存在。
var ErrDraftNotFound = errors.New("parser: draft not found")

// ErrDraftInvalid 草稿内容校验失败（审阅编辑保存 → 422）。
var ErrDraftInvalid = errors.New("parser: draft invalid")

// ErrProjectNotFound 项目未导入（无 meta.db）。
var ErrProjectNotFound = errors.New("parser: project not found")

// Draft 导入草稿（import_drafts 表）。
type Draft struct {
	ID         string `json:"id"`
	SourceFile string `json:"source_file"`
	Status     string `json:"status"` // pending / confirmed / discarded
	TaskCount  int    `json:"task_count"`
	CreatedAt  string `json:"created_at"`
}

// ConfirmResult 草稿确认结果。
type ConfirmResult struct {
	DraftID    string `json:"draft_id"`
	SourceFile string `json:"source_file"`
	Created    int    `json:"created"`
	Archived   int    `json:"archived"`
	// DroppedDeps 无法解析的依赖引用数（标题被修改/引用失效等），已忽略继续导入。
	DroppedDeps int `json:"dropped_deps"`
	// DroppedKnowledge 路径缺失被跳过的知识库文件数（TF-049，QA-K17：仅警告不阻断）。
	DroppedKnowledge int `json:"dropped_knowledge"`
}

// Options Service 构造选项。
type Options struct {
	Logger *slog.Logger
	// LLM 配置提供者：每次调用获取最新全局配置（支持 LLM 配置热重载）。
	LLM func() config.LLMConfig
	// Tasks 任务业务服务（确认入库复用 ImportTasks 事务接口）。
	Tasks task.Service
	// OnEvent 导入域事件回调（draft_ready / draft_confirmed / draft_discarded / failed）。
	OnEvent func(ctx context.Context, workdir, action, target string)
	// Knowledge 知识库业务服务（TF-049：knowledge_files 关联入库）。
	Knowledge knowledge.Service
}

// Service 导入解析业务服务。
type Service struct {
	mu        sync.Mutex
	dbs       map[string]*sql.DB
	fp        map[string]*db.FileFingerprint // workdir → meta.db 文件指纹（TF-001 删除重建校验）
	logger    *slog.Logger
	llmCfg    func() config.LLMConfig
	llmClient *llm.Client // 知识库摘要用（懒构造；nil = 摘要降级）
	tasks     task.Service
	knowledge knowledge.Service
	onEvent   func(ctx context.Context, workdir, action, target string)
}

// NewService 构造解析服务。
func NewService(opts Options) *Service {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.LLM == nil {
		opts.LLM = func() config.LLMConfig { return config.DefaultLLMConfig() }
	}
	return &Service{
		dbs:       make(map[string]*sql.DB),
		fp:        make(map[string]*db.FileFingerprint),
		logger:    opts.Logger,
		llmCfg:    opts.LLM,
		tasks:     opts.Tasks,
		knowledge: opts.Knowledge,
		onEvent:   opts.OnEvent,
	}
}

// projectDB 打开并缓存项目库连接（语义同 task.Service.projectDB）。
// TF-001 修复：缓存命中校验 meta.db 文件指纹，删除重建后重开连接。
func (s *Service) projectDB(workdir string) (*sql.DB, error) {
	clean := filepath.Clean(workdir)
	if !filepath.IsAbs(clean) {
		return nil, fmt.Errorf("%w: %s 不是绝对路径", ErrProjectNotFound, workdir)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if conn, ok := s.dbs[clean]; ok {
		if s.fp[clean].SameAs(db.MetaDBPath(clean)) {
			return conn, nil
		}
		s.logger.Warn("project db file replaced, reopening", "workdir", clean, "path", db.MetaDBPath(clean))
		_ = conn.Close()
		delete(s.dbs, clean)
		delete(s.fp, clean)
	}
	if _, err := os.Stat(db.MetaDBPath(clean)); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProjectNotFound, workdir)
	}
	fp, err := db.CaptureFingerprint(db.MetaDBPath(clean))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProjectNotFound, workdir)
	}
	conn, err := db.EnsureProject(context.Background(), db.MetaDBPath(clean))
	if err != nil {
		return nil, fmt.Errorf("parser: open project db %s: %w", clean, err)
	}
	s.dbs[clean] = conn
	s.fp[clean] = fp
	return conn, nil
}

// Close 关闭全部缓存的项目库连接。
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for wd, conn := range s.dbs {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("parser: close %s: %w", wd, err)
		}
		delete(s.dbs, wd)
		delete(s.fp, wd)
	}
	return firstErr
}

// emit 触发导入域事件（nil 安全；audit + WS 由调用方注入双通道）。
func (s *Service) emit(ctx context.Context, workdir, action, target string) {
	if s.onEvent != nil {
		s.onEvent(ctx, workdir, action, target)
	}
}

// ParseInput 导入入参（QA P4-1 扩展：file_path / file_paths / directory / content 四形态）。
// 多文件（file_paths）与目录（directory）会合并为**一次 LLM 解析**，产出单一草稿。
type ParseInput struct {
	FilePath   string   `json:"file_path"`  // 单文件（相对 workdir 或绝对）
	FilePaths  []string `json:"file_paths"` // 多文件：合并解析（source_file=公共父目录）
	Directory  string   `json:"directory"`  // 目录：递归扫描 *.md/*.markdown 后合并解析（source_file=目录）
	Content    string   `json:"content"`    // 原始内容（须配 source_file）
	SourceFile string   `json:"source_file"`
	// Knowledge 知识库候选（TF-049，QA-K11 扩展草稿流）：候选文档树 + 摘要注入 prompt。
	Knowledge *KnowledgeInput `json:"knowledge,omitempty"`
}

// Parse 解析 Markdown 并生成草稿（QA P4-1 §17.1）。
//
//   - LLM 调用 → 严格 JSON Schema → 校验/规范化（title/status 必填、priority 归一化、status 映射）→
//     写 import_drafts（pending）；
//   - 成功 → 事件 import.draft_ready；任一环节失败 → 整次失败不落库，
//     返回 IMPORT_FAILED（含 LLM 原始输出），事件 import.failed。
func (s *Service) Parse(ctx context.Context, workdir string, in ParseInput) (Draft, error) {
	draft, err := s.parseCore(ctx, workdir, in)
	if err != nil {
		s.logger.Warn("import parse failed", "workdir", workdir, "err", err)
		s.emit(ctx, workdir, "import.failed", "")
		return Draft{}, err
	}
	s.emit(ctx, workdir, "import.draft_ready", draft.ID)
	return draft, nil
}

// parseCore Parse 主体（不含事件）。
func (s *Service) parseCore(ctx context.Context, workdir string, in ParseInput) (Draft, error) {
	// 入参解析：file_path / file_paths / directory / content 四形态。
	var content, sourceFile string
	var err error
	switch {
	case in.Directory != "":
		dir := filepath.Clean(in.Directory)
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(workdir, dir)
		}
		files, err := scanMarkdownFiles(dir)
		if err != nil {
			return Draft{}, fmt.Errorf("%w: %v", ErrImportFailed, err)
		}
		content, sourceFile, err = mergeFiles(workdir, files)
		if err != nil {
			return Draft{}, fmt.Errorf("%w: %v", ErrImportFailed, err)
		}
	case len(in.FilePaths) > 0:
		content, sourceFile, err = mergeFiles(workdir, in.FilePaths)
		if err != nil {
			return Draft{}, fmt.Errorf("%w: %v", ErrImportFailed, err)
		}
	case in.FilePath != "":
		path := filepath.Clean(in.FilePath)
		if !filepath.IsAbs(path) {
			path = filepath.Join(workdir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return Draft{}, fmt.Errorf("%w: 读取文件 %s: %v", ErrImportFailed, path, err)
		}
		content = string(data)
		sourceFile = path
	case in.Content != "" || in.SourceFile != "":
		if strings.TrimSpace(in.SourceFile) == "" {
			return Draft{}, fmt.Errorf("%w: content 方式必须提供 source_file", ErrImportFailed)
		}
		content = in.Content
		sourceFile = filepath.Clean(in.SourceFile)
	default:
		return Draft{}, fmt.Errorf("%w: 必须提供 file_path / file_paths / directory / content 之一", ErrImportFailed)
	}
	if strings.TrimSpace(content) == "" {
		return Draft{}, fmt.Errorf("%w: Markdown 内容为空", ErrImportFailed)
	}

	// 状态机注入。
	sm, err := s.loadStateMachine(workdir)
	if err != nil {
		return Draft{}, err
	}

	// 知识库候选（TF-049，QA-K4：结构 + 预生成摘要，不拼全文）。
	knowledgeTree := ""
	if in.Knowledge != nil {
		tree, kerr := s.buildKnowledgeTree(workdir, in.Knowledge)
		if kerr != nil {
			return Draft{}, fmt.Errorf("%w: %v", ErrImportFailed, kerr)
		}
		knowledgeTree = tree
	}

	// LLM 调用（客户端断开不取消 LLM 请求：用 WithoutCancel 脱离 r.Context()，
	// 超时由 llm.Client 的 http.Timeout 控制；修复 CLI 超时断开导致的 context canceled，2026-08-06）。
	client, err := llm.New(llm.FromConfig(s.llmCfg()), s.logger)
	if err != nil {
		return Draft{}, err // LLM_NOT_CONFIGURED
	}
	raw, err := client.CompleteJSON(context.WithoutCancel(ctx), llm.Request{
		System:      buildSystemPrompt(),
		User:        buildUserPrompt(sm, content, knowledgeTree),
		RequireJSON: true,
		Schema:      buildJSONSchema(),
	})
	if err != nil {
		return Draft{}, fmt.Errorf("%w: LLM 解析失败: %v（原始输出见 detail）", ErrImportFailed, err)
	}
	llmRaw := string(raw)

	// 结构解析 + 校验规范化。
	parsed, err := s.normalizeOutput(sm, raw)
	if err != nil {
		return Draft{}, fmt.Errorf("%w: %v\nLLM 原始输出：%s", ErrImportFailed, err, llmRaw)
	}
	parsedJSON, _ := json.Marshal(parsed)
	if len(parsed.Tasks) == 0 {
		return Draft{}, fmt.Errorf("%w: 未解析出任何任务\nLLM 原始输出：%s", ErrImportFailed, llmRaw)
	}
	// knowledge_files 校验（TF-049）：path 必填；kb 名合法性留 confirm 校验
	// （因需查库名，这里仅做结构校验：path 空 → 整次失败）。
	for i, kf := range parsed.KnowledgeFiles {
		if strings.TrimSpace(kf.Path) == "" {
			return Draft{}, fmt.Errorf("%w: knowledge_files[%d] 缺少 path\nLLM 原始输出：%s",
				ErrImportFailed, i, llmRaw)
		}
	}

	// 写草稿。
	conn, err := s.projectDB(workdir)
	if err != nil {
		return Draft{}, err
	}
	id := uuid.NewString()
	now := time.Now().Format(time.RFC3339)
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO import_drafts (id, project_id, source_file, parsed_json, status, created_at)
		 VALUES (?, ?, ?, ?, 'pending', ?)`,
		id, 1, sourceFile, string(parsedJSON), now); err != nil {
		return Draft{}, fmt.Errorf("parser: insert draft: %w", err)
	}
	draft := Draft{ID: id, SourceFile: sourceFile, Status: "pending", TaskCount: countTasks(parsed.Tasks), CreatedAt: now}
	s.logger.Info("import draft ready", "id", id, "source", sourceFile, "tasks", draft.TaskCount)
	return draft, nil
}

// countTasks 递归统计任务总数（含嵌套子任务）。
func countTasks(tasks []ParsedTask) int {
	n := len(tasks)
	for _, t := range tasks {
		n += countTasks(t.Children)
	}
	return n
}

// List 返回 pending 草稿列表（ts 倒序）。
func (s *Service) List(ctx context.Context, workdir string) ([]Draft, error) {
	conn, err := s.projectDB(workdir)
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx,
		`SELECT id, source_file, status, parsed_json, created_at FROM import_drafts
		 WHERE status = 'pending' ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("parser: list drafts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Draft, 0)
	for rows.Next() {
		var d Draft
		var parsed string
		if err := rows.Scan(&d.ID, &d.SourceFile, &d.Status, &parsed, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("parser: scan draft: %w", err)
		}
		var pr ParseResult
		if err := json.Unmarshal([]byte(parsed), &pr); err == nil {
			d.TaskCount = len(pr.Tasks)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Confirm 确认草稿入库：展平 → 依赖解析 → task.ImportTasks（文件级全量覆盖原子）→ 草稿置 confirmed。
func (s *Service) Confirm(ctx context.Context, workdir, draftID string) (ConfirmResult, error) {
	conn, err := s.projectDB(workdir)
	if err != nil {
		return ConfirmResult{}, err
	}
	var parsedJSON string
	err = conn.QueryRowContext(ctx,
		`SELECT parsed_json FROM import_drafts WHERE id = ? AND status = 'pending'`, draftID).Scan(&parsedJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ConfirmResult{}, fmt.Errorf("%w: %s", ErrDraftNotFound, draftID)
	}
	if err != nil {
		return ConfirmResult{}, fmt.Errorf("parser: query draft: %w", err)
	}
	var pr ParseResult
	if err := json.Unmarshal([]byte(parsedJSON), &pr); err != nil {
		return ConfirmResult{}, fmt.Errorf("%w: 草稿数据损坏: %v", ErrImportFailed, err)
	}

	// TF-054 状态机合并（方式 2）：文档状态机与项目状态机对比，缺失状态确认导入时自动并入，
	// 使文档原始状态（如 BLOCKED / NOT_STARTED）在任务入库后可用，避免状态被抹平或导入失败。
	mergedSM, err := s.mergeDocumentStatuses(ctx, workdir, pr.DocumentStatuses)
	if err != nil {
		return ConfirmResult{}, fmt.Errorf("%w: 状态机合并失败: %v", ErrImportFailed, err)
	}
	_ = mergedSM

	// 展平 + 依赖解析（§17.3）：先补齐临时 ID（旧草稿兼容），再按 ID/标题双索引解析。
	pr.Tasks = ensureTaskIDs(pr.Tasks)
	flattened, err := flattenTasks(pr.Tasks, "")
	if err != nil {
		return ConfirmResult{}, fmt.Errorf("%w: %v", ErrImportFailed, err)
	}
	depIDs, dropped, err := resolveDependsOn(flattened)
	if err != nil {
		return ConfirmResult{}, fmt.Errorf("%w: %v", ErrImportFailed, err)
	}
	tasks := make([]task.Task, 0, len(flattened))
	for _, f := range flattened {
		t := task.Task{
			ID:            f.ID,
			Number:        f.Number,
			Title:         f.Title,
			Description:   f.Description,
			Status:        f.Status,
			Priority:      f.Priority,
			Tags:          f.Tags,
			Assignee:      f.Assignee,
			DependsOn:     depIDs[f.ID],
			ParentID:      f.ParentID,
			SourceFile:    "", // ImportTasks 统一写入 sourceFile
			SourceSection: f.Section,
		}
		tasks = append(tasks, t)
	}

	var sourceFile string
	if err := conn.QueryRowContext(ctx,
		`SELECT source_file FROM import_drafts WHERE id = ?`, draftID).Scan(&sourceFile); err != nil {
		return ConfirmResult{}, fmt.Errorf("parser: query source_file: %w", err)
	}

	res, err := s.tasks.ImportTasks(ctx, workdir, sourceFile, tasks)
	if err != nil {
		return ConfirmResult{}, err
	}

	// TF-049：knowledge_files 关联入库（草稿生成的全部任务 + LLM 建议文档）。
	droppedKnowledge := 0
	if s.knowledge != nil && len(pr.KnowledgeFiles) > 0 {
		taskIDs := make([]string, 0, len(tasks))
		for _, t := range tasks {
			taskIDs = append(taskIDs, t.ID)
		}
		kres, kerr := s.knowledge.LinkFiles(ctx, workdir, taskIDs, pr.KnowledgeFiles, "auto")
		if kerr != nil {
			// 库名不存在等硬错误 → 整次导入失败（QA-K11）；任务已入库但关联失败——
			// 返回错误由调用方处理（任务已落库，关联缺失，符合「宁可报错不可臆断」）。
			return ConfirmResult{}, fmt.Errorf("%w: knowledge_files 关联失败: %v", ErrImportFailed, kerr)
		}
		droppedKnowledge = kres.Dropped
	}

	// 草稿置 confirmed（仅 pending 可转，并发安全）。
	now := time.Now().Format(time.RFC3339)
	if _, err := conn.ExecContext(ctx,
		`UPDATE import_drafts SET status = 'confirmed', confirmed_at = ? WHERE id = ? AND status = 'pending'`,
		now, draftID); err != nil {
		return ConfirmResult{}, fmt.Errorf("parser: confirm draft: %w", err)
	}
	if dropped > 0 {
		s.logger.Warn("import draft confirmed with dropped deps",
			"id", draftID, "dropped", dropped)
	}
	s.emit(ctx, workdir, "import.draft_confirmed", draftID)
	return ConfirmResult{
		DraftID:          draftID,
		SourceFile:       sourceFile,
		Created:          res.Created,
		Archived:         res.Archived,
		DroppedDeps:      dropped,
		DroppedKnowledge: droppedKnowledge,
	}, nil
}

// Discard 丢弃草稿（仅 pending → discarded）。
func (s *Service) Discard(ctx context.Context, workdir, draftID string) error {
	conn, err := s.projectDB(workdir)
	if err != nil {
		return err
	}
	res, err := conn.ExecContext(ctx,
		`UPDATE import_drafts SET status = 'discarded' WHERE id = ? AND status = 'pending'`, draftID)
	if err != nil {
		return fmt.Errorf("parser: discard draft: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrDraftNotFound, draftID)
	}
	s.logger.Info("import draft discarded", "id", draftID)
	s.emit(ctx, workdir, "import.draft_discarded", draftID)
	return nil
}

// mergeDocumentStatuses 状态机合并（TF-054 方式 2）：将文档原始状态并入项目状态机。
//
// 策略：对文档状态做语义归一（英文/中文 → 状态机 key 风格），
// 凡项目状态机不存在的 key 自动追加为状态（label=原文，颜色取默认），
// 并补充 transitions：todo→新状态、新状态→doing、新状态→done（保持可达可流转）。
// 返回合并后的状态机；无差异时原样返回（不落盘、不触发事件）。
//
// 注意：仅追加不删除——不触碰用户既有状态机定义，符合「删除=需人工确认」的约束；
// 合并本身是导入确认流程的一部分（用户已确认草稿），落盘经 task.UpdateStateMachine
// （含编辑校验 + STATUS_IN_USE 占用校验 + state_machine.changed 事件/审计）。
func (s *Service) mergeDocumentStatuses(ctx context.Context, workdir string, docStatuses []string) (config.StateMachine, error) {
	if len(docStatuses) == 0 {
		sm, err := s.loadStateMachine(workdir)
		if err != nil {
			return config.StateMachine{}, err
		}
		return sm, nil
	}
	sm, err := s.loadStateMachine(workdir)
	if err != nil {
		return config.StateMachine{}, err
	}
	existing := make(map[string]bool, len(sm.States))
	for _, st := range sm.States {
		existing[st.Key] = true
	}
	// 收集缺失状态：语义归一后 key 唯一、非 archived、非系统保留态。
	missing := make([]config.State, 0, len(docStatuses))
	seen := make(map[string]bool)
	for _, raw := range docStatuses {
		raw = strings.TrimSpace(raw)
		if raw == "" || raw == task.StatusArchived {
			continue
		}
		key := semanticStatusKey(raw)
		if key == "" || key == task.StatusArchived || existing[key] || seen[key] {
			continue
		}
		seen[key] = true
		missing = append(missing, config.State{
			Key:   key,
			Label: raw, // label 保留文档原文，便于看板识别来源
			Color: defaultStateColor(len(sm.States) + len(missing)),
		})
	}
	if len(missing) == 0 {
		return sm, nil
	}
	// 追加状态 + 补充流转：todo→新状态、新状态→doing/done、doing→新状态、done→新状态。
	sm.States = append(sm.States, missing...)
	toAll := make([]string, 0, len(missing))
	for _, st := range missing {
		toAll = append(toAll, st.Key)
	}
	sm.Transitions = appendTransitions(sm.Transitions,
		config.Transition{From: "todo", To: toAll},
		config.Transition{From: "doing", To: toAll},
		config.Transition{From: "done", To: toAll},
	)
	for _, st := range missing {
		sm.Transitions = appendTransitions(sm.Transitions,
			config.Transition{From: st.Key, To: []string{"doing", "done"}},
		)
	}
	// 经 task.UpdateStateMachine 持久化（校验 + 审计 + 事件）。
	norm, err := s.tasks.UpdateStateMachine(ctx, workdir, sm)
	if err != nil {
		return config.StateMachine{}, err
	}
	s.logger.Info("import state machine merged",
		"workdir", workdir, "added", len(missing), "states", keysOf(norm.States))
	return norm, nil
}

// semanticStatusKey 文档原始状态措辞 → 状态机 key（语义归一；与 mapStatus 的语义组一致）。
func semanticStatusKey(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	// 常见措辞 → 目标 key（英文/中文）。两轮：先完全相等，再长别名子串包含。
	groups := []struct {
		aliases []string
		key     string
	}{
		{[]string{"done", "completed", "complete", "finished", "closed", "accepted", "已完成", "完成", "已验收", "验收通过", "released"}, "done"},
		{[]string{"doing", "in_progress", "in progress", "inprogress", "working", "started", "wip", "进行中", "执行中", "处理中", "开发中", "blocked", "blocking", "on hold", "on_hold", "stuck", "阻塞", "被阻塞", "卡住"}, "doing"},
		{[]string{"todo", "not_started", "not started", "notstarted", "pending", "planned", "backlog", "open", "待办", "未开始", "未启动", "计划中", "新建"}, "todo"},
		{[]string{"verifying", "ready_for_acceptance", "ready for acceptance", "rfa", "review", "in_review", "in review", "qa", "待验收", "待核验", "待验证", "验收中", "核验中", "待审查"}, "verifying"},
	}
	for _, g := range groups {
		for _, a := range g.aliases {
			if lower == a {
				return g.key
			}
		}
	}
	for _, g := range groups {
		for _, a := range g.aliases {
			if len(a) >= 6 && strings.Contains(lower, a) {
				return g.key
			}
		}
	}
	// 无法语义归一：按原文规范化成 key（小写、空白→下划线、去特殊字符）。
	key := strings.ToLower(strings.TrimSpace(raw))
	key = regexp.MustCompile(`[^a-z0-9_\-]+`).ReplaceAllString(key, "_")
	key = strings.Trim(key, "_")
	if key == "" || key == "archived" {
		return ""
	}
	return key
}

// appendTransitions 追加流转规则（from 已存在时合并 to，去重）。
func appendTransitions(list []config.Transition, adds ...config.Transition) []config.Transition {
	out := list
	for _, a := range adds {
		found := false
		for i := range out {
			if out[i].From == a.From {
				out[i].To = mergeStrings(out[i].To, a.To)
				found = true
				break
			}
		}
		if !found {
			out = append(out, a)
		}
	}
	return out
}

// mergeStrings 合并去重（保序）。
func mergeStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, s := range b {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// keysOf 提取状态 key 列表（日志用）。
func keysOf(states []config.State) []string {
	out := make([]string, 0, len(states))
	for _, st := range states {
		out = append(out, st.Key)
	}
	return out
}

// defaultStateColor 默认状态颜色（新增状态按序号取色板，保证与既有状态视觉可区分）。
func defaultStateColor(i int) string {
	palette := []string{"#8b5cf6", "#ec4899", "#14b8a6", "#f97316", "#84cc16", "#06b6d4", "#a855f7", "#f43f5e"}
	return palette[i%len(palette)]
}

// DraftDetail 草稿明细（审阅界面数据源）：含完整解析任务树。
type DraftDetail struct {
	Draft
	Tasks []ParsedTask `json:"tasks"`
	// KnowledgeFiles LLM 建议关联的知识库文件（TF-049；草稿审阅展示/勾选）。
	KnowledgeFiles []knowledge.KnowledgeFile `json:"knowledge_files,omitempty"`
	// DocumentStatuses 文档原始状态集合（TF-054；确认导入时并入项目状态机）。
	DocumentStatuses []string `json:"document_statuses,omitempty"`
}

// Get 读取单个 pending 草稿明细（含任务树；供审阅/编辑）。
func (s *Service) Get(ctx context.Context, workdir, draftID string) (DraftDetail, error) {
	conn, err := s.projectDB(workdir)
	if err != nil {
		return DraftDetail{}, err
	}
	var d DraftDetail
	var parsed string
	err = conn.QueryRowContext(ctx,
		`SELECT id, source_file, status, parsed_json, created_at FROM import_drafts WHERE id = ?`,
		draftID).Scan(&d.ID, &d.SourceFile, &d.Status, &parsed, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DraftDetail{}, fmt.Errorf("%w: %s", ErrDraftNotFound, draftID)
		}
		return DraftDetail{}, fmt.Errorf("parser: get draft: %w", err)
	}
	if d.Status != "pending" {
		return DraftDetail{}, fmt.Errorf("%w: %s（status=%s）", ErrDraftNotFound, draftID, d.Status)
	}
	// 统一按 ParseResult 解析（tasks + knowledge_files；兼容旧存储直接 tasks 数组）。
	var pr ParseResult
	if err := json.Unmarshal([]byte(parsed), &pr); err == nil && len(pr.Tasks) > 0 {
		d.Tasks = pr.Tasks
		d.KnowledgeFiles = pr.KnowledgeFiles
		d.DocumentStatuses = pr.DocumentStatuses
	} else {
		if err := json.Unmarshal([]byte(parsed), &d.Tasks); err != nil {
			return DraftDetail{}, fmt.Errorf("parser: parse draft json: %w", err)
		}
	}
	// 补齐临时 ID（旧草稿兼容），保证前端依赖匹配与二次编辑回写以 ID 为键。
	d.Tasks = ensureTaskIDs(d.Tasks)
	d.TaskCount = countTasks(d.Tasks)
	return d, nil
}

// UpdateTasks 整体更新草稿任务树（审阅编辑保存）：补齐临时 ID → 校验 → 重写 parsed_json。
func (s *Service) UpdateTasks(ctx context.Context, workdir, draftID string, tasks []ParsedTask) error {
	sm, err := s.loadStateMachine(workdir)
	if err != nil {
		return err
	}
	tasks = ensureTaskIDs(tasks)
	if err := validateParsedTasks(sm, tasks); err != nil {
		return fmt.Errorf("%w: %v", ErrDraftInvalid, err)
	}
	data, err := json.Marshal(ParseResult{Tasks: tasks})
	if err != nil {
		return fmt.Errorf("parser: marshal draft tasks: %w", err)
	}
	conn, err := s.projectDB(workdir)
	if err != nil {
		return err
	}
	res, err := conn.ExecContext(ctx,
		`UPDATE import_drafts SET parsed_json = ? WHERE id = ? AND status = 'pending'`,
		string(data), draftID)
	if err != nil {
		return fmt.Errorf("parser: update draft tasks: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrDraftNotFound, draftID)
	}
	s.logger.Info("import draft tasks updated", "id", draftID, "count", countTasks(tasks))
	return nil
}

// validateParsedTasks 草稿任务树校验（title/status 状态机/priority 范围，递归）。
func validateParsedTasks(sm config.StateMachine, tasks []ParsedTask) error {
	// 字段校验 + 临时 ID 唯一性（结构性约束）。
	// 依赖引用存在性**不校验**（宽容）：标题引用在用户修改标题后可能失效，属草稿中间态，
	// 由确认导入时 resolveDependsOn 宽容跳过 + dropped 提示处理。
	idSeen := make(map[string]bool)
	var collect func(list []ParsedTask) error
	collect = func(list []ParsedTask) error {
		for i, t := range list {
			if strings.TrimSpace(t.Title) == "" {
				return fmt.Errorf("第 %d 项缺少 title", i+1)
			}
			status, ok := mapStatus(sm, t.Status)
			if !ok || status == "archived" {
				return fmt.Errorf("第 %d 项 status 非法或不在状态机中: %v", i+1, t.Status)
			}
			prio, err := task.NormalizePriority(t.Priority)
			if err != nil {
				return fmt.Errorf("第 %d 项 priority 非法: %v", i+1, err)
			}
			if prio != t.Priority {
				// 归一化不一致视为非法（已归一化数据不允许回退）
				return fmt.Errorf("第 %d 项 priority 非法: %v", i+1, t.Priority)
			}
			if t.ID != "" {
				if idSeen[t.ID] {
					return fmt.Errorf("草稿任务 ID 不唯一: %q", t.ID)
				}
				idSeen[t.ID] = true
			}
			if err := collect(t.Children); err != nil {
				return err
			}
		}
		return nil
	}
	return collect(tasks)
}

// loadStateMachine 读取项目状态机（缺失回退默认四态）。
func (s *Service) loadStateMachine(workdir string) (config.StateMachine, error) {
	cfg, err := config.LoadProject(workdir)
	if err != nil {
		return config.StateMachine{}, fmt.Errorf("parser: load state machine %s: %w", workdir, err)
	}
	return cfg.StateMachine, nil
}

// normalizeOutput 校验并规范化 LLM 输出（title/status 必填、priority 归一化、status 映射、tags 归一化）。
func (s *Service) normalizeOutput(sm config.StateMachine, raw json.RawMessage) (ParseResult, error) {
	var out rawParseOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return ParseResult{}, fmt.Errorf("JSON 结构不合规: %v", err)
	}
	counter := 0
	seen := make(map[string]bool)
	normalized, err := s.normalizeTasks(sm, out.Tasks, &counter, seen)
	if err != nil {
		return ParseResult{}, err
	}
	// knowledge_files 规范化（去空白；空 path 保留由 parseCore 整次失败校验）。
	kfs := make([]knowledge.KnowledgeFile, 0, len(out.KnowledgeFiles))
	for _, kf := range out.KnowledgeFiles {
		kfs = append(kfs, knowledge.KnowledgeFile{
			Path:   strings.TrimSpace(kf.Path),
			KB:     strings.TrimSpace(kf.KB),
			Reason: strings.TrimSpace(kf.Reason),
		})
	}
	// document_statuses 规范化：去空白去重（TF-054 状态机适配，LLM 识别文档原始状态）。
	docStatuses := make([]string, 0, len(out.DocumentStatuses))
	seenStatus := make(map[string]bool)
	for _, st := range out.DocumentStatuses {
		st = strings.TrimSpace(st)
		if st == "" || seenStatus[st] {
			continue
		}
		seenStatus[st] = true
		docStatuses = append(docStatuses, st)
	}
	return ParseResult{Tasks: normalized, KnowledgeFiles: kfs, DocumentStatuses: docStatuses}, nil
}

// normalizeTasks 递归校验规范化任务树。
// counter/seen 跨递归共享：为每个任务分配草稿内唯一临时 ID——
// LLM 给出且不重复 → 保留；缺失/重复 → 按遍历顺序自动补 T{n}（依赖引用不受标题变更影响）。
func (s *Service) normalizeTasks(sm config.StateMachine, raw []rawTask, counter *int, seen map[string]bool) ([]ParsedTask, error) {
	out := make([]ParsedTask, 0, len(raw))
	for i, rt := range raw {
		title := strings.TrimSpace(rt.Title)
		if title == "" {
			return nil, fmt.Errorf("第 %d 项缺少 title", i+1)
		}
		// 状态解析：空 status（LLM 对章节/里程碑父节点的常见省略）允许——有 children 时
		// 由子任务推断；无 children 仍必须给出合法状态（避免把"无状态的任务"当叶子静默入库）。
		var status string
		if rawStatusIsEmpty(rt.Status) {
			if len(rt.Children) == 0 {
				return nil, fmt.Errorf("第 %d 项 status 非法或不在状态机中: %v", i+1, rt.Status)
			}
			status = "" // 待推断
		} else {
			mapped, ok := mapStatus(sm, rt.Status)
			if !ok || mapped == "archived" {
				return nil, fmt.Errorf("第 %d 项 status 非法或不在状态机中: %v", i+1, rt.Status)
			}
			status = mapped
		}
		prio, err := normalizePriority(rt.Priority)
		if err != nil {
			return nil, fmt.Errorf("第 %d 项 priority 非法: %v", i+1, err)
		}
		id := strings.TrimSpace(rt.ID)
		if id == "" || seen[id] {
			*counter++
			id = fmt.Sprintf("T%d", *counter)
		}
		seen[id] = true
		children, err := s.normalizeTasks(sm, rt.Children, counter, seen)
		if err != nil {
			return nil, err
		}
		// 父任务状态推断（TF-054 层级保留）：章节/里程碑父节点若 LLM 未给明确状态，
		// 由子任务推断——全部 done → done；存在 doing/verifying → doing；否则 todo。
		if status == "" && len(children) > 0 {
			status = inferParentStatus(status, children)
		}
		// 里程碑标签（TF-054）：父任务标题前缀（如 "M1：工程底座" / "M2 数据闭环"）提取为
		// 里程碑标识，注入本任务及其全部后代 tags，便于筛选与索引。
		milestone := milestoneOf(title)
		tags := normalizeTags(rt.Tags)
		if milestone != "" {
			tags = ensureMilestoneTag(tags, milestone)
		}
		out = append(out, ParsedTask{
			ID:          id,
			Number:      strings.TrimSpace(rt.Number),
			Title:       title,
			Description: rt.Description,
			Status:      status,
			Priority:    prio,
			Tags:        tags,
			Assignee:    strings.TrimSpace(rt.Assignee),
			DependsOn:   cleanStrings(rt.DependsOn),
			Children:    injectMilestoneTags(children, milestone),
		})
	}
	return out, nil
}

// rawStatusIsEmpty 判断 LLM 原始 status 是否为空（nil / 空串 / 纯空白）。
func rawStatusIsEmpty(raw any) bool {
	if raw == nil {
		return true
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}

// inferParentStatus 由子任务状态推断父任务状态（章节/里程碑节点）。
func inferParentStatus(_ string, children []ParsedTask) string {
	hasDone, hasActive := false, false
	var walk func(list []ParsedTask)
	walk = func(list []ParsedTask) {
		for _, c := range list {
			switch c.Status {
			case "done":
				hasDone = true
			case "doing", "verifying":
				hasActive = true
			}
			walk(c.Children)
		}
	}
	walk(children)
	switch {
	case hasActive:
		return "doing"
	case hasDone:
		return "done"
	default:
		return "todo"
	}
}

// milestoneOf 从章节/任务标题提取里程碑标识（如 "M1：xxx" → "M1"、"M2 xxx" → "M2"）。
func milestoneOf(title string) string {
	t := strings.TrimSpace(title)
	// 匹配行首里程碑前缀：字母 M + 数字（可带 -），后接分隔符（：:、空白、-）。
	re := regexp.MustCompile(`^(M\d+[-]?\d*)\s*[：:、\-\s]`)
	if m := re.FindStringSubmatch(t); m != nil {
		return m[1]
	}
	return ""
}

// injectMilestoneTags 将父任务里程碑标识注入全部后代任务 tags（大小写不敏感去重）。
func injectMilestoneTags(children []ParsedTask, milestone string) []ParsedTask {
	if milestone == "" || len(children) == 0 {
		return children
	}
	for i := range children {
		children[i].Tags = ensureMilestoneTag(children[i].Tags, milestone)
		children[i].Children = injectMilestoneTags(children[i].Children, milestone)
	}
	return children
}

// ensureMilestoneTag 注入里程碑标签：已存在（大小写不敏感）则不重复追加，
// 并移除旧的小写/大写变体，统一为里程碑规范形式（如 M1）。
func ensureMilestoneTag(tags []string, milestone string) []string {
	out := make([]string, 0, len(tags)+1)
	seen := make(map[string]bool, len(tags)+1)
	for _, t := range tags {
		if strings.EqualFold(t, milestone) {
			continue // 旧变体由下方统一写回规范形式
		}
		low := strings.ToLower(t)
		if seen[low] {
			continue
		}
		seen[low] = true
		out = append(out, t)
	}
	out = append(out, milestone)
	return out
}

// ensureTaskIDs 递归补齐草稿任务临时 ID（旧草稿/外部编辑兼容）：
// 保留已有 id，缺失或重复按遍历顺序补 T{n}。
func ensureTaskIDs(tasks []ParsedTask) []ParsedTask {
	counter := 0
	seen := make(map[string]bool)
	var walk func(list []ParsedTask) []ParsedTask
	walk = func(list []ParsedTask) []ParsedTask {
		out := make([]ParsedTask, 0, len(list))
		for _, t := range list {
			id := strings.TrimSpace(t.ID)
			if id == "" || seen[id] {
				counter++
				id = fmt.Sprintf("T%d", counter)
			}
			seen[id] = true
			t.ID = id
			t.Children = walk(t.Children)
			out = append(out, t)
		}
		return out
	}
	return walk(tasks)
}

// normalizePriority 归一化优先级（委托 task.NormalizePriority，语义一致 §3）。
func normalizePriority(raw any) (int, error) {
	return task.NormalizePriority(raw)
}

// cleanStrings 去空串保序。
func cleanStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

// flattenTasks 递归展平嵌套任务（深度优先），生成 UUID、parent_id 与 section 路径（如 1.2.3）。
func flattenTasks(tasks []ParsedTask, prefix string) ([]flattenResult, error) {
	var out []flattenResult
	for i, t := range tasks {
		section := fmt.Sprintf("%d", i+1)
		if prefix != "" {
			section = prefix + "." + section
		}
		out = append(out, flattenResult{
			RefID:       t.ID,
			ID:          uuid.NewString(),
			Number:      strings.TrimSpace(t.Number),
			Title:       t.Title,
			Description: t.Description,
			Status:      t.Status,
			Priority:    t.Priority,
			Tags:        t.Tags,
			Assignee:    t.Assignee,
			DependsOn:   t.DependsOn,
			Section:     section,
		})
		// children 展平，parent 指向本任务 ID。
		if len(t.Children) > 0 {
			childID := out[len(out)-1].ID
			children, err := flattenTasks(t.Children, section)
			if err != nil {
				return nil, err
			}
			for j := range children {
				pid := childID
				children[j].ParentID = &pid
			}
			out = append(out, children...)
		}
	}
	return out, nil
}
