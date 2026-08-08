package task

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"tangoforge/internal/db"
)

// importEnv 构造带写钩子计数的测试环境（验证 ImportTasks 不触发 task.* 钩子）。
func importEnv(t *testing.T) (Service, string, *atomic.Int32) {
	t.Helper()
	workdir := t.TempDir()
	initMetaDB(t, workdir)
	var hookCalls atomic.Int32
	svc := NewService(Options{
		OnWrite: func(context.Context, string, string, string) { hookCalls.Add(1) },
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc, workdir, &hookCalls
}

// initMetaDB 初始化项目库（meta.db；状态机缺失回退默认四态）。
func initMetaDB(t *testing.T, workdir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workdir, ".taskboard"), 0o755); err != nil {
		t.Fatalf("mkdir .taskboard: %v", err)
	}
	conn, err := db.EnsureProject(context.Background(), db.MetaDBPath(workdir))
	if err != nil {
		t.Fatalf("init meta.db: %v", err)
	}
	_ = conn.Close()
}

// importTask 构造一个待导入任务（默认 todo）。
func importTask(id, title string) Task {
	return Task{ID: id, Title: title, Status: "todo"}
}

// insertRaw 经 repo 直插任务（可设置内部字段，如 source_file）。
func insertRaw(t *testing.T, svc Service, workdir string, tk Task) {
	t.Helper()
	s := svc.(*service)
	conn, err := s.projectDB(context.Background(), workdir)
	if err != nil {
		t.Fatalf("projectDB: %v", err)
	}
	if err := newSQLRepo(conn).Create(context.Background(), &tk); err != nil {
		t.Fatalf("repo create: %v", err)
	}
}

func TestImportTasks_ArchiveAndInsert(t *testing.T) {
	svc, wd, _ := importEnv(t)
	ctx := context.Background()

	// 旧任务（同 source_file，经 repo 直插标记）。
	insertRaw(t, svc, wd, Task{
		ID: "old-1", ProjectID: 1, Title: "旧任务", Status: "todo",
		SourceFile: "/doc.md", SourceSection: "1",
	})

	res, err := svc.ImportTasks(ctx, wd, "/doc.md", []Task{
		importTask("11111111-1111-1111-1111-111111111111", "新任务A"),
		importTask("22222222-2222-2222-2222-222222222222", "新任务B"),
	})
	if err != nil {
		t.Fatalf("ImportTasks: %v", err)
	}
	if res.Created != 2 || res.Archived != 1 {
		t.Fatalf("Created=%d Archived=%d, 期望 2/1", res.Created, res.Archived)
	}

	// 新任务可查（含 source_file 保留）。
	a, err := svc.Get(ctx, wd, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("Get A: %v", err)
	}
	if a.SourceFile != "/doc.md" {
		t.Fatalf("source_file: %+v", a)
	}
	// 旧任务已归档（archived_from=todo）。
	oldNow, err := svc.Get(ctx, wd, "old-1")
	if err != nil {
		t.Fatalf("Get old: %v", err)
	}
	if oldNow.Status != "archived" || oldNow.ArchivedFrom != "todo" {
		t.Fatalf("旧任务应归档: %+v", oldNow)
	}
}

func TestImportTasks_Validation(t *testing.T) {
	svc, wd, _ := importEnv(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		tasks []Task
		code  string
	}{
		{"空标题", []Task{importTask("id-1", "  ")}, CodeTaskInvalid},
		{"状态不存在", []Task{{ID: "id-1", Title: "x", Status: "no-such"}}, CodeStatusNotFound},
		{"archived 不可作为导入状态", []Task{{ID: "id-1", Title: "x", Status: "archived"}}, CodeStatusNotFound},
		{"依赖不存在", []Task{importTask("id-1", "A"), {ID: "id-2", Title: "B", Status: "todo", DependsOn: []string{"no-exist"}}}, CodeDependencyNotFound},
		{"本批自环", []Task{{ID: "id-1", Title: "A", Status: "todo", DependsOn: []string{"id-1"}}}, CodeCircularDependency},
		{"本批两任务环", []Task{
			{ID: "id-1", Title: "A", Status: "todo", DependsOn: []string{"id-2"}},
			{ID: "id-2", Title: "B", Status: "todo", DependsOn: []string{"id-1"}},
		}, CodeCircularDependency},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.ImportTasks(ctx, wd, "/doc.md", tc.tasks)
			var te *Error
			if !errors.As(err, &te) || te.Code != tc.code {
				t.Fatalf("期望 code=%s, got %v", tc.code, err)
			}
		})
	}
}

func TestImportTasks_NoWriteHook(t *testing.T) {
	svc, wd, hook := importEnv(t)
	ctx := context.Background()

	if _, err := svc.ImportTasks(ctx, wd, "/doc.md", []Task{importTask("id-1", "批量导入")}); err != nil {
		t.Fatalf("ImportTasks: %v", err)
	}
	// 导入不触发 task.* 钩子（QA P4-1：事件由 parser 层发 import.draft_confirmed）。
	if n := hook.Load(); n != 0 {
		t.Fatalf("ImportTasks 不应触发写钩子, got %d", n)
	}
}

