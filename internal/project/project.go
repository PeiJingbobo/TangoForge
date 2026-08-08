// Package project 是项目注册表业务层（QA Q10：独立子包）。
//
// 职责：将本地目录导入为项目、项目列表、移除注册记录（绝不删除磁盘数据）、
// last_opened_at 维护；导入无元数据目录时自动初始化 .taskboard/（QA Q11）。
//
// 分层铁律（AGENTS.md §3.2）：本包为业务层，禁止引用 api / mcp / cmd；
// 数据库事务边界在本层控制；传输层（api / mcp / cli）共享本层实现。
package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tangoforge/internal/config"
	"tangoforge/internal/db"
)

// ErrNotFound 表示目标项目注册记录不存在。
var ErrNotFound = errors.New("project: not found")

// ErrInvalidWorkdir 表示工作目录非法（不存在、非目录或非绝对路径）。
var ErrInvalidWorkdir = errors.New("project: invalid workdir")

// AllActions v1 Agent 权限动作全集（REQUIREMENTS.md §7.1），初始化时全部显式写入。
var AllActions = []string{
	"project.read",
	"task.read",
	"task.create",
	"task.update",
	"task.update_status",
	"task.delete",
	"task.restore",
	"import.run",
	"import.confirm",
	"export.run",
	"graph.read",
	"skill.read",
	"skill.install",
	"state_machine.read",
	"state_machine.write",
	"audit.read",
	"permission.read",
}

// DefaultGrantedActions 新项目默认授予 Agent 的只读权限（REQUIREMENTS.md §7.1）。
var DefaultGrantedActions = map[string]bool{
	"task.read":       true,
	"graph.read":      true,
	"skill.read":      true,
	"project.read":    true,
	"permission.read": true,
}

// Project 项目注册表记录（projects 表，仅「名称 + 工作目录」）。
type Project struct {
	ID           int64   `json:"id" db:"id"`
	Name         string  `json:"name" db:"name"`
	Workdir      string  `json:"workdir" db:"workdir"`
	CreatedAt    string  `json:"created_at" db:"created_at"` // RFC3339 本地时区
	LastOpenedAt *string `json:"last_opened_at" db:"last_opened_at"`
}

// Service 项目注册表业务服务。
type Service struct {
	registry *sql.DB // 全局注册表库（已迁移）
	logger   *slog.Logger
}

// NewService 构造项目服务。
func NewService(registry *sql.DB, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{registry: registry, logger: logger}
}

// Import 将目录导入为项目（REQUIREMENTS.md §1.4）。
//
//   - 目录不存在 / 非目录 / 非绝对路径 → ErrInvalidWorkdir；
//   - 已注册（重复导入）→ 幂等返回已有记录；
//   - 未注册：目录已有 .taskboard/ 则直接注册；否则自动初始化
//     （meta.db 4 表 + config.yaml 默认状态机/export + 默认 Agent 只读权限）。
func (s *Service) Import(ctx context.Context, workdir string) (Project, error) {
	clean := filepath.Clean(workdir)
	if !filepath.IsAbs(clean) {
		return Project{}, fmt.Errorf("%w: %s 不是绝对路径", ErrInvalidWorkdir, workdir)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return Project{}, fmt.Errorf("%w: %s 不存在或不可访问", ErrInvalidWorkdir, workdir)
	}
	if !info.IsDir() {
		return Project{}, fmt.Errorf("%w: %s 不是目录", ErrInvalidWorkdir, workdir)
	}

	// 已注册 → 幂等返回。
	if existing, ok, err := s.findByWorkdir(ctx, clean); err != nil {
		return Project{}, err
	} else if ok {
		s.logger.Info("project already registered (idempotent)", "workdir", clean, "id", existing.ID)
		return existing, nil
	}

	// 初始化或复用元数据目录。
	metaDir := filepath.Join(clean, ".taskboard")
	if _, err := os.Stat(metaDir); errors.Is(err, os.ErrNotExist) {
		if err := s.initProjectDir(ctx, clean); err != nil {
			return Project{}, err
		}
		s.logger.Info("project meta initialized", "workdir", clean)
	} else if err != nil {
		return Project{}, fmt.Errorf("project: stat %s: %w", metaDir, err)
	}

	// 注册。
	name := filepath.Base(clean)
	now := time.Now().Format(time.RFC3339)
	res, err := s.registry.ExecContext(ctx,
		`INSERT INTO projects (name, workdir, created_at) VALUES (?, ?, ?)`,
		name, clean, now)
	if err != nil {
		return Project{}, fmt.Errorf("project: register %s: %w", clean, err)
	}
	id, _ := res.LastInsertId()
	s.logger.Info("project imported", "id", id, "name", name, "workdir", clean)
	return Project{ID: id, Name: name, Workdir: clean, CreatedAt: now}, nil
}

