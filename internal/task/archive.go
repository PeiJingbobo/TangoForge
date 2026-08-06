package task

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// 归档 / 还原 / 物理删除（TF-007，docs/TASK-SEMANTICS.md §8）。
//
// 语义要点：
//   - 归档：status → archived + 记录 archived_from + 直接子任务级联置空为顶层，事务原子；
//     已归档任务再次归档幂等返回（Q2-B）；返回 DependentCount（未归档任务依赖数，仅提示不阻断）。
//   - 还原：仅 archived 可还原，恢复到 archived_from（缺失回退 todo）；
//     目标状态已从状态机删除 → 默认 STATUS_NOT_FOUND，FallbackTodo 时回退 todo（Q5）。
//   - 物理删除：仅回收站（archived）任务；父任务删除时子任务不可一并删除（Q8-A），
//     仅级联置空 parent_id，事务原子；返回被删任务快照（Q9-A）。

// ArchiveResult 归档返回（docs/TASK-SEMANTICS.md §8.1）。
type ArchiveResult struct {
	Task Task `json:"task"`
	// DependentCount 未归档任务中 depends_on 包含该任务的个数（不阻断归档，仅提示）。
	DependentCount int `json:"dependent_count"`
	// ChildrenCleared 被级联置空的直接子任务数（供 UI 确认流使用，Q3）。
	ChildrenCleared int `json:"children_cleared"`
}

// RestoreOptions 还原选项（docs/TASK-SEMANTICS.md §8.2，Q5）。
type RestoreOptions struct {
	// FallbackTodo 为 true 时，archived_from 目标状态已从状态机删除则回退 todo；
	// 默认 false：目标状态不存在返回 STATUS_NOT_FOUND（由 UI 询问用户处置）。
	FallbackTodo bool `json:"fallback_todo"`
}

// Archive 归档任务（docs/TASK-SEMANTICS.md §8.1）。
//
// 已归档任务再次归档 → 幂等返回当前状态（Q2-B，不重复记录、不触发钩子）。
// 归档 + 级联置空在同一事务内原子完成（QA Q3）。
func (s *service) Archive(ctx context.Context, workdir, id string) (ArchiveResult, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return ArchiveResult{}, err
	}

	// 幂等短路（Q2-B）：已归档 → 返回当前状态。
	pre, err := newSQLRepo(conn).GetByID(ctx, id)
	if err != nil {
		return ArchiveResult{}, err
	}
	if pre == nil {
		return ArchiveResult{}, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	if pre.Status == StatusArchived {
		return ArchiveResult{Task: *pre}, nil
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return ArchiveResult{}, fmt.Errorf("task: archive begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	repo := newSQLRepoTx(tx)

	t, err := repo.GetByID(ctx, id)
	if err != nil {
		return ArchiveResult{}, err
	}
	if t == nil {
		return ArchiveResult{}, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}

	now := time.Now()
	t.ArchivedFrom = t.Status
	t.Status = StatusArchived
	t.UpdatedAt = now
	if err := repo.Update(ctx, t); err != nil {
		return ArchiveResult{}, err
	}
	children, err := repo.ClearParentsByParentID(ctx, id, now)
	if err != nil {
		return ArchiveResult{}, err
	}
	dep, err := s.dependentCount(ctx, repo, id)
	if err != nil {
		return ArchiveResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return ArchiveResult{}, fmt.Errorf("task: archive commit: %w", err)
	}
	s.logger.Debug("task archived", "id", t.ID, "children", children, "workdir", workdir)
	s.emit(ctx, workdir, "task.archived", t.ID)
	return ArchiveResult{Task: *t, DependentCount: dep, ChildrenCleared: int(children)}, nil
}

// Restore 还原归档任务（docs/TASK-SEMANTICS.md §8.2）。
func (s *service) Restore(ctx context.Context, workdir, id string, opts RestoreOptions) (Task, error) {
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
	if t.Status != StatusArchived {
		return Task{}, NewInvalid("仅归档任务可还原")
	}

	target := strings.TrimSpace(t.ArchivedFrom)
	if target == "" {
		target = StatusTodo // Q6-A：archived_from 缺失（异常数据）回退 todo
	}
	// Q5：目标状态已从状态机删除 → 默认拒绝，由 UI 询问用户；FallbackTodo 时回退 todo。
	sm, err := loadStateMachine(workdir)
	if err != nil {
		return Task{}, err
	}
	if !stateExists(sm, target) {
		if !opts.FallbackTodo {
			return Task{}, fmt.Errorf("%w: 还原目标状态 %q 已不存在，可重试 FallbackTodo 或完善状态机", ErrStatusNotFound, target)
		}
		target = StatusTodo
	}

	t.Status = target
	t.ArchivedFrom = "" // 还原后清空（Q7）
	t.UpdatedAt = time.Now()
	if err := repo.Update(ctx, t); err != nil {
		return Task{}, err
	}
	s.logger.Debug("task restored", "id", t.ID, "status", target, "workdir", workdir)
	s.emit(ctx, workdir, "task.restored", t.ID)
	return *t, nil
}

// Delete 物理删除回收站任务（docs/TASK-SEMANTICS.md §8.3）。
//
// 仅 archived（回收站）任务可物理删除（DELETE_NOT_ALLOWED）；
// 父任务删除时子任务不可一并删除（Q8-A），仅级联置空 parent_id 保留为顶层，事务原子；
// 返回被删任务快照（Q9-A）。
func (s *service) Delete(ctx context.Context, workdir, id string) (Task, error) {
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
	if t.Status != StatusArchived {
		return Task{}, fmt.Errorf("%w: %s", ErrDeleteNotAllowed, id)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, fmt.Errorf("task: delete begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	repo := newSQLRepoTx(tx)

	if _, err := repo.ClearParentsByParentID(ctx, id, time.Now()); err != nil {
		return Task{}, err
	}
	if err := repo.Delete(ctx, id); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("task: delete commit: %w", err)
	}
	s.logger.Debug("task deleted", "id", t.ID, "workdir", workdir)
	s.emit(ctx, workdir, "task.deleted", t.ID)
	return *t, nil
}

// dependentCount 统计未归档任务中 depends_on 包含目标 id 的个数（Q1-A 口径）。
func (s *service) dependentCount(ctx context.Context, repo TaskRepo, id string) (int, error) {
	tasks, err := repo.List(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range tasks {
		if t.Status == StatusArchived {
			continue
		}
		for _, d := range t.DependsOn {
			if d == id {
				n++
				break
			}
		}
	}
	return n, nil
}
