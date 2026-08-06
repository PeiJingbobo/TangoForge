package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// TaskRepo 任务数据访问接口（接口先行，docs/TECHNICAL.md §2.3）。
//
// 方法签名均以「当前项目库连接」为上下文——项目隔离由"一项目一 meta.db"天然保证
// （docs/TASK-SEMANTICS.md §1），repo 不感知 workdir 与 project_id 语义。
type TaskRepo interface {
	// Create 插入新任务（t.ID 由调用方生成，UUID v4）。
	Create(ctx context.Context, t *Task) error
	// GetByID 按 ID 查询；不存在返回 (nil, nil)。
	GetByID(ctx context.Context, id string) (*Task, error)
	// List 返回项目库内全部任务（含 archived）。
	List(ctx context.Context) ([]Task, error)
	// Update 全字段覆盖更新（service 已构造最终值）。
	Update(ctx context.Context, t *Task) error
	// Delete 物理删除任务（仅回收站任务，由 service 校验）。
	Delete(ctx context.Context, id string) error
	// ClearParentsByParentID 将 parent_id=parentID 的任务级联置空为顶层，返回受影响行数。
	ClearParentsByParentID(ctx context.Context, parentID string, updatedAt time.Time) (int64, error)
}

// dbtx 抽象 *sql.DB 与 *sql.Tx：业务层（service）控制事务边界（分层铁律），
// 事务内创建 newSQLRepoTx 复用同一套 SQL 实现。
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// sqlRepo TaskRepo 的 SQLite 实现（modernc.org/sqlite，纯 Go）。
type sqlRepo struct {
	q dbtx
}

// newSQLRepo 构造绑定项目库连接（非事务）的 repo。
func newSQLRepo(db *sql.DB) TaskRepo { return &sqlRepo{q: db} }

// newSQLRepoTx 构造绑定事务的 repo（归档/物理删除等原子操作使用）。
func newSQLRepoTx(tx *sql.Tx) TaskRepo { return &sqlRepo{q: tx} }

const taskColumns = `id, project_id, parent_id, title, description, status, priority,
	tags, assignee, depends_on, archived_from, source_file, source_section, created_at, updated_at`

func (r *sqlRepo) Create(ctx context.Context, t *Task) error {
	_, err := r.q.ExecContext(ctx, `INSERT INTO tasks (`+taskColumns+`) VALUES
		(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.ProjectID, nullStr(t.ParentID), t.Title, t.Description, t.Status,
		t.Priority, jsonArr(t.Tags), t.Assignee, jsonArr(t.DependsOn),
		nullStr(&t.ArchivedFrom), nullStr(&t.SourceFile), nullStr(&t.SourceSection),
		formatTime(t.CreatedAt), formatTime(t.UpdatedAt))
	if err != nil {
		return fmt.Errorf("task: insert %s: %w", t.ID, err)
	}
	return nil
}

func (r *sqlRepo) GetByID(ctx context.Context, id string) (*Task, error) {
	row := r.q.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("task: get %s: %w", id, err)
	}
	return t, nil
}

func (r *sqlRepo) List(ctx context.Context) ([]Task, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks`)
	if err != nil {
		return nil, fmt.Errorf("task: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("task: scan: %w", err)
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (r *sqlRepo) Update(ctx context.Context, t *Task) error {
	_, err := r.q.ExecContext(ctx, `UPDATE tasks SET
		parent_id=?, title=?, description=?, status=?, priority=?, tags=?, assignee=?,
		depends_on=?, archived_from=?, source_file=?, source_section=?, updated_at=?
		WHERE id=?`,
		nullStr(t.ParentID), t.Title, t.Description, t.Status, t.Priority, jsonArr(t.Tags),
		t.Assignee, jsonArr(t.DependsOn), nullStr(&t.ArchivedFrom), nullStr(&t.SourceFile),
		nullStr(&t.SourceSection), formatTime(t.UpdatedAt), t.ID)
	if err != nil {
		return fmt.Errorf("task: update %s: %w", t.ID, err)
	}
	return nil
}

// Delete 物理删除任务（仅回收站任务，调用方已校验 archived）。
func (r *sqlRepo) Delete(ctx context.Context, id string) error {
	if _, err := r.q.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id); err != nil {
		return fmt.Errorf("task: delete %s: %w", id, err)
	}
	return nil
}

// ClearParentsByParentID 将直接子任务级联置空为顶层（归档/物理删除父任务时，TF-007）。
func (r *sqlRepo) ClearParentsByParentID(ctx context.Context, parentID string, updatedAt time.Time) (int64, error) {
	res, err := r.q.ExecContext(ctx,
		`UPDATE tasks SET parent_id = NULL, updated_at = ? WHERE parent_id = ?`,
		formatTime(updatedAt), parentID)
	if err != nil {
		return 0, fmt.Errorf("task: clear parent %s: %w", parentID, err)
	}
	return res.RowsAffected()
}

// rowScanner 兼容 *sql.Row 与 *sql.Rows。
type rowScanner interface {
	Scan(dest ...any) error
}

// scanTask 将一行扫描为 Task（TEXT 时间戳 → time.Time，JSON 列 → []string）。
// 注意：modernc.org/sqlite 驱动不支持直接 Scan 到 []string，JSON 列先扫 string 再解析。
func scanTask(s rowScanner) (*Task, error) {
	var t Task
	var parent, archivedFrom, sourceFile, sourceSection sql.NullString
	var tagsStr, dependsStr, created, updated string
	if err := s.Scan(&t.ID, &t.ProjectID, &parent, &t.Title, &t.Description, &t.Status,
		&t.Priority, &tagsStr, &t.Assignee, &dependsStr, &archivedFrom, &sourceFile,
		&sourceSection, &created, &updated); err != nil {
		return nil, err
	}
	if parent.Valid {
		t.ParentID = &parent.String
	}
	if archivedFrom.Valid {
		t.ArchivedFrom = archivedFrom.String
	}
	if sourceFile.Valid {
		t.SourceFile = sourceFile.String
	}
	if sourceSection.Valid {
		t.SourceSection = sourceSection.String
	}
	if err := unmarshalStrings(tagsStr, &t.Tags); err != nil {
		return nil, fmt.Errorf("task: parse tags: %w", err)
	}
	if err := unmarshalStrings(dependsStr, &t.DependsOn); err != nil {
		return nil, fmt.Errorf("task: parse depends_on: %w", err)
	}
	parsed, err := time.Parse(time.RFC3339, created)
	if err != nil {
		return nil, fmt.Errorf("task: parse created_at %q: %w", created, err)
	}
	t.CreatedAt = parsed
	parsed, err = time.Parse(time.RFC3339, updated)
	if err != nil {
		return nil, fmt.Errorf("task: parse updated_at %q: %w", updated, err)
	}
	t.UpdatedAt = parsed
	return &t, nil
}

// unmarshalStrings 将 JSON 数组文本解析为 []string（空文本 → 空切片）。
func unmarshalStrings(s string, out *[]string) error {
	*out = []string{}
	if s == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(s), out); err != nil {
		return err
	}
	return nil
}

// jsonArr 编码 []string 为 JSON 数组文本（tags / depends_on 列）。
func jsonArr(v []string) string {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// nullStr 将 *string 转为 SQL 可空值（nil → NULL）。
func nullStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// formatTime 将 time.Time 格式化为 RFC3339 文本（QA Q7 时间戳约定）。
func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}
