package task

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"tangoforge/internal/config"
	"tangoforge/internal/db"
)

// Service 任务域业务服务（三端共享入口：HTTP / MCP / CLI，docs/TECHNICAL.md §2.3）。
//
// 方法签名统一携带 workdir（多项目显式标识，QA Q2-B：不依赖全局注册表，
// 项目识别以 {workdir}/.taskboard/meta.db 元数据存在为准，tasks.project_id 固定 1）。
// 删除语义不在本接口（归档/还原/物理删除属 TF-007）。
type Service interface {
	// Create 创建任务（UUID v4、默认 todo、priority 别名归一化，docs/TASK-SEMANTICS.md §3）。
	Create(ctx context.Context, workdir string, in CreateInput) (Task, error)
	// Get 返回任务详情（不含 children，§7）。
	Get(ctx context.Context, workdir, id string) (Task, error)
	// List 任务树 / 扁平分页（§6）。
	List(ctx context.Context, workdir string, f ListFilter) (ListResult, error)
	// Update 任务详情更新（禁止修改 status，§4.1）。
	Update(ctx context.Context, workdir, id string, in UpdateInput) (Task, error)
	// ChangeStatus 独立的状态更新接口（§5；transitions 校验已接入，TF-006）。
	ChangeStatus(ctx context.Context, workdir, id, status string) (Task, error)
	// GetStateMachine 读取项目状态机定义（缺失回退默认四态，§5.2）。
	GetStateMachine(ctx context.Context, workdir string) (config.StateMachine, error)
	// UpdateStateMachine 编辑状态机（编辑校验 + STATUS_IN_USE 占用校验 + 持久化，§5.2）。
	UpdateStateMachine(ctx context.Context, workdir string, sm config.StateMachine) (config.StateMachine, error)
	// Archive 归档任务（删除语义 §8.1：status→archived + archived_from + 级联置空原子；幂等 Q2-B）。
	Archive(ctx context.Context, workdir, id string) (ArchiveResult, error)
	// Restore 还原归档任务（§8.2：恢复到 archived_from；FallbackTodo 处理状态已删除 Q5）。
	Restore(ctx context.Context, workdir, id string, opts RestoreOptions) (Task, error)
	// Delete 物理删除回收站任务（§8.3：仅 archived，级联置空原子，返回被删快照）。
	Delete(ctx context.Context, workdir, id string) (Task, error)
	// ImportTasks 文件级全量覆盖导入（TF-018 草稿确认入库，§17.3：归档旧 source_file + 批量重建原子）。
	ImportTasks(ctx context.Context, workdir, sourceFile string, tasks []Task) (ImportResult, error)
	// Graph 全景图全量数据（§12.5：未归档任务 + parent/dependency 边；服务端不聚簇，TF-017）。
	Graph(ctx context.Context, workdir string) (GraphData, error)
	// Close 关闭全部缓存的项目库连接（进程退出/测试清理时调用）。
	Close() error
}

// WriteHook 写操作成功后的回调钩子（docs/TASK-SEMANTICS.md §11，Q14-A）。
//
// 签名扩展（QA P3-1 确认）：携带 workdir（审计表在项目库、WS 事件需要 project 字段），
// actor/actor_class 由调用方写入 ctx（auth.WithActor）后经 ctx 读取。
// action 取 task.created / task.updated / task.status_changed / task.archived /
// task.restored / task.deleted / state_machine.changed；target 为任务 ID（state_machine 为 workdir）。
// TF-012 异步审计与 TF-014 WS 事件经此接入，不改 Service 业务签名。
type WriteHook func(ctx context.Context, workdir, action, target string)

// Options Service 构造选项。
type Options struct {
	Logger  *slog.Logger
	OnWrite WriteHook
}