func TestImportTasks_DependsOnExisting(t *testing.T) {
	svc, wd, _ := importEnv(t)
	ctx := context.Background()
	existing := mustCreate(t, svc, wd, CreateInput{Title: "库内任务"})

	res, err := svc.ImportTasks(ctx, wd, "/doc.md", []Task{
		{ID: "id-1", Title: "新任务", Status: "todo", DependsOn: []string{existing.ID}},
	})
	if err != nil {
		t.Fatalf("ImportTasks: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("Created=%d", res.Created)
	}
	got, err := svc.Get(ctx, wd, "id-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DependsOn) != 1 || got.DependsOn[0] != existing.ID {
		t.Fatalf("依赖未保留: %+v", got.DependsOn)
	}
}

func TestImportTasks_SameFileReimport(t *testing.T) {
	svc, wd, _ := importEnv(t)
	ctx := context.Background()

	r1, err := svc.ImportTasks(ctx, wd, "/doc.md", []Task{importTask("id-1", "V1")})
	if err != nil || r1.Created != 1 {
		t.Fatalf("r1: %+v err=%v", r1, err)
	}
	// 同一文件再次导入 → 旧任务归档 + 新任务重建。
	r2, err := svc.ImportTasks(ctx, wd, "/doc.md", []Task{importTask("id-2", "V2")})
	if err != nil {
		t.Fatalf("r2: %v", err)
	}
	if r2.Created != 1 || r2.Archived != 1 {
		t.Fatalf("r2 Created=%d Archived=%d, 期望 1/1", r2.Created, r2.Archived)
	}
	old, err := svc.Get(ctx, wd, "id-1")
	if err != nil {
		t.Fatal(err)
	}
	if old.Status != StatusArchived {
		t.Fatalf("V1 应归档: %+v", old)
	}
	if _, err := svc.Get(ctx, wd, "id-2"); err != nil {
		t.Fatalf("V2 应存在: %v", err)
	}
}

func TestImportTasks_EmptyList(t *testing.T) {
	svc, wd, _ := importEnv(t)
	_, err := svc.ImportTasks(context.Background(), wd, "/doc.md", nil)
	if !strings.Contains(err.Error(), "为空") {
		t.Fatalf("空列表应报错: %v", err)
	}
}

// TF-040：Create 自动分配编号（T01/T02 递增）；指定编号沿用；重复编号拒绝。
func TestCreate_TaskNumber(t *testing.T) {
	svc, wd, _ := importEnv(t)
	ctx := context.Background()

	a, err := svc.Create(ctx, wd, CreateInput{Title: "A"})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	if a.Number != "T001" {
		t.Fatalf("第一个任务编号 = %q, want T001", a.Number)
	}
	b, err := svc.Create(ctx, wd, CreateInput{Title: "B"})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	if b.Number != "T002" {
		t.Fatalf("第二个任务编号 = %q, want T002", b.Number)
	}
	// 指定编号沿用。
	c, err := svc.Create(ctx, wd, CreateInput{Title: "C", Number: "P0"})
	if err != nil {
		t.Fatalf("create C: %v", err)
	}
	if c.Number != "P0" {
		t.Fatalf("指定编号 = %q, want P0", c.Number)
	}
	// 重复指定 → 拒绝。
	if _, err := svc.Create(ctx, wd, CreateInput{Title: "D", Number: "P0"}); err == nil {
		t.Fatalf("重复编号应被拒绝")
	}
	// 自动分配从库内最大序号推进（P0 不参与 T 序号，继续 T003）。
	d, err := svc.Create(ctx, wd, CreateInput{Title: "D"})
	if err != nil {
		t.Fatalf("create D: %v", err)
	}
	if d.Number != "T003" {
		t.Fatalf("编号 = %q, want T003", d.Number)
	}
}

// TF-040：ImportTasks 沿用文档编号；空/冲突 → 自动分配。
func TestImportTasks_TaskNumber(t *testing.T) {
	svc, wd, _ := importEnv(t)
	ctx := context.Background()

	// 批内：沿用 P0/P1；空号自动分配 T001/T002。
	_, err := svc.ImportTasks(ctx, wd, "/doc1.md", []Task{
		{ID: "u1", Title: "A", Status: "todo", Number: "P0"},
		{ID: "u2", Title: "B", Status: "todo", Number: ""},
		{ID: "u3", Title: "C", Status: "todo", Number: "P1"},
		{ID: "u4", Title: "D", Status: "todo", Number: ""},
	})
	if err != nil {
		t.Fatalf("import1: %v", err)
	}
	a, _ := svc.Get(ctx, wd, "u1")
	b, _ := svc.Get(ctx, wd, "u2")
	d, _ := svc.Get(ctx, wd, "u4")
	if a.Number != "P0" || d.Number != "T002" {
		t.Fatalf("沿用/分配: A=%q D=%q (want P0/T002)", a.Number, d.Number)
	}
	if b.Number != "T001" {
		t.Fatalf("B 编号 = %q, want T001", b.Number)
	}
	// 第二次导入：文档编号与库内冲突 → 自动分配；新空号从库内最大推进。
	_, err = svc.ImportTasks(ctx, wd, "/doc2.md", []Task{
		{ID: "v1", Title: "E", Status: "todo", Number: "P0"}, // 与库内 P0 冲突
		{ID: "v2", Title: "F", Status: "todo", Number: "P9"},
	})
	if err != nil {
		t.Fatalf("import2: %v", err)
	}
	e, _ := svc.Get(ctx, wd, "v1")
	f, _ := svc.Get(ctx, wd, "v2")
	if e.Number == "P0" {
		t.Fatalf("冲突编号应自动重编, got %q", e.Number)
	}
	if f.Number != "P9" {
		t.Fatalf("F 编号 = %q, want P9", f.Number)
	}
}