// Init 仅初始化 {workdir}/.taskboard/ 元数据（QA P4-1 Q6：MCP project_init 语义），
// **不注册**项目；幂等：目录已有 .taskboard/ 则直接返回。
func (s *Service) Init(ctx context.Context, workdir string) error {
	clean := filepath.Clean(workdir)
	if !filepath.IsAbs(clean) {
		return fmt.Errorf("%w: %s 不是绝对路径", ErrInvalidWorkdir, workdir)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return fmt.Errorf("%w: %s 不存在或不可访问", ErrInvalidWorkdir, workdir)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s 不是目录", ErrInvalidWorkdir, workdir)
	}
	metaDir := filepath.Join(clean, ".taskboard")
	if _, err := os.Stat(metaDir); err == nil {
		s.logger.Info("project meta already initialized (idempotent)", "workdir", clean)
		return nil
	}
	if err := s.initProjectDir(ctx, clean); err != nil {
		return err
	}
	s.logger.Info("project meta initialized", "workdir", clean)
	return nil
}

// ImportExisting 仅注册导入（QA P4-1 Q6：MCP project_import 语义），
// **要求目录已初始化**（存在 {workdir}/.taskboard/meta.db），否则返回错误引导 init/create。
// 幂等：已注册 → 返回已有记录。
func (s *Service) ImportExisting(ctx context.Context, workdir string) (Project, error) {
	clean := filepath.Clean(workdir)
	if !filepath.IsAbs(clean) {
		return Project{}, fmt.Errorf("%w: %s 不是绝对路径", ErrInvalidWorkdir, workdir)
	}
	if _, err := os.Stat(db.MetaDBPath(clean)); err != nil {
		return Project{}, fmt.Errorf("%w: %s 尚未初始化为项目（请先执行 init/create）", ErrInvalidWorkdir, workdir)
	}
	if existing, ok, err := s.findByWorkdir(ctx, clean); err != nil {
		return Project{}, err
	} else if ok {
		return existing, nil
	}
	name := filepath.Base(clean)
	now := time.Now().Format(time.RFC3339)
	res, err := s.registry.ExecContext(ctx,
		`INSERT INTO projects (name, workdir, created_at) VALUES (?, ?, ?)`,
		name, clean, now)
	if err != nil {
		return Project{}, fmt.Errorf("project: register %s: %w", clean, err)
	}
	id, _ := res.LastInsertId()
	s.logger.Info("project imported (existing meta)", "id", id, "name", name, "workdir", clean)
	return Project{ID: id, Name: name, Workdir: clean, CreatedAt: now}, nil
}

// Create 创建全新项目（QA P4-1 Q6：MCP project_create 语义）：先 Init（幂等），成功后再 ImportExisting。
func (s *Service) Create(ctx context.Context, workdir string) (Project, error) {
	if err := s.Init(ctx, workdir); err != nil {
		return Project{}, err
	}
	return s.ImportExisting(ctx, workdir)
}

