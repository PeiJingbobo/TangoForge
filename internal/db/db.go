// Package db 是 TangoForge 的 SQLite 基础设施层：连接管理、迁移（up/down）、版本管理。
//
// 分层铁律（AGENTS.md §3.1）：本层仅提供原生 SQL 与迁移能力，不做业务判断；
// 数据库事务边界由业务层（task / project 等）控制。
//
// 双库模型（QA Q4 已确认）：
//   - 全局注册表库（~/.taskboard-app/registry.db）：仅 projects 表，是守护进程枚举全部项目的唯一来源；
//   - 项目库（{workdir}/.taskboard/meta.db）：tasks / permissions / import_drafts / skills / audit_log 5 表；
//   - 项目库中各表的 project_id 字段保留全局注册表分配的项目 id，
//     项目库内不建跨库外键（一致性由应用层维护，SQLite 默认不启用 foreign_keys 强制）。
//
// 全部代码 CGO_ENABLED=0 可编译（依赖 modernc.org/sqlite 纯 Go 实现）。
package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动（零信任依赖约束）
)

// 默认连接参数（QA Q7 时间戳 TEXT 列由业务层写入 RFC3339 本地时区）。
const (
	// busyTimeout 连接等待锁的超时（毫秒），避免并发写冲突时立刻报 SQLITE_BUSY。
	busyTimeout = 5000
)

// Open 打开（必要时创建）SQLite 数据库，配置 WAL 与 busy_timeout，不执行迁移。
//
// path 为数据库文件绝对路径；":memory:" 用于单元测试隔离。
// 调用方负责在业务结束后 Close。
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)", path, busyTimeout)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// 本地单进程场景下限制连接数，避免 SQLite 文件锁竞争。
	conn.SetMaxOpenConns(1)
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping sqlite %s: %w", path, err)
	}
	return conn, nil
}

// EnsureGlobal 打开全局注册表库并迁移至最新版本（守护进程启动与 TF-004 项目管理共用）。
func EnsureGlobal(ctx context.Context, path string) (*sql.DB, error) {
	conn, err := Open(path)
	if err != nil {
		return nil, err
	}
	if err := Migrate(ctx, conn, GlobalMigrations); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("migrate global db %s: %w", path, err)
	}
	return conn, nil
}

// EnsureProject 打开项目库并迁移至最新版本（TF-004 导入目录初始化时调用）。
func EnsureProject(ctx context.Context, path string) (*sql.DB, error) {
	conn, err := Open(path)
	if err != nil {
		return nil, err
	}
	if err := Migrate(ctx, conn, ProjectMigrations); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("migrate project db %s: %w", path, err)
	}
	return conn, nil
}

// MetaDBPath 返回项目目录下 .taskboard/meta.db 的绝对路径（显式优于隐式，禁止硬编码路径）。
func MetaDBPath(workdir string) string {
	return filepath.Join(workdir, ".taskboard", "meta.db")
}

// ProjectExists 检查工作目录是否已注册为项目（查询全局注册表库，原生 SQL）。
//
// 供传输层中间件（TF-003）与业务层（TF-004）共用；workdir 由调用方规范化后传入。
func ProjectExists(ctx context.Context, db *sql.DB, workdir string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM projects WHERE workdir = ?`, workdir).Scan(&n); err != nil {
		return false, fmt.Errorf("db: project exists: %w", err)
	}
	return n > 0, nil
}

// RegistryDBPath 返回全局注册表库的绝对路径（默认 ~/.taskboard-app/registry.db）。
//
// homeDir 由调用方提供（如 os.UserHomeDir），本层不做环境探测。
func RegistryDBPath(homeDir string) string {
	return filepath.Join(homeDir, ".taskboard-app", "registry.db")
}

// FileFingerprint 捕获文件系统指纹（跨平台：os.SameFile 语义的等价快照）。
//
// 用途：业务层按 workdir 缓存项目库连接时，记录打开时的文件指纹；
// 缓存命中后再次校验，发现 meta.db 被删除重建（inode 变化）则废弃旧连接重新打开，
// 防止连接仍指向已被移除/回收站中的旧文件（TF-001 根因：删除 .taskboard 后重建新库）。
type FileFingerprint struct {
	fi os.FileInfo
}

// CaptureFingerprint 捕获 path 当前的文件指纹（文件必须存在）。
func CaptureFingerprint(path string) (*FileFingerprint, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &FileFingerprint{fi: fi}, nil
}

// SameAs 判断 path 当前文件与缓存的指纹是否仍为同一文件（跨平台 inode/dev 比较）。
// 文件不存在视为不一致（返回 false）。
func (fp *FileFingerprint) SameAs(path string) bool {
	if fp == nil || fp.fi == nil {
		return false
	}
	cur, err := os.Stat(path)
	if err != nil {
		return false
	}
	return os.SameFile(fp.fi, cur)
}
