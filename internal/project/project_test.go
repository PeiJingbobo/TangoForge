package project

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tangoforge/internal/config"
	"tangoforge/internal/db"
)

func newService(t *testing.T) *Service {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(context.Background(), conn, db.GlobalMigrations); err != nil {
		t.Fatalf("migrate registry: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(conn, logger)
}

func countRows(t *testing.T, conn *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestImport_NewProjectInitializesMeta(t *testing.T) {
	svc := newService(t)
	workdir := t.TempDir()

	p, err := svc.Import(context.Background(), workdir)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if p.ID <= 0 {
		t.Errorf("id = %d, want > 0", p.ID)
	}
	if p.Name != filepath.Base(workdir) {
		t.Errorf("name = %q, want %q", p.Name, filepath.Base(workdir))
	}
	if p.Workdir != filepath.Clean(workdir) {
		t.Errorf("workdir = %q", p.Workdir)
	}

	// .taskboard/ 内容：meta.db + config.yaml（TF-033 v3 起不再创建 skills/ 目录）。
	metaDir := filepath.Join(workdir, ".taskboard")
	for _, f := range []string{"meta.db", "config.yaml"} {
		if _, err := os.Stat(filepath.Join(metaDir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}

	// meta.db 4 表齐全（skills 表已由 v3 迁移移除，TF-033）。
	projDB, err := db.Open(db.MetaDBPath(workdir))
	if err != nil {
		t.Fatalf("open meta.db: %v", err)
	}
	defer func() { _ = projDB.Close() }()
	for _, table := range []string{"tasks", "permissions", "import_drafts", "audit_log"} {
		if n := countRows(t, projDB, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='`+table+`'`); n != 1 {
			t.Errorf("meta.db missing table %s", table)
		}
	}
	// skills 表不应存在（v3 drop）。
	if n := countRows(t, projDB, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='skills'`); n != 0 {
		t.Error("skills table should be dropped by v3 migration (TF-033)")
	}

	// config.yaml：默认状态机（3 态）。
	pc, err := config.LoadProject(workdir)
	if err != nil {
		t.Fatalf("load project config: %v", err)
	}
	if len(pc.StateMachine.States) != 3 {
		t.Errorf("states = %d, want 3", len(pc.StateMachine.States))
	}
}

func TestImport_WritesDefaultPermissions(t *testing.T) {
	svc := newService(t)
	workdir := t.TempDir()
	if _, err := svc.Import(context.Background(), workdir); err != nil {
		t.Fatalf("import: %v", err)
	}
	projDB, err := db.Open(db.MetaDBPath(workdir))
	if err != nil {
		t.Fatalf("open meta.db: %v", err)
	}
	defer func() { _ = projDB.Close() }()

	// 动作全集 16 行。
	if n := countRows(t, projDB, `SELECT COUNT(*) FROM permissions`); n != len(AllActions) {
		t.Fatalf("permissions rows = %d, want %d", n, len(AllActions))
	}
	// 默认只读 5 项 true。
	if n := countRows(t, projDB, `SELECT COUNT(*) FROM permissions WHERE allowed = 1`); n != len(DefaultGrantedActions) {
		t.Errorf("granted = %d, want %d", n, len(DefaultGrantedActions))
	}
	// 关键 action 值抽查。
	for action, want := range DefaultGrantedActions {
		w := 0
		if want {
			w = 1
		}
		var got int
		if err := projDB.QueryRow(`SELECT allowed FROM permissions WHERE action = ?`, action).Scan(&got); err != nil {
			t.Fatalf("query %s: %v", action, err)
		}
		if got != w {
			t.Errorf("permission %s = %d, want %d", action, got, w)
		}
	}
	// 写权限默认关闭。
	var got int
	if err := projDB.QueryRow(`SELECT allowed FROM permissions WHERE action = 'task.create'`).Scan(&got); err != nil {
		t.Fatalf("query task.create: %v", err)
	}
	if got != 0 {
		t.Errorf("task.create should default to 0, got %d", got)
	}
}

func TestImport_ExistingMetaDirNotReinitialized(t *testing.T) {
	svc := newService(t)
	workdir := t.TempDir()

	// 手工创建 meta.db（无 config.yaml / skills/）。
	metaDir := filepath.Join(workdir, ".taskboard")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	projDB, err := db.EnsureProject(context.Background(), db.MetaDBPath(workdir))
	if err != nil {
		t.Fatalf("ensure project: %v", err)
	}
	_ = projDB.Close()

	if _, err := svc.Import(context.Background(), workdir); err != nil {
		t.Fatalf("import: %v", err)
	}
	// 不重新初始化：config.yaml 不应被创建（目录已有元数据）。
	if _, err := os.Stat(filepath.Join(metaDir, "config.yaml")); !os.IsNotExist(err) {
		t.Error("config.yaml should not be created when meta dir already exists")
	}
}

func TestImport_DuplicateIdempotent(t *testing.T) {
	svc := newService(t)
	workdir := t.TempDir()
	p1, err := svc.Import(context.Background(), workdir)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	p2, err := svc.Import(context.Background(), workdir)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if p1.ID != p2.ID {
		t.Errorf("idempotent import should return same record: %d vs %d", p1.ID, p2.ID)
	}
	if n := countRows(t, svc.registry, `SELECT COUNT(*) FROM projects`); n != 1 {
		t.Errorf("registry rows = %d, want 1", n)
	}
}

func TestImport_InvalidWorkdir(t *testing.T) {
	svc := newService(t)
	// 不存在目录。
	if _, err := svc.Import(context.Background(), filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("expected error for nonexistent dir")
	}
	// 相对路径。
	if _, err := svc.Import(context.Background(), "relative/path"); err == nil {
		t.Error("expected error for relative path")
	}
	// 文件而非目录。
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := svc.Import(context.Background(), file); err == nil {
		t.Error("expected error for non-directory")
	}
}

func TestList_OrderByLastOpened(t *testing.T) {
	svc := newService(t)
	d1 := t.TempDir()
	d2 := t.TempDir()
	d3 := t.TempDir()

	if _, err := svc.Import(context.Background(), d1); err != nil {
		t.Fatalf("import d1: %v", err)
	}
	p2, err := svc.Import(context.Background(), d2)
	if err != nil {
		t.Fatalf("import d2: %v", err)
	}
	if _, err := svc.Import(context.Background(), d3); err != nil {
		t.Fatalf("import d3: %v", err)
	}
	// TF-043：UI 导入默认隐藏，走完引导（CompleteOnboarding）后列表可见。
	for _, d := range []string{d1, d2, d3} {
		if _, err := svc.CompleteOnboarding(context.Background(), d); err != nil {
			t.Fatalf("complete %s: %v", d, err)
		}
	}

	// 打开 d2（Touch），d2 应排最前；d1/d3 从未打开按创建先后。
	if err := svc.Touch(context.Background(), p2.Workdir); err != nil {
		t.Fatalf("touch: %v", err)
	}

	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
	if list[0].Workdir != filepath.Clean(p2.Workdir) {
		t.Errorf("first should be touched project, got %s", list[0].Workdir)
	}
}

func TestRemove_KeepsDiskData(t *testing.T) {
	svc := newService(t)
	workdir := t.TempDir()
	p, err := svc.Import(context.Background(), workdir)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	if err := svc.Remove(context.Background(), p.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	// 注册记录消失。
	if n := countRows(t, svc.registry, `SELECT COUNT(*) FROM projects`); n != 0 {
		t.Errorf("registry rows = %d, want 0", n)
	}
	// 磁盘元数据完好。
	if _, err := os.Stat(filepath.Join(workdir, ".taskboard", "meta.db")); err != nil {
		t.Errorf("meta.db should survive removal: %v", err)
	}
	// 重新导入 → 数据完整（幂等语义下旧记录被删后重新注册）。
	if _, err := svc.Import(context.Background(), workdir); err != nil {
		t.Errorf("re-import after remove: %v", err)
	}
}

func TestRemove_NotFound(t *testing.T) {
	svc := newService(t)
	if err := svc.Remove(context.Background(), 9999); err == nil {
		t.Error("expected ErrNotFound for missing id")
	}
}

func TestRename_UpdatesNameKeepsWorkdir(t *testing.T) {
	svc := newService(t)
	workdir := t.TempDir()
	p, err := svc.Import(context.Background(), workdir)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	// TF-043：完成引导后列表可见。
	if _, err := svc.CompleteOnboarding(context.Background(), workdir); err != nil {
		t.Fatalf("complete: %v", err)
	}

	renamed, err := svc.Rename(context.Background(), p.ID, "  新项目名  ")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	// 名称去空白更新；workdir / id 不变。
	if renamed.Name != "新项目名" {
		t.Errorf("name = %q, want 新项目名", renamed.Name)
	}
	if renamed.Workdir != filepath.Clean(workdir) {
		t.Errorf("workdir changed: %q", renamed.Workdir)
	}
	if renamed.ID != p.ID {
		t.Errorf("id changed: %d -> %d", p.ID, renamed.ID)
	}
	// 列表反映新名称。
	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, item := range list {
		if item.ID == p.ID && item.Name == "新项目名" {
			found = true
		}
	}
	if !found {
		t.Errorf("list 未反映重命名: %+v", list)
	}
}

func TestRename_EmptyNameRejected(t *testing.T) {
	svc := newService(t)
	workdir := t.TempDir()
	p, err := svc.Import(context.Background(), workdir)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, err := svc.Rename(context.Background(), p.ID, "   "); err == nil {
		t.Error("空名应被拒绝")
	}
}

func TestRename_NotFound(t *testing.T) {
	svc := newService(t)
	if _, err := svc.Rename(context.Background(), 9999, "x"); err == nil {
		t.Error("expected ErrNotFound for missing id")
	}
}

func TestImport_VisibleToProjectExists(t *testing.T) {
	// 联调验证：project.Import 写入的注册记录可被 TF-003 中间件使用的 db.ProjectExists 命中。
	svc := newService(t)
	workdir := t.TempDir()
	if _, err := svc.Import(context.Background(), workdir); err != nil {
		t.Fatalf("import: %v", err)
	}
	ok, err := db.ProjectExists(context.Background(), svc.registry, filepath.Clean(workdir))
	if err != nil {
		t.Fatalf("project exists: %v", err)
	}
	if !ok {
		t.Error("imported project should be visible to db.ProjectExists")
	}
}

func TestTouch_UpdatesTimestamp(t *testing.T) {
	svc := newService(t)
	workdir := t.TempDir()
	if _, err := svc.Import(context.Background(), workdir); err != nil {
		t.Fatalf("import: %v", err)
	}
	before := time.Now().Add(-time.Minute).Format(time.RFC3339)
	if _, err := svc.registry.Exec(`UPDATE projects SET last_opened_at = ? WHERE workdir = ?`, before, filepath.Clean(workdir)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := svc.Touch(context.Background(), workdir); err != nil {
		t.Fatalf("touch: %v", err)
	}
	var last string
	if err := svc.registry.QueryRow(`SELECT last_opened_at FROM projects WHERE workdir = ?`, filepath.Clean(workdir)).Scan(&last); err != nil {
		t.Fatalf("query: %v", err)
	}
	if last == before {
		t.Error("last_opened_at should be updated")
	}
	// Touch 未注册目录不报错（静默）。
	if err := svc.Touch(context.Background(), filepath.Join(t.TempDir(), "ghost")); err != nil {
		t.Errorf("touch unregistered should not error: %v", err)
	}
}

// TestHidden_ImportVsAIEntry（TF-043）：UI 导入默认隐藏，AI 入口（ImportExisting）可见；
// List 只返回可见项目；CompleteOnboarding 幂等置可见；Check.Onboarded 跟随。
func TestHidden_ImportVsAIEntry(t *testing.T) {
	svc := newService(t)

	// UI 导入（Import）→ hidden=1：不在列表。
	uiDir := t.TempDir()
	p, err := svc.Import(context.Background(), uiDir)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !p.Hidden {
		t.Fatalf("UI 导入应默认隐藏, got %+v", p)
	}
	if list, _ := svc.List(context.Background()); len(list) != 0 {
		t.Fatalf("隐藏项目不应出现在列表: %+v", list)
	}
	// Check：registered=true, onboarded=false。
	ck, err := svc.Check(context.Background(), uiDir)
	if err != nil || !ck.Registered || ck.Onboarded {
		t.Fatalf("check: %+v err=%v", ck, err)
	}

	// AI 入口（ImportExisting）→ hidden=0：直接可见。
	aiDir := t.TempDir()
	if err := svc.Init(context.Background(), aiDir); err != nil {
		t.Fatalf("init: %v", err)
	}
	ap, err := svc.ImportExisting(context.Background(), aiDir)
	if err != nil {
		t.Fatalf("import existing: %v", err)
	}
	if ap.Hidden {
		t.Fatalf("AI 入口导入应直接可见, got %+v", ap)
	}
	if list, _ := svc.List(context.Background()); len(list) != 1 || list[0].Workdir != filepath.Clean(aiDir) {
		t.Fatalf("AI 项目应可见: %+v", list)
	}

	// CompleteOnboarding：UI 项目置可见（幂等，二次调用不报错）。
	done, err := svc.CompleteOnboarding(context.Background(), uiDir)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if done.Hidden {
		t.Fatalf("完成引导后应可见: %+v", done)
	}
	if list, _ := svc.List(context.Background()); len(list) != 2 {
		t.Fatalf("完成后两个项目都应可见: %+v", list)
	}
	if _, err := svc.CompleteOnboarding(context.Background(), uiDir); err != nil {
		t.Fatalf("complete 应幂等: %v", err)
	}
	ck2, _ := svc.Check(context.Background(), uiDir)
	if !ck2.Onboarded {
		t.Fatalf("check 应反映 onboarded: %+v", ck2)
	}

	// 未注册目录 complete → ErrNotFound。
	if _, err := svc.CompleteOnboarding(context.Background(), t.TempDir()); err == nil {
		t.Fatal("未注册目录 complete 应报错")
	}
}