// List 返回全部项目，按最近打开时间倒序（从未打开者排最后）。
func (s *Service) List(ctx context.Context) ([]Project, error) {
	rows, err := s.registry.QueryContext(ctx,
		`SELECT id, name, workdir, created_at, last_opened_at FROM projects
		 ORDER BY last_opened_at IS NULL, last_opened_at DESC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("project: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Project
	for rows.Next() {
		var p Project
		var last sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.Workdir, &p.CreatedAt, &last); err != nil {
			return nil, fmt.Errorf("project: scan: %w", err)
		}
		if last.Valid {
			p.LastOpenedAt = &last.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Remove 移除项目注册记录（仅删 projects 行，绝不删除/修改磁盘元数据）。
func (s *Service) Remove(ctx context.Context, id int64) error {
	res, err := s.registry.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("project: remove %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: id %d", ErrNotFound, id)
	}
	s.logger.Info("project record removed", "id", id)
	return nil
}

// Rename 更新项目显示名称（仅改 projects.name 行，不触碰磁盘与 workdir）。
// 名称去首尾空白；空名视为非法（ErrInvalidWorkdir 语义复用为参数错误）。
func (s *Service) Rename(ctx context.Context, id int64, name string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, fmt.Errorf("%w: 项目名称不能为空", ErrInvalidWorkdir)
	}
	res, err := s.registry.ExecContext(ctx, `UPDATE projects SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return Project{}, fmt.Errorf("project: rename %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Project{}, fmt.Errorf("%w: id %d", ErrNotFound, id)
	}
	// 返回更新后的记录（供响应回显）。
	row := s.registry.QueryRowContext(ctx,
		`SELECT id, name, workdir, created_at, last_opened_at FROM projects WHERE id = ?`, id)
	var p Project
	var last sql.NullString
	if err := row.Scan(&p.ID, &p.Name, &p.Workdir, &p.CreatedAt, &last); err != nil {
		return Project{}, fmt.Errorf("project: rename refetch %d: %w", id, err)
	}
	if last.Valid {
		p.LastOpenedAt = &last.String
	}
	s.logger.Info("project renamed", "id", id, "name", name)
	return p, nil
}

// Touch 更新项目最近打开时间（X-Project 校验命中时调用，QA Q12）。
// workdir 未注册时静默忽略（不视为错误，中间件已先校验存在）。
func (s *Service) Touch(ctx context.Context, workdir string) error {
	now := time.Now().Format(time.RFC3339)
	if _, err := s.registry.ExecContext(ctx,
		`UPDATE projects SET last_opened_at = ? WHERE workdir = ?`, now, filepath.Clean(workdir)); err != nil {
		return fmt.Errorf("project: touch %s: %w", workdir, err)
	}
	return nil
}

// CheckResult 目录导入前检查结果（TF-041 引导流程 Step 0）。
type CheckResult struct {
	// Registered 目录是否已注册为项目。
	Registered bool `json:"registered"`
	// HasMeta 是否存在 {workdir}/.taskboard/ 历史遗留元数据。
	HasMeta bool `json:"has_meta"`
	// MetaValid 元数据是否合法（config.yaml 可解析且状态机非空）。
	// 非法 = 版本过旧/损坏 → 引导提示清空重来。
	MetaValid bool `json:"meta_valid"`
	// MetaReason 非法原因（人类可读，供 UI 展示）。
	MetaReason string `json:"meta_reason,omitempty"`
}

// Check 检查目录是否可导入（TF-041）：已注册 / 历史元数据 / 元数据合法性。
// 目录不存在/非目录/非绝对路径 → ErrInvalidWorkdir。
func (s *Service) Check(ctx context.Context, workdir string) (CheckResult, error) {
	clean := filepath.Clean(workdir)
	if !filepath.IsAbs(clean) {
		return CheckResult{}, fmt.Errorf("%w: %s 不是绝对路径", ErrInvalidWorkdir, workdir)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return CheckResult{}, fmt.Errorf("%w: %s 不存在或不可访问", ErrInvalidWorkdir, workdir)
	}
	if !info.IsDir() {
		return CheckResult{}, fmt.Errorf("%w: %s 不是目录", ErrInvalidWorkdir, workdir)
	}

	res := CheckResult{}
	if _, ok, err := s.findByWorkdir(ctx, clean); err != nil {
		return res, err
	} else {
		res.Registered = ok
	}

	metaDir := filepath.Join(clean, ".taskboard")
	if _, err := os.Stat(metaDir); errors.Is(err, os.ErrNotExist) {
		return res, nil // 无历史元数据 → 正常初始化
	} else if err != nil {
		return res, fmt.Errorf("project: stat %s: %w", metaDir, err)
	}
	res.HasMeta = true

	// 元数据合法性：config.yaml 可解析 + 状态机非空 + meta.db 存在。
	cfgPath := config.ProjectConfigPath(clean)
	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
		res.MetaValid = false
		res.MetaReason = "config.yaml 缺失（元数据版本过旧或损坏）"
		return res, nil
	}
	cfg, err := config.LoadProjectFile(cfgPath)
	if err != nil {
		res.MetaValid = false
		res.MetaReason = fmt.Sprintf("config.yaml 解析失败: %v", err)
		return res, nil
	}
	if len(cfg.StateMachine.States) == 0 {
		res.MetaValid = false
		res.MetaReason = "状态机为空（元数据版本过旧或损坏）"
		return res, nil
	}
	// meta.db 存在性（核心业务库）。
	if _, err := os.Stat(db.MetaDBPath(clean)); err != nil {
		res.MetaValid = false
		res.MetaReason = "meta.db 缺失（元数据损坏）"
		return res, nil
	}
	res.MetaValid = true
	return res, nil
}

