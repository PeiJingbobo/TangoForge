package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"tangoforge/internal/db"
	"tangoforge/internal/project"
)

// ErrProjectNotFound 表示工作目录未导入为项目（无 {workdir}/.taskboard/meta.db）。
// 与 task 包语义一致（docs/TASK-SEMANTICS.md §1：项目识别以元数据存在为准）。
var ErrProjectNotFound = errors.New("auth: project not found")

// ErrInvalidAction 表示权限动作不在 v1 全集内。
var ErrInvalidAction = errors.New("auth: invalid action")

// ErrPermissionDenied 权限拒绝（MCP 等非 HTTP 通道复用，TF-016）。
var ErrPermissionDenied = errors.New("auth: permission denied")

// PermissionStore 项目权限存储：permissions 表（项目库 meta.db）读写。
//
// 语义（QA P3-5/P3-6 确认）：
//   - Get / Set 均操作**全量 16 项**（缺失行视为 false）；
//   - Set 为**全量覆盖**：提交的 action 必须属于 v1 全集，未提交项重置 false；
//   - Allowed 单动作判定：行缺失 / 未知 action → false（安全默认）。
//
// 连接管理：按 workdir 打开并缓存项目库连接（与 task.Service 分离，
// 各自 SetMaxOpenConns(1)，WAL 下并发安全）。
type PermissionStore struct {
	mu     sync.Mutex
	dbs    map[string]*sql.DB
	logger *slog.Logger
	// OnDenied 权限拒绝回调（由 api 层注入，TF-012 审计 denied 记录接入点；
	// auth 包不直接依赖 audit，保持职责单一）。
	OnDenied func(ctx context.Context, workdir, action string)
}

// NewPermissionStore 构造权限存储。
func NewPermissionStore(logger *slog.Logger) *PermissionStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &PermissionStore{dbs: make(map[string]*sql.DB), logger: logger}
}

// projectDB 打开并缓存项目库连接（语义同 task.Service.projectDB）。
func (s *PermissionStore) projectDB(workdir string) (*sql.DB, error) {
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
		return nil, fmt.Errorf("auth: open project db %s: %w", clean, err)
	}
	s.dbs[clean] = conn
	return conn, nil
}

// Get 返回项目全部权限动作（全量 16 项，缺失行视为 false，QA P3-6）。
func (s *PermissionStore) Get(ctx context.Context, workdir string) (map[string]bool, error) {
	conn, err := s.projectDB(workdir)
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx,
		`SELECT action, allowed FROM permissions WHERE project_id = 1`)
	if err != nil {
		return nil, fmt.Errorf("auth: query permissions %s: %w", workdir, err)
	}
	defer func() { _ = rows.Close() }()

	// 以 v1 全集为骨架，避免依赖库内残留行。
	out := make(map[string]bool, len(project.AllActions))
	for _, a := range project.AllActions {
		out[a] = false
	}
	for rows.Next() {
		var action string
		var allowed int
		if err := rows.Scan(&action, &allowed); err != nil {
			return nil, fmt.Errorf("auth: scan permission: %w", err)
		}
		if _, ok := out[action]; ok {
			out[action] = allowed != 0
		}
	}
	return out, rows.Err()
}

// Allowed 单动作判定：ui 之外的来源查询权限表；行缺失 / 未知 action → false。
func (s *PermissionStore) Allowed(ctx context.Context, workdir, action string) (bool, error) {
	conn, err := s.projectDB(workdir)
	if err != nil {
		return false, err
	}
	var allowed int
	err = conn.QueryRowContext(ctx,
		`SELECT allowed FROM permissions WHERE project_id = 1 AND action = ?`, action).Scan(&allowed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil // 行缺失 → denied（安全默认）
	}
	if err != nil {
		return false, fmt.Errorf("auth: query permission %s/%s: %w", workdir, action, err)
	}
	return allowed != 0, nil
}

// Require 供非 HTTP 通道（MCP stdio / HTTP 工具，TF-016）复用授权判定：
//   - actor_class == ui → 放行（防御性；MCP 通道识别恒为 agent/unknown）；
//   - 其余 → 查项目 permissions 表，未授权触发 OnDenied（审计 denied）并返回 ErrPermissionDenied。
func (s *PermissionStore) Require(ctx context.Context, workdir, action string) error {
	actor := ActorFrom(ctx)
	if actor.Class == ClassUI {
		return nil
	}
	allowed, err := s.Allowed(ctx, workdir, action)
	if err != nil {
		return err
	}
	if !allowed {
		if s.OnDenied != nil {
			s.OnDenied(ctx, workdir, action)
		}
		return fmt.Errorf("%w: %s", ErrPermissionDenied, action)
	}
	return nil
}

// Set 全量覆盖项目权限（QA P3-5）：提交项必须属于 v1 全集，未提交项重置 false。
// 事务内完成；返回覆盖后的全量结果。
func (s *PermissionStore) Set(ctx context.Context, workdir string, actions map[string]bool) (map[string]bool, error) {
	// 先校验：action 必须属于 v1 全集（拒绝未知动作，防止脏数据）。
	for action := range actions {
		if !validAction(action) {
			return nil, fmt.Errorf("%w: %s", ErrInvalidAction, action)
		}
	}
	conn, err := s.projectDB(workdir)
	if err != nil {
		return nil, err
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("auth: begin tx %s: %w", workdir, err)
	}
	defer func() { _ = tx.Rollback() }()

	// 全量重置：未提交项置 false（全量覆盖语义）。
	for _, action := range project.AllActions {
		allowed := 0
		if actions[action] {
			allowed = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO permissions (project_id, action, allowed) VALUES (1, ?, ?)
			 ON CONFLICT(project_id, action) DO UPDATE SET allowed = excluded.allowed`,
			action, allowed); err != nil {
			return nil, fmt.Errorf("auth: upsert permission %s: %w", action, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("auth: commit permissions %s: %w", workdir, err)
	}

	out := make(map[string]bool, len(project.AllActions))
	for _, a := range project.AllActions {
		out[a] = actions[a]
	}
	s.logger.Info("permissions updated", "workdir", workdir)
	return out, nil
}

// Close 关闭全部缓存的项目库连接。
func (s *PermissionStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for wd, conn := range s.dbs {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("auth: close %s: %w", wd, err)
		}
		delete(s.dbs, wd)
	}
	return firstErr
}

// validAction 判断动作是否属于 v1 权限全集（REQUIREMENTS.md §7.1）。
func validAction(action string) bool {
	for _, a := range project.AllActions {
		if a == action {
			return true
		}
	}
	return false
}

// AllActions 导出 v1 动作全集（api 层 GET /api/permissions 校验用，转发 project.AllActions）。
func AllActions() []string { return project.AllActions }