// CreateInput 创建任务入参（字段语义见 docs/TASK-SEMANTICS.md §2/§3）。
type CreateInput struct {
	ParentID    *string  `json:"parent_id"` // nil = 顶层任务
	Title       string   `json:"title"`     // 必填，去空白后非空
	Description string   `json:"description"`
	Status      *string  `json:"status"`   // nil = 默认 todo
	Priority    any      `json:"priority"` // int 0-5 | 字符串别名（§3 归一化）
	Tags        []string `json:"tags"`
	Assignee    string   `json:"assignee"`
	DependsOn   []string `json:"depends_on"` // TF-005 仅存储，环校验 TF-008
}

// UpdateInput 部分更新入参（指针语义见 docs/TASK-SEMANTICS.md §4.2，Q7-A）。
// nil = 不更新；tags/depends_on 的 &[] = 清空；parent_id 三重态：nil 不改 / &nil 置顶 / &str 改父。
type UpdateInput struct {
	Title       *string   `json:"title"`
	Description *string   `json:"description"`
	Priority    *any      `json:"priority"`
	Tags        *[]string `json:"tags"`
	Assignee    *string   `json:"assignee"`
	DependsOn   *[]string `json:"depends_on"`
	ParentID    **string  `json:"parent_id"`
}

// ListFilter 列表过滤/分页参数（docs/TASK-SEMANTICS.md §6）。
type ListFilter struct {
	Status string // 单值状态过滤；空 = 排除 archived；"archived" = 仅归档
	Q      string // 匹配 title/description（大小写不敏感包含）
	Page   int    // 0 = 返回全量任务树；>0 = 扁平分页
	Size   int    // 分页大小，默认 100，上限 500
}

// ListResult 列表返回（树形或扁平分页二选一）。
type ListResult struct {
	Tree  []*TaskTreeNode `json:"tree,omitempty"`  // 非分页模式
	Items []Task          `json:"items,omitempty"` // 分页模式
	Total int             `json:"total"`
	Page  int             `json:"page"`
	Size  int             `json:"size"`
}

// TaskTreeNode 树形节点：Task 字段平铺 + children（docs/TASK-SEMANTICS.md §6.1）。
type TaskTreeNode struct {
	Task
	Children []*TaskTreeNode `json:"children"`
}

// 分页默认值与上限。
const (
	defaultPageSize = 100
	maxPageSize     = 500
)

// service 实现。
type service struct {
	mu      sync.Mutex
	dbs     map[string]*sql.DB // workdir → 项目库连接（SetMaxOpenConns(1)）
	logger  *slog.Logger
	onWrite WriteHook
}

// NewService 构造任务服务。
func NewService(opts Options) Service {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &service{
		dbs:     make(map[string]*sql.DB),
		logger:  opts.Logger,
		onWrite: opts.OnWrite,
	}
}

// projectDB 打开并缓存项目库连接（QA Q1-A）。
//
// 项目识别（QA Q2-B）：校验 {workdir}/.taskboard/meta.db 存在即视为项目，
// 不查询全局注册表；连接复用，重复打开幂等（EnsureProject 迁移幂等）。
func (s *service) projectDB(ctx context.Context, workdir string) (*sql.DB, error) {
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
	conn, err := db.EnsureProject(ctx, db.MetaDBPath(clean))
	if err != nil {
		return nil, fmt.Errorf("task: open project db %s: %w", clean, err)
	}
	s.dbs[clean] = conn
	return conn, nil
}

