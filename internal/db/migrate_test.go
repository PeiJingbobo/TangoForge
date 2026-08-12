package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// openMem 打开独立内存库（sqlite:memory: 隔离，不依赖文件系统）。
func openMem(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// assertTableExists 断言指定表存在（替代 sqlite3 CLI 的表结构验收）。
func assertTableExists(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&n)
	if err != nil {
		t.Fatalf("query table %s: %v", name, err)
	}
	if n != 1 {
		t.Errorf("table %s should exist, got %d", name, n)
	}
}

// assertTableAbsent 断言指定表不存在。
func assertTableAbsent(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&n)
	if err != nil {
		t.Fatalf("query table %s: %v", name, err)
	}
	if n != 0 {
		t.Errorf("table %s should not exist, got %d", name, n)
	}
}

// assertIndexExists 断言指定索引存在。
func assertIndexExists(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, name,
	).Scan(&n)
	if err != nil {
		t.Fatalf("query index %s: %v", name, err)
	}
	if n != 1 {
		t.Errorf("index %s should exist, got %d", name, n)
	}
}

// assertColumns 断言表包含全部指定列（PRAGMA table_info）。
func assertColumns(t *testing.T, db *sql.DB, table string, want ...string) {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("pragma table_info(%s): %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	got := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		got[name] = true
	}
	for _, c := range want {
		if !got[c] {
			t.Errorf("table %s missing column %q", table, c)
		}
	}
}

func TestMigrate_GlobalCreatesProjectsTable(t *testing.T) {
	conn := openMem(t)
	if err := Migrate(context.Background(), conn, GlobalMigrations); err != nil {
		t.Fatalf("migrate global: %v", err)
	}
	assertTableExists(t, conn, "projects")
	assertColumns(t, conn, "projects", "id", "name", "workdir", "created_at", "last_opened_at")
	// schema_migrations 自身存在。
	assertTableExists(t, conn, "schema_migrations")
}

func TestMigrate_ProjectCreatesAllTablesAndIndexes(t *testing.T) {
	conn := openMem(t)
	if err := Migrate(context.Background(), conn, ProjectMigrations); err != nil {
		t.Fatalf("migrate project: %v", err)
	}
	// 5 张业务表（含 schema_migrations）均存在；skills 表在 v3 已移除（TF-033）。
	for _, table := range []string{"tasks", "permissions", "import_drafts", "audit_log", "schema_migrations"} {
		assertTableExists(t, conn, table)
	}
	// 关键列校验（与 REQUIREMENTS.md §四.4 DDL 一致）。
	assertColumns(t, conn, "tasks",
		"id", "project_id", "parent_id", "title", "description", "status",
		"priority", "tags", "assignee", "depends_on", "archived_from",
		"source_file", "source_section", "created_at", "updated_at")
	assertColumns(t, conn, "permissions", "id", "project_id", "action", "allowed")
	assertColumns(t, conn, "import_drafts", "id", "project_id", "source_file", "parsed_json", "status", "created_at", "confirmed_at")
	assertColumns(t, conn, "audit_log", "id", "ts", "actor", "actor_class", "action", "target", "result", "detail")
	// skills 表已被 v3 删除（TF-033：技能包改为内置 embed + 全局库）。
	assertTableAbsent(t, conn, "skills")
	// 索引齐全。
	assertIndexExists(t, conn, "idx_tasks_project")
	assertIndexExists(t, conn, "idx_tasks_status")
	assertIndexExists(t, conn, "idx_audit_project")
}

func TestMigrate_Idempotent(t *testing.T) {
	conn := openMem(t)
	ctx := context.Background()
	if err := Migrate(ctx, conn, ProjectMigrations); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	v1, err := CurrentVersion(ctx, conn)
	if err != nil {
		t.Fatalf("current version: %v", err)
	}
	// 重复迁移不报错、版本不变。
	if err := Migrate(ctx, conn, ProjectMigrations); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	v2, err := CurrentVersion(ctx, conn)
	if err != nil {
		t.Fatalf("current version: %v", err)
	}
	if v1 != v2 || v2 != 4 {
		t.Errorf("version should stay 4 after idempotent migrate, got %d -> %d", v1, v2)
	}
}

func TestMigrate_UpDownRoundTrip(t *testing.T) {
	conn := openMem(t)
	ctx := context.Background()
	if err := Migrate(ctx, conn, ProjectMigrations); err != nil {
		t.Fatalf("up: %v", err)
	}
	assertTableExists(t, conn, "tasks")
	// 全部回退。
	if err := MigrateDown(ctx, conn, ProjectMigrations, 0); err != nil {
		t.Fatalf("down to 0: %v", err)
	}
	for _, table := range []string{"tasks", "permissions", "import_drafts", "skills", "audit_log"} {
		assertTableAbsent(t, conn, table)
	}
	v, err := CurrentVersion(ctx, conn)
	if err != nil {
		t.Fatalf("current version after down: %v", err)
	}
	if v != 0 {
		t.Errorf("version should be 0 after full rollback, got %d", v)
	}
	// 重新向上，表恢复。
	if err := Migrate(ctx, conn, ProjectMigrations); err != nil {
		t.Fatalf("re-up: %v", err)
	}
	assertTableExists(t, conn, "tasks")
}

