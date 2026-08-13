package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Migration 定义一次数据库迁移：向上应用（Up）与向下回退（Down）各一个事务。
//
// 版本号全项目唯一且递增；已发布的 Migration 定义不得修改，新增变更以更高版本追加
// （对应 REQUIREMENTS.md §5.2「数据库迁移机制」扩展约定）。
type Migration struct {
	Version int
	Name    string
	Up      func(ctx context.Context, tx *sql.Tx) error
	Down    func(ctx context.Context, tx *sql.Tx) error
}

// ErrNoMigrations 表示传入空迁移集合。
var ErrNoMigrations = errors.New("db: empty migrations")

// ErrMigrationNotFound 表示目标回退版本在迁移集合中不存在。
var ErrMigrationNotFound = errors.New("db: migration not found")

// schemaMigrationsDDL 版本管理表（记录每次成功应用的迁移版本）。
const schemaMigrationsDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version    INTEGER PRIMARY KEY,
	name       TEXT NOT NULL,
	applied_at TEXT NOT NULL
)`

// GlobalMigrations 全局注册表库迁移集合。
//
// v1：projects 表（项目注册表，仅记录「项目名称 + 工作目录」，不含业务数据；
// 移除记录绝不删除磁盘 .taskboard/ 数据）。
// v2（TF-043）：projects 加 hidden 列——导入默认隐藏（引导完成前不在列表展示），
// 走完导入引导（欢迎页）后置 0 可见；AI 入口（MCP project_import/create）注册即可见。
var GlobalMigrations = []Migration{
	{
		Version: 1,
		Name:    "create_projects_table",
		Up: func(_ context.Context, tx *sql.Tx) error {
			_, err := tx.Exec(`CREATE TABLE projects (
				id             INTEGER PRIMARY KEY AUTOINCREMENT,
				name           TEXT NOT NULL,
				workdir        TEXT NOT NULL UNIQUE,
				created_at     TEXT NOT NULL,
				last_opened_at TEXT
			)`)
			return err
		},
		Down: func(_ context.Context, tx *sql.Tx) error {
			_, err := tx.Exec(`DROP TABLE IF EXISTS projects`)
			return err
		},
	},
	{
		Version: 2,
		Name:    "add_projects_hidden",
		Up: func(_ context.Context, tx *sql.Tx) error {
			// 存量项目视为已完成引导（可见）；新导入默认 hidden=1（引导中暂时隐藏）。
			_, err := tx.Exec(`ALTER TABLE projects ADD COLUMN hidden INTEGER NOT NULL DEFAULT 1`)
			if err != nil {
				return err
			}
			_, err = tx.Exec(`UPDATE projects SET hidden = 0`)
			return err
		},
		Down: func(_ context.Context, tx *sql.Tx) error {
			_, err := tx.Exec(`ALTER TABLE projects DROP COLUMN hidden`)
			return err
		},
	},
}

// ProjectMigrations 项目库迁移集合。
//
// v1：tasks / permissions / import_drafts / skills / audit_log 5 表 + 索引。
// 各表 project_id 字段保留全局注册表分配的项目 id，不建跨库外键（见包文档）。
// tasks.parent_id 为库内自引用；未启用 foreign_keys 强制（SQLite 默认 OFF）。
var ProjectMigrations = []Migration{
	{
		Version: 1,
		Name:    "create_project_tables",
		Up: func(_ context.Context, tx *sql.Tx) error {
			stmts := []string{
				`CREATE TABLE tasks (
					id            TEXT PRIMARY KEY,
					project_id    INTEGER NOT NULL,
					parent_id     TEXT REFERENCES tasks(id),
					title         TEXT NOT NULL,
					description   TEXT NOT NULL DEFAULT '',
					status        TEXT NOT NULL,
					priority      INTEGER NOT NULL DEFAULT 0,
					tags          TEXT NOT NULL DEFAULT '[]',
					assignee      TEXT NOT NULL DEFAULT '',
					depends_on    TEXT NOT NULL DEFAULT '[]',
					archived_from TEXT,
					source_file   TEXT,
					source_section TEXT,
					created_at    TEXT NOT NULL,
					updated_at    TEXT NOT NULL
				)`,
				`CREATE INDEX idx_tasks_project ON tasks(project_id)`,
				`CREATE INDEX idx_tasks_status ON tasks(project_id, status)`,
				`CREATE TABLE permissions (
					id         INTEGER PRIMARY KEY AUTOINCREMENT,
					project_id INTEGER NOT NULL,
					action     TEXT NOT NULL,
					allowed    INTEGER NOT NULL DEFAULT 0,
					UNIQUE(project_id, action)
				)`,
				`CREATE TABLE import_drafts (
					id           TEXT PRIMARY KEY,
					project_id   INTEGER NOT NULL,
					source_file  TEXT NOT NULL,
					parsed_json  TEXT NOT NULL,
					status       TEXT NOT NULL DEFAULT 'pending',
					created_at   TEXT NOT NULL,
					confirmed_at TEXT
				)`,
				`CREATE TABLE skills (
					name       TEXT PRIMARY KEY,
					content    TEXT NOT NULL,
					updated_at TEXT NOT NULL
				)`,
				`CREATE TABLE audit_log (
					id          INTEGER PRIMARY KEY AUTOINCREMENT,
					ts          TEXT NOT NULL,
					actor       TEXT NOT NULL,
					actor_class TEXT NOT NULL,
					action      TEXT NOT NULL,
					target      TEXT NOT NULL,
					result      TEXT NOT NULL,
					detail      TEXT
				)`,
				`CREATE INDEX idx_audit_project ON audit_log(action, ts)`,
			}
			for _, stmt := range stmts {
				if _, err := tx.Exec(stmt); err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(_ context.Context, tx *sql.Tx) error {
			stmts := []string{
				`DROP INDEX IF EXISTS idx_audit_project`,
				`DROP TABLE IF EXISTS audit_log`,
				`DROP TABLE IF EXISTS skills`,
				`DROP TABLE IF EXISTS import_drafts`,
				`DROP TABLE IF EXISTS permissions`,
				`DROP INDEX IF EXISTS idx_tasks_status`,
				`DROP INDEX IF EXISTS idx_tasks_project`,
				`DROP TABLE IF EXISTS tasks`,
			}
			for _, stmt := range stmts {
				if _, err := tx.Exec(stmt); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		// v2（TF-020）：skills 表扩展结构化元数据列（version/description/instructions），
		// 支撑 YAML 双格式 skill_info 返回；文件系统仍为唯一数据源，表仅缓存。
		Version: 2,
		Name:    "extend_skills_meta",
		Up: func(_ context.Context, tx *sql.Tx) error {
			for _, stmt := range []string{
				`ALTER TABLE skills ADD COLUMN version TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE skills ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE skills ADD COLUMN instructions TEXT NOT NULL DEFAULT ''`,
			} {
				if _, err := tx.Exec(stmt); err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(_ context.Context, tx *sql.Tx) error {
			// SQLite 旧版不支持 DROP COLUMN 链式执行；重建表回退到 v1 结构。
			stmts := []string{
				`CREATE TABLE skills_v1 (
					name       TEXT PRIMARY KEY,
					content    TEXT NOT NULL,
					updated_at TEXT NOT NULL
				)`,
				`INSERT INTO skills_v1 (name, content, updated_at) SELECT name, content, updated_at FROM skills`,
				`DROP TABLE skills`,
				`ALTER TABLE skills_v1 RENAME TO skills`,
			}
			for _, stmt := range stmts {
				if _, err := tx.Exec(stmt); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		// v3（TF-033）：移除 skills 表——Skill 功能重设计（docs/task/SKILLS-REDESIGN.md，
		// 用户确认 Q2）：彻底废弃 .taskboard/skills/ 文件扫描与 skills 缓存表，
		// 技能包改为内置 embed + 全局技能库（~/.taskboard-app/skills/），
		// 安装状态实时扫描宿主位置（无状态设计，不依赖项目库）。
		Version: 3,
		Name:    "drop_skills_table",
		Up: func(_ context.Context, tx *sql.Tx) error {
			_, err := tx.Exec(`DROP TABLE IF EXISTS skills`)
			return err
		},
		Down: func(_ context.Context, tx *sql.Tx) error {
			// 回退：重建 v1 结构 skills 表（原缓存表；TF-033 起不再读写，重建仅保迁移可逆）。
			_, err := tx.Exec(`CREATE TABLE skills (
				name       TEXT PRIMARY KEY,
				content    TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`)
			return err
		},
	},
	{
		// v4（TF-040）：任务简短编号 number（如 T01）。加列 → 按 created_at 顺序回填
		// 存量任务 → 唯一索引（编号必须唯一，创建/导入自动分配）。
		Version: 4,
		Name:    "add_task_number",
		Up: func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx,
				`ALTER TABLE tasks ADD COLUMN number TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
			// 存量任务回填：按创建顺序分配 T01..Tn（NULL 或空号）。
			rows, err := tx.QueryContext(ctx,
				`SELECT id, ROW_NUMBER() OVER (ORDER BY created_at, id) FROM tasks`)
			if err != nil {
				return err
			}
			var ids []string
			var seq []int64
			for rows.Next() {
				var id string
				var n int64
				if err := rows.Scan(&id, &n); err != nil {
					_ = rows.Close()
					return err
				}
				ids = append(ids, id)
				seq = append(seq, n)
			}
			if err := rows.Close(); err != nil {
				return err
			}
			for i, id := range ids {
				num := fmt.Sprintf("T%03d", seq[i])
				if _, err := tx.ExecContext(ctx,
					`UPDATE tasks SET number = ? WHERE id = ?`, num, id); err != nil {
					return err
				}
			}
			_, err = tx.ExecContext(ctx,
				`CREATE UNIQUE INDEX idx_tasks_number ON tasks(number)`)
			return err
		},
		Down: func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS idx_tasks_number`); err != nil {
				return err
			}
			// SQLite 旧版不支持 DROP COLUMN；重建表回退到 v3 结构。
			stmts := []string{
				`CREATE TABLE tasks_v3 (
					id            TEXT PRIMARY KEY,
					project_id    INTEGER NOT NULL,
					parent_id     TEXT REFERENCES tasks(id),
					title         TEXT NOT NULL,
					description   TEXT NOT NULL DEFAULT '',
					status        TEXT NOT NULL,
					priority      INTEGER NOT NULL DEFAULT 0,
					tags          TEXT NOT NULL DEFAULT '[]',
					assignee      TEXT NOT NULL DEFAULT '',
					depends_on    TEXT NOT NULL DEFAULT '[]',
					archived_from TEXT,
					source_file   TEXT,
					source_section TEXT,
					created_at    TEXT NOT NULL,
					updated_at    TEXT NOT NULL
				)`,
				`INSERT INTO tasks_v3 (id, project_id, parent_id, title, description, status, priority,
					tags, assignee, depends_on, archived_from, source_file, source_section, created_at, updated_at)
					SELECT id, project_id, parent_id, title, description, status, priority,
					tags, assignee, depends_on, archived_from, source_file, source_section, created_at, updated_at
					FROM tasks`,
				`DROP TABLE tasks`,
				`ALTER TABLE tasks_v3 RENAME TO tasks`,
				`CREATE INDEX idx_tasks_project ON tasks(project_id)`,
				`CREATE INDEX idx_tasks_status ON tasks(project_id, status)`,
			}
			for _, stmt := range stmts {
				if _, err := tx.Exec(stmt); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		// v5（TF-044）：知识库 5 表（docs/KNOWLEDGE-BASE.md §3）。
		// 知识库 = 文档引用注册表 + 任务关联 + 语义索引（摘要/向量），不存原文；
		// knowledge_documents 按 (project_id, abs_path) 唯一，多库/多任务共享单向量。
		// 命名多库：默认库由项目初始化流程创建（is_default=1），此处仅建 schema。
		Version: 5,
		Name:    "create_knowledge_tables",
		Up: func(ctx context.Context, tx *sql.Tx) error {
			stmts := []string{
				`CREATE TABLE knowledge_bases (
					id          INTEGER PRIMARY KEY AUTOINCREMENT,
					project_id  INTEGER NOT NULL DEFAULT 1,
					name        TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					is_default  INTEGER NOT NULL DEFAULT 0,
					created_at  TEXT NOT NULL,
					updated_at  TEXT NOT NULL,
					UNIQUE(project_id, name)
				)`,
				`CREATE TABLE knowledge_documents (
					id             TEXT PRIMARY KEY,
					project_id     INTEGER NOT NULL DEFAULT 1,
					path           TEXT NOT NULL,
					abs_path       TEXT NOT NULL,
					rel_path       TEXT,
					origin_path    TEXT,
					display_name   TEXT NOT NULL,
					type           TEXT NOT NULL DEFAULT 'text',
					size           INTEGER NOT NULL DEFAULT 0,
					mtime          TEXT,
					content_hash   TEXT,
					summary        TEXT NOT NULL DEFAULT '',
					status         TEXT NOT NULL DEFAULT 'ok',
					embedded       INTEGER NOT NULL DEFAULT 0,
					embedding_model TEXT,
					index_error    TEXT,
					history        TEXT NOT NULL DEFAULT '[]',
					created_at     TEXT NOT NULL,
					updated_at     TEXT NOT NULL
				)`,
				`CREATE UNIQUE INDEX idx_kd_abs ON knowledge_documents(project_id, abs_path)`,
				`CREATE INDEX idx_kd_status ON knowledge_documents(status)`,
				`CREATE TABLE knowledge_base_documents (
					kb_id       INTEGER NOT NULL REFERENCES knowledge_bases(id),
					document_id TEXT NOT NULL REFERENCES knowledge_documents(id),
					created_at  TEXT NOT NULL,
					PRIMARY KEY (kb_id, document_id)
				)`,
				`CREATE TABLE task_documents (
					task_id     TEXT NOT NULL,
					document_id TEXT NOT NULL REFERENCES knowledge_documents(id),
					created_at  TEXT NOT NULL,
					PRIMARY KEY (task_id, document_id)
				)`,
				`CREATE INDEX idx_td_doc ON task_documents(document_id)`,
				`CREATE INDEX idx_td_task ON task_documents(task_id)`,
				`CREATE TABLE knowledge_chunks (
					id          TEXT PRIMARY KEY,
					document_id TEXT NOT NULL REFERENCES knowledge_documents(id),
					seq         INTEGER NOT NULL,
					heading     TEXT NOT NULL DEFAULT '',
					content     TEXT NOT NULL,
					vector      BLOB NOT NULL,
					dim         INTEGER NOT NULL,
					created_at  TEXT NOT NULL,
					UNIQUE(document_id, seq)
				)`,
				`CREATE INDEX idx_kc_doc ON knowledge_chunks(document_id)`,
			}
			for _, stmt := range stmts {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(ctx context.Context, tx *sql.Tx) error {
			stmts := []string{
				`DROP INDEX IF EXISTS idx_kc_doc`,
				`DROP TABLE IF EXISTS knowledge_chunks`,
				`DROP INDEX IF EXISTS idx_td_task`,
				`DROP INDEX IF EXISTS idx_td_doc`,
				`DROP TABLE IF EXISTS task_documents`,
				`DROP TABLE IF EXISTS knowledge_base_documents`,
				`DROP INDEX IF EXISTS idx_kd_status`,
				`DROP INDEX IF EXISTS idx_kd_abs`,
				`DROP TABLE IF EXISTS knowledge_documents`,
				`DROP TABLE IF EXISTS knowledge_bases`,
			}
			for _, stmt := range stmts {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		// v6（TF-052 归档）：knowledge_documents 增加 archived 列。
		// 归档语义：文档从默认列表/检索/扫描隐藏，但任务引用（task_documents）与文件保留可访问。
		Version: 6,
		Name:    "add_document_archived",
		Up: func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx,
				`ALTER TABLE knowledge_documents ADD COLUMN archived INTEGER NOT NULL DEFAULT 0`); err != nil {
				return err
			}
			return nil
		},
		Down: func(ctx context.Context, tx *sql.Tx) error {
			// SQLite 不支持 DROP COLUMN；重建表回退到 v5 结构（无 archived 列）。
			stmts := []string{
				`CREATE TABLE knowledge_documents_v5 (
					id             TEXT PRIMARY KEY,
					project_id     INTEGER NOT NULL DEFAULT 1,
					path           TEXT NOT NULL,
					abs_path       TEXT NOT NULL,
					rel_path       TEXT,
					origin_path    TEXT,
					display_name   TEXT NOT NULL,
					type           TEXT NOT NULL DEFAULT 'text',
					size           INTEGER NOT NULL DEFAULT 0,
					mtime          TEXT,
					content_hash   TEXT,
					summary        TEXT NOT NULL DEFAULT '',
					status         TEXT NOT NULL DEFAULT 'ok',
					embedded       INTEGER NOT NULL DEFAULT 0,
					embedding_model TEXT,
					index_error    TEXT,
					history        TEXT NOT NULL DEFAULT '[]',
					created_at     TEXT NOT NULL,
					updated_at     TEXT NOT NULL
				)`,
				`INSERT INTO knowledge_documents_v5 (id, project_id, path, abs_path, rel_path, origin_path,
					display_name, type, size, mtime, content_hash, summary, status, embedded,
					embedding_model, index_error, history, created_at, updated_at)
					SELECT id, project_id, path, abs_path, rel_path, origin_path,
					display_name, type, size, mtime, content_hash, summary, status, embedded,
					embedding_model, index_error, history, created_at, updated_at
					FROM knowledge_documents`,
				`DROP TABLE knowledge_documents`,
				`ALTER TABLE knowledge_documents_v5 RENAME TO knowledge_documents`,
				`CREATE UNIQUE INDEX idx_kd_abs ON knowledge_documents(project_id, abs_path)`,
				`CREATE INDEX idx_kd_status ON knowledge_documents(status)`,
			}
			for _, stmt := range stmts {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return err
				}
			}
			return nil
		},
	},
}

// Migrate 将数据库向上迁移至迁移集合中的最新版本。
//
// 幂等：已应用版本自动跳过，重复调用不产生副作用；
// 每个迁移在独立事务中执行，失败回滚且不记录版本。
func Migrate(ctx context.Context, db *sql.DB, migrations []Migration) error {
	if len(migrations) == 0 {
		return ErrNoMigrations
	}
	if _, err := db.ExecContext(ctx, schemaMigrationsDDL); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	current, err := currentVersion(ctx, db)
	if err != nil {
		return err
	}
	sorted := sortedMigrations(migrations)
	for _, m := range sorted {
		if m.Version <= current {
			continue
		}
		if err := applyOne(ctx, db, m); err != nil {
			return fmt.Errorf("migrate up v%d (%s): %w", m.Version, m.Name, err)
		}
	}
	return nil
}

// MigrateDown 将数据库向下回退到目标版本（含）。
// to=0 表示回退全部迁移（表结构清空，仅保留 schema_migrations）。
// 每个迁移在独立事务中执行；目标版本必须存在于迁移集合。
func MigrateDown(ctx context.Context, db *sql.DB, migrations []Migration, to int) error {
	if _, err := db.ExecContext(ctx, schemaMigrationsDDL); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	current, err := currentVersion(ctx, db)
	if err != nil {
		return err
	}
	sorted := sortedMigrations(migrations)
	// 目标版本存在性校验。
	if to > 0 {
		found := false
		for _, m := range sorted {
			if m.Version == to {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: version %d", ErrMigrationNotFound, to)
		}
	}
	for i := len(sorted) - 1; i >= 0; i-- {
		m := sorted[i]
		if m.Version <= to {
			continue
		}
		if m.Version > current {
			// 跳过尚未应用的版本（防御性，正常流程不会出现）。
			continue
		}
		if err := rollbackOne(ctx, db, m); err != nil {
			return fmt.Errorf("migrate down v%d (%s): %w", m.Version, m.Name, err)
		}
	}
	return nil
}

// CurrentVersion 返回当前已应用的最大迁移版本；无迁移时返回 0。
func CurrentVersion(ctx context.Context, db *sql.DB) (int, error) {
	if err := db.PingContext(ctx); err != nil {
		return 0, err
	}
	// schema_migrations 可能不存在（从未迁移），此时返回 0 不视为错误。
	if _, err := db.ExecContext(ctx, schemaMigrationsDDL); err != nil {
		return 0, fmt.Errorf("ensure schema_migrations: %w", err)
	}
	return currentVersion(ctx, db)
}

// currentVersion 读取 schema_migrations 的最大版本（表已确保存在）。
func currentVersion(ctx context.Context, db *sql.DB) (int, error) {
	var v sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, fmt.Errorf("read current version: %w", err)
	}
	return int(v.Int64), nil
}

// applyOne 在单事务中应用一个迁移并记录版本。
func applyOne(ctx context.Context, db *sql.DB, m Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := m.Up(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.Version, m.Name, time.Now().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
}

// rollbackOne 在单事务中回退一个迁移并删除版本记录。
func rollbackOne(ctx context.Context, db *sql.DB, m Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := m.Down(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = ?`, m.Version); err != nil {
		return err
	}
	return tx.Commit()
}

// sortedMigrations 按版本号升序排列迁移集合。
func sortedMigrations(migrations []Migration) []Migration {
	out := make([]Migration, len(migrations))
	copy(out, migrations)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}