// ResetMetadata 清空 {workdir}/.taskboard/ 历史元数据（TF-041 引导流程：
// 元数据版本过旧/损坏时用户确认后重置）。仅删除元数据目录，
// 不触碰用户工作目录其他内容；目录不存在视为成功（幂等）。
func (s *Service) ResetMetadata(ctx context.Context, workdir string) error {
	clean := filepath.Clean(workdir)
	if !filepath.IsAbs(clean) {
		return fmt.Errorf("%w: %s 不是绝对路径", ErrInvalidWorkdir, workdir)
	}
	// 已注册的项目不允许直接重置元数据（会破坏数据）——先移除注册再清。
	// 引导流程中调用前已校验未注册（Check.Registered=false），此处防御。
	if _, ok, err := s.findByWorkdir(ctx, clean); err != nil {
		return err
	} else if ok {
		return fmt.Errorf("%w: %s 已注册为项目，禁止重置元数据", ErrInvalidWorkdir, workdir)
	}
	metaDir := filepath.Join(clean, ".taskboard")
	if err := os.RemoveAll(metaDir); err != nil {
		return fmt.Errorf("project: reset metadata %s: %w", metaDir, err)
	}
	s.logger.Info("project metadata reset", "workdir", clean)
	return nil
}

// findByWorkdir 按工作目录查注册记录。
func (s *Service) findByWorkdir(ctx context.Context, workdir string) (Project, bool, error) {
	var p Project
	var last sql.NullString
	err := s.registry.QueryRowContext(ctx,
		`SELECT id, name, workdir, created_at, last_opened_at FROM projects WHERE workdir = ?`,
		workdir).Scan(&p.ID, &p.Name, &p.Workdir, &p.CreatedAt, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, false, nil
	}
	if err != nil {
		return Project{}, false, fmt.Errorf("project: find %s: %w", workdir, err)
	}
	if last.Valid {
		p.LastOpenedAt = &last.String
	}
	return p, true, nil
}

// initProjectDir 初始化 {workdir}/.taskboard/（QA Q11）：
// meta.db（4 表，TF-033 v3 移除 skills 表）+ config.yaml（默认状态机 + export）+ 默认 Agent 只读权限。
func (s *Service) initProjectDir(ctx context.Context, workdir string) error {
	metaDir := filepath.Join(workdir, ".taskboard")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return fmt.Errorf("project: mkdir %s: %w", metaDir, err)
	}

	// 1) 项目库（4 表；skills 表已在 v3 迁移删除，TF-033）。
	projDB, err := db.EnsureProject(ctx, db.MetaDBPath(workdir))
	if err != nil {
		return fmt.Errorf("project: init meta.db: %w", err)
	}
	defer func() { _ = projDB.Close() }()

	// 2) 默认权限：v1 动作全集显式写入（默认只读 5 项 true，其余 false）。
	if err := s.writeDefaultPermissions(ctx, projDB); err != nil {
		return fmt.Errorf("project: write default permissions: %w", err)
	}

	// 3) 默认 config.yaml（状态机 + export）。
	if err := config.SaveProject(workdir, config.DefaultProjectConfig()); err != nil {
		return fmt.Errorf("project: init config.yaml: %w", err)
	}
	return nil
}

// writeDefaultPermissions 向项目库 permissions 表写入 v1 动作全集。
func (s *Service) writeDefaultPermissions(ctx context.Context, projDB *sql.DB) error {
	// 初始化时项目尚未拿到注册表 id：permissions.project_id 语义为「本项目」，
	// 统一写入 1（项目库内 project_id 仅作文档性冗余，一致性由应用层维护，见 db 包文档）。
	const localProjectID = 1
	tx, err := projDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, action := range AllActions {
		allowed := 0
		if DefaultGrantedActions[action] {
			allowed = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO permissions (project_id, action, allowed) VALUES (?, ?, ?)`,
			localProjectID, action, allowed); err != nil {
			return fmt.Errorf("insert permission %s: %w", action, err)
		}
	}
	return tx.Commit()
}