// Create 创建任务（docs/TASK-SEMANTICS.md §3）。
func (s *service) Create(ctx context.Context, workdir string, in CreateInput) (Task, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return Task{}, err
	}
	repo := newSQLRepo(conn)

	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Task{}, NewInvalid("title 必填且不能为空白")
	}

	// 状态：缺省 todo；显式传入须存在于状态机 states 且非 archived 保留态。
	status := StatusTodo
	if in.Status != nil {
		status = strings.TrimSpace(*in.Status)
		if status == StatusArchived {
			return Task{}, ErrStatusNotFound
		}
		if err := s.checkStatusExists(workdir, status); err != nil {
			return Task{}, err
		}
	}

	// 父任务：存在且同项目。
	if in.ParentID != nil {
		if err := s.ensureParent(ctx, repo, *in.ParentID); err != nil {
			return Task{}, err
		}
	}

	priority, err := NormalizePriority(in.Priority)
	if err != nil {
		return Task{}, err
	}

	// 依赖校验（TF-008）：先于写入，拒绝不产生脏数据（Q4-A）。
	depends := cleanStrings(in.DependsOn)
	id := uuid.NewString()
	if err := s.validateDependencies(ctx, repo, id, depends); err != nil {
		return Task{}, err
	}

	now := time.Now()
	t := Task{
		ID:          id,
		ProjectID:   1, // docs/TASK-SEMANTICS.md §1：项目库内固定写 1（文档性冗余）
		ParentID:    in.ParentID,
		Title:       title,
		Description: in.Description,
		Status:      status,
		Priority:    priority,
		Tags:        normalizeTags(in.Tags),
		Assignee:    in.Assignee,
		DependsOn:   depends,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repo.Create(ctx, &t); err != nil {
		return Task{}, err
	}
	s.logger.Debug("task created", "id", t.ID, "workdir", workdir)
	s.emit(ctx, workdir, "task.created", t.ID)
	return t, nil
}

// Get 返回任务详情（不含 children）。
func (s *service) Get(ctx context.Context, workdir, id string) (Task, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return Task{}, err
	}
	t, err := newSQLRepo(conn).GetByID(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if t == nil {
		return Task{}, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	return *t, nil
}

// List 任务树 / 扁平分页（docs/TASK-SEMANTICS.md §6）。
func (s *service) List(ctx context.Context, workdir string, f ListFilter) (ListResult, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return ListResult{}, err
	}
	all, err := newSQLRepo(conn).List(ctx)
	if err != nil {
		return ListResult{}, err
	}

	// 状态过滤：空 = 排除 archived；显式 status = 精确匹配（含 archived）。
	matched := make([]Task, 0, len(all))
	for _, t := range all {
		if f.Status == "" {
			if t.Status == StatusArchived {
				continue
			}
		} else if t.Status != f.Status {
			continue
		}
		if f.Q != "" {
			hay := strings.ToLower(t.Title + "\n" + t.Description)
			if !strings.Contains(hay, strings.ToLower(f.Q)) {
				continue
			}
		}
		matched = append(matched, t)
	}

	if f.Page <= 0 {
		// 树形模式：祖先保留、后代过滤。
		ids := make(map[string]bool, len(matched))
		allMap := make(map[string]Task, len(all))
		for i := range matched {
			ids[matched[i].ID] = true
		}
		for i := range all {
			allMap[all[i].ID] = all[i]
		}
		collectAncestors(allMap, ids)
		set := make([]Task, 0, len(ids))
		for _, t := range allMap {
			if ids[t.ID] {
				set = append(set, t)
			}
		}
		return ListResult{Tree: buildTree(set)}, nil
	}

	// 扁平分页：全局排序后切片（page 从 1 起）。
	size := f.Size
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	sortTasks(matched)
	total := len(matched)
	start := (page - 1) * size
	if start >= total {
		return ListResult{Items: []Task{}, Total: total, Page: page, Size: size}, nil
	}
	end := start + size
	if end > total {
		end = total
	}
	return ListResult{Items: matched[start:end], Total: total, Page: page, Size: size}, nil
}