// multiVersions 构造 v1+v2 两段迁移，验证框架的多版本顺序与回退。
func multiVersions(t *testing.T) []Migration {
	t.Helper()
	return []Migration{
		{
			Version: 1,
			Name:    "v1_create_a",
			Up: func(_ context.Context, tx *sql.Tx) error {
				_, err := tx.Exec(`CREATE TABLE tbl_a (id INTEGER PRIMARY KEY)`)
				return err
			},
			Down: func(_ context.Context, tx *sql.Tx) error {
				_, err := tx.Exec(`DROP TABLE IF EXISTS tbl_a`)
				return err
			},
		},
		{
			Version: 2,
			Name:    "v2_create_b",
			Up: func(_ context.Context, tx *sql.Tx) error {
				_, err := tx.Exec(`CREATE TABLE tbl_b (id INTEGER PRIMARY KEY)`)
				return err
			},
			Down: func(_ context.Context, tx *sql.Tx) error {
				_, err := tx.Exec(`DROP TABLE IF EXISTS tbl_b`)
				return err
			},
		},
	}
}

func TestMigrate_MultiVersionOrderAndPartialRollback(t *testing.T) {
	conn := openMem(t)
	ctx := context.Background()
	migs := multiVersions(t)
	if err := Migrate(ctx, conn, migs); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	assertTableExists(t, conn, "tbl_a")
	assertTableExists(t, conn, "tbl_b")
	v, _ := CurrentVersion(ctx, conn)
	if v != 2 {
		t.Fatalf("version should be 2, got %d", v)
	}
	// 回退到 v1：只删 tbl_b。
	if err := MigrateDown(ctx, conn, migs, 1); err != nil {
		t.Fatalf("partial down: %v", err)
	}
	assertTableExists(t, conn, "tbl_a")
	assertTableAbsent(t, conn, "tbl_b")
	v, _ = CurrentVersion(ctx, conn)
	if v != 1 {
		t.Errorf("version should be 1, got %d", v)
	}
}

func TestMigrateDown_InvalidTarget(t *testing.T) {
	conn := openMem(t)
	ctx := context.Background()
	migs := multiVersions(t)
	if err := Migrate(ctx, conn, migs); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	err := MigrateDown(ctx, conn, migs, 99)
	if !errors.Is(err, ErrMigrationNotFound) {
		t.Errorf("expected ErrMigrationNotFound, got %v", err)
	}
}

func TestMigrate_EmptyMigrations(t *testing.T) {
	conn := openMem(t)
	if err := Migrate(context.Background(), conn, nil); !errors.Is(err, ErrNoMigrations) {
		t.Errorf("expected ErrNoMigrations, got %v", err)
	}
}

// TestEnsureGlobal_FileDB 验证临时目录建全局库（TF-001 验收：临时目录初始化后表可查）。
func TestEnsureGlobal_FileDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.db")
	conn, err := EnsureGlobal(context.Background(), path)
	if err != nil {
		t.Fatalf("ensure global: %v", err)
	}
	defer func() { _ = conn.Close() }()
	assertTableExists(t, conn, "projects")
}

// TestEnsureProject_FileDB 验证临时目录建项目库：meta.db 文件 + 4 表（TF-033 v3 移除 skills）+ 索引齐全。
func TestEnsureProject_FileDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.db")
	conn, err := EnsureProject(context.Background(), path)
	if err != nil {
		t.Fatalf("ensure project: %v", err)
	}
	defer func() { _ = conn.Close() }()
	for _, table := range []string{"tasks", "permissions", "import_drafts", "audit_log"} {
		assertTableExists(t, conn, table)
	}
	// skills 表已由 v3 迁移移除（TF-033：技能包改为内置 embed + 全局库，无项目库依赖）。
	assertTableAbsent(t, conn, "skills")
	assertIndexExists(t, conn, "idx_tasks_project")
	assertIndexExists(t, conn, "idx_tasks_status")
	assertIndexExists(t, conn, "idx_audit_project")
}

func TestOpen_WALMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal.db")
	conn, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()
	var mode string
	if err := conn.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode should be wal, got %q", mode)
	}
}

func TestMetaAndRegistryPaths(t *testing.T) {
	if got := MetaDBPath(`D:\work\proj`); got != filepath.Join(`D:\work\proj`, ".taskboard", "meta.db") {
		t.Errorf("MetaDBPath mismatch: %s", got)
	}
	if got := RegistryDBPath(`C:\Users\me`); got != filepath.Join(`C:\Users\me`, ".taskboard-app", "registry.db") {
		t.Errorf("RegistryDBPath mismatch: %s", got)
	}
}

// TestFileFingerprint_SameAs TF-001 回归：meta.db 被删除重建（inode 变化）后
// 缓存指纹必须失效，防止业务层连接仍指向回收站旧库。
func TestFileFingerprint_SameAs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.db")

	// 先写入旧文件，捕获指纹。
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	fp, err := CaptureFingerprint(path)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if !fp.SameAs(path) {
		t.Fatal("指纹应匹配自身")
	}

	// 模拟 macOS 删除 .taskboard 目录 → 重建新库：删除旧文件并写入新内容。
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("write new: %v", err)
	}
	if fp.SameAs(path) {
		t.Fatal("删除重建后指纹应失效（SameAs=false）")
	}

	// 文件不存在 → SameAs=false。
	missing := filepath.Join(dir, "nope.db")
	if fp.SameAs(missing) {
		t.Fatal("文件不存在应 SameAs=false")
	}
}
