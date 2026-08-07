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
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"tangoforge/internal/config"
	"tangoforge/internal/db"
	"tangoforge/internal/llm"
	"tangoforge/internal/task"
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
}

// Service 导入解析业务服务。
type Service struct {
	mu      sync.Mutex
	dbs     map[string]*sql.DB
	logger  *slog.Logger
	llmCfg  func() config.LLMConfig
	tasks   task.Service
	onEvent func(ctx context.Context, workdir, action, target string)
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
		dbs:     make(map[string]*sql.DB),
		logger:  opts.Logger,
		llmCfg:  opts.LLM,
		tasks:   opts.Tasks,
		onEvent: opts.OnEvent,
	}
}

// projectDB 打开并缓存项目库连接（语义同 task.Service.projectDB）。
func (s *Service) projectDB(workdir string) (*sql.DB, error) {
	clean := filepath.Clean(workdir)
	if !filepath.IsAbs(clean) {
		return nil, fmt.Errorf("%w: %s 不是绝对路径", ErrProjectNotFound, workdir)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if conn, ok := s.dbs[clean]; ok {
		return conn, nil
	}
	if _, err := os.Stat(db.MetaDBPath(clean)); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProjectNotFound, workdir)
	}
	conn, err := db.EnsureProject(context.Background(), db.MetaDBPath(clean))
	if err != nil {
		return nil, fmt.Errorf("parser: open project db %s: %w", clean, err)
	}
	s.dbs[clean] = conn
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

	// LLM 调用（客户端断开不取消 LLM 请求：用 WithoutCancel 脱离 r.Context()，
	// 超时由 llm.Client 的 http.Timeout 控制；修复 CLI 超时断开导致的 context canceled，2026-08-06）。
	client, err := llm.New(llm.FromConfig(s.llmCfg()), s.logger)
	if err != nil {
		return Draft{}, err // LLM_NOT_CONFIGURED
	}
	raw, err := client.CompleteJSON(context.WithoutCancel(ctx), llm.Request{
		System:      buildSystemPrompt(),
		User:        buildUserPrompt(sm, content),
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

	// 展平 + 依赖解析（§17.3）：先补齐临时 ID（旧草稿兼容），再按 ID/标题双索引解析。
	pr.Tasks = ensureTaskIDs(pr.Tasks)
	flattened, err := flattenTasks(pr.Tasks, "")
	if err != nil {
		return ConfirmResult{}, fmt.Errorf("%w: %v", ErrImportFailed, err)
	}
	depIDs, err := resolveDependsOn(flattened)
	if err != nil {
		return ConfirmResult{}, fmt.Errorf("%w: %v", ErrImportFailed, err)
	}
	tasks := make([]task.Task, 0, len(flattened))
	for _, f := range flattened {
		t := task.Task{
			ID:            f.ID,
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

	// 草稿置 confirmed（仅 pending 可转，并发安全）。
	now := time.Now().Format(time.RFC3339)
	if _, err := conn.ExecContext(ctx,
		`UPDATE import_drafts SET status = 'confirmed', confirmed_at = ? WHERE id = ? AND status = 'pending'`,
		now, draftID); err != nil {
		return ConfirmResult{}, fmt.Errorf("parser: confirm draft: %w", err)
	}
	s.logger.Info("import draft confirmed", "id", draftID, "created", res.Created, "archived", res.Archived)
	s.emit(ctx, workdir, "import.draft_confirmed", draftID)
	return ConfirmResult{DraftID: draftID, SourceFile: sourceFile, Created: res.Created, Archived: res.Archived}, nil
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

// DraftDetail 草稿明细（审阅界面数据源）：含完整解析任务树。
type DraftDetail struct {
	Draft
	Tasks []ParsedTask `json:"tasks"`
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
	if err := json.Unmarshal([]byte(parsed), &d.Tasks); err != nil {
		// 兼容旧存储（ParseResult 包裹）
		var pr ParseResult
		if err2 := json.Unmarshal([]byte(parsed), &pr); err2 == nil {
			d.Tasks = pr.Tasks
		} else {
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
	// 第一遍：字段校验 + 收集 ID/标题索引（依赖引用存在性检查）。
	idIndex := make(map[string]bool)
	titleIndex := make(map[string]string) // 标题 → ID
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
				if idIndex[t.ID] {
					return fmt.Errorf("草稿任务 ID 不唯一: %q", t.ID)
				}
				idIndex[t.ID] = true
			}
			titleIndex[t.Title] = t.ID
			if err := collect(t.Children); err != nil {
				return err
			}
		}
		return nil
	}
	if err := collect(tasks); err != nil {
		return err
	}
	// 第二遍：depends_on 引用存在性（临时 ID 优先，标题兜底兼容旧草稿）。
	var checkRefs func(list []ParsedTask) error
	checkRefs = func(list []ParsedTask) error {
		for _, t := range list {
			for _, dep := range t.DependsOn {
				ref := strings.TrimSpace(dep)
				if idIndex[ref] {
					continue
				}
				if _, ok := titleIndex[ref]; ok {
					continue
				}
				return fmt.Errorf("依赖任务不存在: %q（任务 %q）", dep, t.Title)
			}
			if err := checkRefs(t.Children); err != nil {
				return err
			}
		}
		return nil
	}
	return checkRefs(tasks)
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
	return ParseResult{Tasks: normalized}, err
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
		status, ok := mapStatus(sm, rt.Status)
		if !ok || status == "archived" {
			return nil, fmt.Errorf("第 %d 项 status 非法或不在状态机中: %v", i+1, rt.Status)
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
		out = append(out, ParsedTask{
			ID:          id,
			Title:       title,
			Description: rt.Description,
			Status:      status,
			Priority:    prio,
			Tags:        normalizeTags(rt.Tags),
			Assignee:    strings.TrimSpace(rt.Assignee),
			DependsOn:   cleanStrings(rt.DependsOn),
			Children:    children,
		})
	}
	return out, nil
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
