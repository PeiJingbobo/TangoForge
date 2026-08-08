package task

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ImportResult 导入确认入库结果（TF-018 草稿确认）。
type ImportResult struct {
	SourceFile string `json:"source_file"`
	Created    int    `json:"created"`
	Archived   int    `json:"archived"`
}

// ImportTasks 文件级全量覆盖导入（TF-018 草稿确认入库专用，QA P4-1）。
//
// 语义（docs/TASK-SEMANTICS.md §17.3）：
//   - 以 source_file 为同步单元：确认后归档该文件来源的全部旧任务（未归档者），
//     按新结果批量重建；任一步失败整体回滚（事务内原子）。
//   - **WAL 写锁铁律**：事务外完成全部校验（状态机加载、title/status/依赖校验、环检测），
//     事务内首条语句即写（归档 UPDATE），随后批量 INSERT。
//   - **不触发 task.* 写钩子**（批量导入避免事件风暴）；import.* 事件与审计由 parser 层发送。
//
// 前置校验（任一项失败 → 返回错误，不落库）：
//   - title 去空白后非空；status 存在于项目状态机 states 且非 archived；
//   - depends_on 引用的任务 ID 必须存在（本批新任务或库内旧任务）；
//   - 本批内部依赖图无环（CIRCULAR_DEPENDENCY）。
func (s *service) ImportTasks(ctx context.Context, workdir, sourceFile string, tasks []Task) (ImportResult, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return ImportResult{}, err
	}
	if strings.TrimSpace(sourceFile) == "" {
		return ImportResult{}, NewInvalid("source_file 必填")
	}
	if len(tasks) == 0 {
		return ImportResult{}, NewInvalid("导入任务列表为空")
	}

	// --- 事务外校验（WAL 铁律） ---
	sm, err := loadStateMachine(workdir)
	if err != nil {
		return ImportResult{}, err
	}
	ids := make(map[string]bool, len(tasks))
	for i := range tasks {
		t := &tasks[i]
		if strings.TrimSpace(t.Title) == "" {
			return ImportResult{}, NewInvalid("任务标题不能为空（第 %d 项）", i+1)
		}
		if t.Status == "" || t.Status == StatusArchived || !stateExists(sm, t.Status) {
			return ImportResult{}, fmt.Errorf("%w: %q（第 %d 项）", ErrStatusNotFound, t.Status, i+1)
		}
		if t.ID == "" {
			return ImportResult{}, NewInvalid("任务 ID 缺失（第 %d 项）", i+1)
		}
		if ids[t.ID] {
			return ImportResult{}, NewInvalid("任务 ID 重复: %s", t.ID)
		}
		ids[t.ID] = true
	}

	repo := newSQLRepo(conn)
	// 简短编号（TF-040）：沿用文档编号；空/冲突 → 自动分配 T{n}（事务外读快照，WAL 铁律）。
	if err := ensureTaskNumbers(ctx, repo, tasks); err != nil {
		return ImportResult{}, err
	}
	// 依赖存在性：本批 + 库内（archived 允许，与 §9 一致）。
	for i := range tasks {
		t := &tasks[i]
		for _, dep := range t.DependsOn {
			if ids[dep] {
				continue
			}
			exists, err := repo.GetByID(ctx, dep)
			if err != nil {
				return ImportResult{}, err
			}
			if exists == nil {
				return ImportResult{}, NewDependencyNotFound(dep)
			}
		}
	}
	// 本批内部依赖图环检测（新任务间引用；指向库内旧任务的引用不可能成环）。
	if err := detectBatchCycle(tasks); err != nil {
		return ImportResult{}, err
	}

	// --- 事务：首语句即写（归档旧任务）→ 批量插入 ---
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, fmt.Errorf("task: begin import tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	res, err := tx.ExecContext(ctx,
		`UPDATE tasks SET status = ?, archived_from = status, updated_at = ?
		 WHERE source_file = ? AND status != ?`,
		StatusArchived, formatTime(now), sourceFile, StatusArchived)
	if err != nil {
		return ImportResult{}, fmt.Errorf("task: archive old source %s: %w", sourceFile, err)
	}
	archived, _ := res.RowsAffected()

	rtx := newSQLRepoTx(tx)
	for i := range tasks {
		t := tasks[i]
		t.ProjectID = 1 // 项目库内固定写 1（§1）。
		t.ArchivedFrom = ""
		t.SourceFile = sourceFile
		if t.CreatedAt.IsZero() {
			t.CreatedAt = now
		}
		t.UpdatedAt = now
		if err := rtx.Create(ctx, &t); err != nil {
			return ImportResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ImportResult{}, fmt.Errorf("task: commit import: %w", err)
	}
	s.logger.Info("tasks imported", "source", sourceFile, "created", len(tasks), "archived", archived)
	return ImportResult{SourceFile: sourceFile, Created: len(tasks), Archived: int(archived)}, nil
}

// detectBatchCycle 检测导入批次内部依赖图是否有环（多跳 DFS）。
func detectBatchCycle(tasks []Task) error {
	byID := make(map[string][]string, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t.DependsOn
	}
	visiting := make(map[string]bool, len(tasks))
	visited := make(map[string]bool, len(tasks))
	var dfs func(id string) error
	dfs = func(id string) error {
		if visiting[id] {
			return NewCircularDependency(id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dep := range byID[id] {
			// 仅本批内部引用参与环检测（库内旧任务不可能反向依赖新任务）。
			if _, ok := byID[dep]; ok {
				if err := dfs(dep); err != nil {
					return err
				}
			}
		}
		delete(visiting, id)
		visited[id] = true
		return nil
	}
	for id := range byID {
		if err := dfs(id); err != nil {
			return err
		}
	}
	return nil
}