// Update 任务详情更新（docs/TASK-SEMANTICS.md §4；禁止修改 status）。
func (s *service) Update(ctx context.Context, workdir, id string, in UpdateInput) (Task, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return Task{}, err
	}
	repo := newSQLRepo(conn)

	t, err := repo.GetByID(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if t == nil {
		return Task{}, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}

	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" {
			return Task{}, NewInvalid("title 不能为空白")
		}
		t.Title = title
	}
	if in.Description != nil {
		t.Description = *in.Description
	}
	if in.Priority != nil {
		p, err := NormalizePriority(*in.Priority)
		if err != nil {
			return Task{}, err
		}
		t.Priority = p
	}
	if in.Tags != nil {
		t.Tags = normalizeTags(*in.Tags)
	}
	if in.Assignee != nil {
		t.Assignee = *in.Assignee
	}
	if in.DependsOn != nil {
		newDep := cleanStrings(*in.DependsOn)
		// 依赖校验（TF-008）：先于写入，拒绝时任务状态不脏。
		if err := s.validateDependencies(ctx, repo, t.ID, newDep); err != nil {
			return Task{}, err
		}
		t.DependsOn = newDep
	}
	if in.ParentID != nil {
		if *in.ParentID == nil {
			t.ParentID = nil // 置为顶层
		} else {
			newParent := **in.ParentID
			if newParent == t.ID {
				return Task{}, ErrParentCycle // 自引用
			}
			if err := s.ensureParent(ctx, repo, newParent); err != nil {
				return Task{}, err
			}
			if err := s.checkParentCycle(ctx, repo, t.ID, newParent); err != nil {
				return Task{}, err
			}
			t.ParentID = &newParent
		}
	}

	t.UpdatedAt = time.Now()
	if err := repo.Update(ctx, t); err != nil {
		return Task{}, err
	}
	s.logger.Debug("task updated", "id", t.ID, "workdir", workdir)
	s.emit(ctx, workdir, "task.updated", t.ID)
	return *t, nil
}

// ChangeStatus 独立状态更新（docs/TASK-SEMANTICS.md §5）。
// ChangeStatus 独立状态更新（docs/TASK-SEMANTICS.md §5）。
// 校验链：任务存在 → 同态幂等（Q2-A）→ 状态存在于状态机（非 archived）→
// 流转校验（validateTransition，TF-006：Q1-B 宽松 / Q3-A 空规则特例）。
func (s *service) ChangeStatus(ctx context.Context, workdir, id, status string) (Task, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return Task{}, err
	}
	repo := newSQLRepo(conn)

	t, err := repo.GetByID(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if t == nil {
		return Task{}, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	status = strings.TrimSpace(status)
	if status == StatusArchived {
		return Task{}, ErrStatusNotFound // archived 仅由归档/还原（TF-007）设置
	}
	sm, err := loadStateMachine(workdir)
	if err != nil {
		return Task{}, err
	}
	if !stateExists(sm, status) {
		return Task{}, fmt.Errorf("%w: %s", ErrStatusNotFound, status)
	}
	// 同态流转：幂等成功，不校验、不刷新 updated_at（Q2-A）。
	if t.Status == status {
		return *t, nil
	}
	// 流转校验（TF-006）。
	if err := validateTransition(sm, t.Status, status); err != nil {
		return Task{}, err
	}

	t.Status = status
	t.UpdatedAt = time.Now()
	if err := repo.Update(ctx, t); err != nil {
		return Task{}, err
	}
	s.logger.Debug("task status changed", "id", t.ID, "status", status, "workdir", workdir)
	s.emit(ctx, workdir, "task.status_changed", t.ID)
	return *t, nil
}

// checkStatusExists 校验状态存在于项目状态机 states（统一走 loadStateMachine）。
func (s *service) checkStatusExists(workdir, status string) error {
	sm, err := loadStateMachine(workdir)
	if err != nil {
		return err
	}
	if stateExists(sm, status) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrStatusNotFound, status)
}

// ensureParent 校验父任务存在（同项目库内查询天然同项目）。
func (s *service) ensureParent(ctx context.Context, repo TaskRepo, parentID string) error {
	p, err := repo.GetByID(ctx, parentID)
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("%w: %s", ErrParentNotFound, parentID)
	}
	return nil
}

// checkParentCycle 沿新父链向上走，若回到任务自身则存在父链环（PARENT_CYCLE）。
func (s *service) checkParentCycle(ctx context.Context, repo TaskRepo, id, newParent string) error {
	cur := newParent
	for cur != "" {
		if cur == id {
			return ErrParentCycle
		}
		p, err := repo.GetByID(ctx, cur)
		if err != nil {
			return err
		}
		if p == nil || p.ParentID == nil {
			break
		}
		cur = *p.ParentID
	}
	return nil
}

// emit 触发写钩子（nil 安全，TF-012/014 接入点）。
func (s *service) emit(ctx context.Context, workdir, action, target string) {
	if s.onWrite != nil {
		s.onWrite(ctx, workdir, action, target)
	}
}

// Close 关闭全部缓存的项目库连接并清空缓存。
func (s *service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for wd, conn := range s.dbs {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("task: close %s: %w", wd, err)
		}
		delete(s.dbs, wd)
	}
	return firstErr
}

// normalizeTags 去重 + 去空串 + 保持插入顺序（docs/TASK-SEMANTICS.md §2/§3）。
func normalizeTags(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

// cleanStrings 去空串（保持顺序，不去重；depends_on 用）。
func cleanStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// sortTasks 全局排序：priority DESC, created_at ASC, id ASC（docs/TASK-SEMANTICS.md §6.3）。
func sortTasks(ts []Task) {
	sort.SliceStable(ts, func(i, j int) bool {
		if ts[i].Priority != ts[j].Priority {
			return ts[i].Priority > ts[j].Priority
		}
		if !ts[i].CreatedAt.Equal(ts[j].CreatedAt) {
			return ts[i].CreatedAt.Before(ts[j].CreatedAt)
		}
		return ts[i].ID < ts[j].ID
	})
}

// buildTree 组装任务树（孤儿或 parent 不在集合内 → 顶层），每层内部排序。
func buildTree(tasks []Task) []*TaskTreeNode {
	if len(tasks) == 0 {
		return []*TaskTreeNode{}
	}
	nodes := make(map[string]*TaskTreeNode, len(tasks))
	for i := range tasks {
		t := tasks[i]
		nodes[t.ID] = &TaskTreeNode{Task: t, Children: []*TaskTreeNode{}}
	}
	roots := make([]*TaskTreeNode, 0, len(tasks))
	for _, n := range nodes {
		if n.ParentID != nil {
			if p, ok := nodes[*n.ParentID]; ok {
				p.Children = append(p.Children, n)
				continue
			}
		}
		roots = append(roots, n)
	}
	sortTreeNodes(roots)
	return roots
}

// sortTreeNodes 按排序规则排序节点切片。
func sortTreeNodes(nodes []*TaskTreeNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Priority != nodes[j].Priority {
			return nodes[i].Priority > nodes[j].Priority
		}
		if !nodes[i].CreatedAt.Equal(nodes[j].CreatedAt) {
			return nodes[i].CreatedAt.Before(nodes[j].CreatedAt)
		}
		return nodes[i].ID < nodes[j].ID
	})
	for _, n := range nodes {
		if len(n.Children) > 0 {
			sortTreeNodes(n.Children)
		}
	}
}

// collectAncestors 祖先保留：把匹配集合中任务的祖先（不在集合内者）补入集合。
// 祖先自身不匹配 filter，仅作为容器保留（docs/TASK-SEMANTICS.md §6.2）。
func collectAncestors(all map[string]Task, matched map[string]bool) {
	changed := true
	for changed {
		changed = false
		for id := range matched {
			t, ok := all[id]
			if !ok || t.ParentID == nil {
				continue
			}
			pid := *t.ParentID
			if _, exists := all[pid]; exists && !matched[pid] {
				matched[pid] = true
				changed = true
			}
		}
	}
}

// 编译期断言：*service 满足 Service 接口。
var _ Service = (*service)(nil)
