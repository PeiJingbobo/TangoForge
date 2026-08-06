package task

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"tangoforge/internal/config"
	"tangoforge/internal/db"
)

// ---- 归档（Archive） ----

func TestArchive_Ok(t *testing.T) {
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "归档我", Status: strPtr("doing")})

	res, err := svc.Archive(context.Background(), wd, task.ID)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if res.Task.Status != StatusArchived {
		t.Errorf("归档后 status 应为 archived，got %q", res.Task.Status)
	}
	if res.Task.ArchivedFrom != "doing" {
		t.Errorf("archived_from 应记录归档前状态 doing，got %q", res.Task.ArchivedFrom)
	}
	if !res.Task.UpdatedAt.After(task.UpdatedAt) {
		t.Errorf("updated_at 应刷新")
	}
	if res.DependentCount != 0 || res.ChildrenCleared != 0 {
		t.Errorf("无依赖无子任务时计数应为 0，got dep=%d child=%d", res.DependentCount, res.ChildrenCleared)
	}
}

func TestArchive_AlreadyArchivedIdempotent(t *testing.T) {
	// Q2-B：已归档再次归档 → 幂等返回当前状态，不重复记录、不触发钩子。
	var mu sync.Mutex
	actions := []string{}
	svc := NewService(Options{
		OnWrite: func(_ context.Context, a, _ string) { mu.Lock(); actions = append(actions, a); mu.Unlock() },
	})
	wd := t.TempDir()
	initEnv(t, wd)
	t.Cleanup(func() { _ = svc.Close() })

	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	if _, err := svc.Archive(context.Background(), wd, task.ID); err != nil {
		t.Fatal(err)
	}
	// 第二次归档。
	res, err := svc.Archive(context.Background(), wd, task.ID)
	if err != nil {
		t.Fatalf("幂等归档应成功，got %v", err)
	}
	if res.Task.Status != StatusArchived || res.Task.ArchivedFrom != "todo" {
		t.Errorf("幂等返回当前状态：%+v", res.Task)
	}
	mu.Lock()
	defer mu.Unlock()
	archivedCount := 0
	for _, a := range actions {
		if a == "task.archived" {
			archivedCount++
		}
	}
	if archivedCount != 1 {
		t.Errorf("幂等归档不应重复触发 task.archived，got %v", actions)
	}
}

func TestArchive_NotFound(t *testing.T) {
	svc, wd := newTestEnv(t)
	_, err := svc.Archive(context.Background(), wd, "ghost")
	if !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("应返回 TASK_NOT_FOUND，got %v", err)
	}
}

func TestArchive_CascadeClearsChildren(t *testing.T) {
	// Q3：归档父任务 → 直接子任务级联置空为顶层（ChildrenCleared=1），孙任务不受影响。
	svc, wd := newTestEnv(t)
	parent := mustCreate(t, svc, wd, CreateInput{Title: "父"})
	child := mustCreate(t, svc, wd, CreateInput{Title: "子", ParentID: strPtr(parent.ID)})
	grand := mustCreate(t, svc, wd, CreateInput{Title: "孙", ParentID: strPtr(child.ID)})

	res, err := svc.Archive(context.Background(), wd, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.ChildrenCleared != 1 {
		t.Errorf("ChildrenCleared 应为 1，got %d", res.ChildrenCleared)
	}
	// 子任务成为顶层。
	gotChild, _ := svc.Get(context.Background(), wd, child.ID)
	if gotChild.ParentID != nil {
		t.Errorf("子任务应级联置空为顶层，got parent=%v", *gotChild.ParentID)
	}
	// 孙任务 parent 仍是子任务（不递归）。
	gotGrand, _ := svc.Get(context.Background(), wd, grand.ID)
	if gotGrand.ParentID == nil || *gotGrand.ParentID != child.ID {
		t.Errorf("孙任务 parent 应保持为子任务，got %v", gotGrand.ParentID)
	}
}

func TestArchive_DependentCount(t *testing.T) {
	// Q1-A：未归档任务中 depends_on 包含目标的个数（不阻断归档）。
	svc, wd := newTestEnv(t)
	a := mustCreate(t, svc, wd, CreateInput{Title: "A"})
	mustCreate(t, svc, wd, CreateInput{Title: "B", DependsOn: []string{a.ID}})
	mustCreate(t, svc, wd, CreateInput{Title: "C", DependsOn: []string{a.ID}})

	res, err := svc.Archive(context.Background(), wd, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.DependentCount != 2 {
		t.Errorf("DependentCount 应为 2，got %d", res.DependentCount)
	}
	// 已归档的依赖者不再计入：E 被 F（未归档）与 G（归档）依赖。
	b := mustCreate(t, svc, wd, CreateInput{Title: "E"})
	mustCreate(t, svc, wd, CreateInput{Title: "F", DependsOn: []string{b.ID}})
	g := mustCreate(t, svc, wd, CreateInput{Title: "G", DependsOn: []string{b.ID}})
	_, _ = svc.Archive(context.Background(), wd, g.ID) // G 归档，其依赖不计
	resB, _ := svc.Archive(context.Background(), wd, b.ID)
	if resB.DependentCount != 1 {
		t.Errorf("归档依赖者不计入统计，DependentCount 应为 1，got %d", resB.DependentCount)
	}
}

// ---- 还原（Restore） ----

func TestRestore_Ok(t *testing.T) {
	// Q7：还原恢复到 archived_from 并清空该字段。
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t", Status: strPtr("doing")})
	if _, err := svc.Archive(context.Background(), wd, task.ID); err != nil {
		t.Fatal(err)
	}

	restored, err := svc.Restore(context.Background(), wd, task.ID, RestoreOptions{})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.Status != "doing" {
		t.Errorf("还原应回到 archived_from 状态 doing，got %q", restored.Status)
	}
	if restored.ArchivedFrom != "" {
		t.Errorf("还原后 archived_from 应清空，got %q", restored.ArchivedFrom)
	}
	// 往返：还原后可再次归档。
	res, _ := svc.Archive(context.Background(), wd, task.ID)
	if res.Task.ArchivedFrom != "doing" {
		t.Errorf("往返后 archived_from 应重新记录 doing，got %q", res.Task.ArchivedFrom)
	}
}

func TestRestore_OnlyArchived(t *testing.T) {
	// Q6：非归档任务还原 → TASK_INVALID。
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	_, err := svc.Restore(context.Background(), wd, task.ID, RestoreOptions{})
	if !errors.Is(err, ErrTaskInvalid) {
		t.Errorf("非归档还原应 TASK_INVALID，got %v", err)
	}
}

func TestRestore_MissingArchivedFrom(t *testing.T) {
	// Q6：archived_from 缺失（异常数据）→ 回退 todo。
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	if _, err := svc.Archive(context.Background(), wd, task.ID); err != nil {
		t.Fatal(err)
	}
	// repo 直写清空 archived_from（模拟异常数据）。
	conn, err := openTestConn(wd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	arch, _ := newSQLRepo(conn).GetByID(context.Background(), task.ID)
	arch.ArchivedFrom = ""
	arch.UpdatedAt = arch.CreatedAt
	if err := newSQLRepo(conn).Update(context.Background(), arch); err != nil {
		t.Fatal(err)
	}

	restored, err := svc.Restore(context.Background(), wd, task.ID, RestoreOptions{})
	if err != nil {
		t.Fatalf("archived_from 缺失应回退 todo：%v", err)
	}
	if restored.Status != StatusTodo {
		t.Errorf("应回退 todo，got %q", restored.Status)
	}
}

func TestRestore_StatusDeleted(t *testing.T) {
	// Q5：archived_from 目标状态已从状态机删除 → 默认 STATUS_NOT_FOUND；FallbackTodo 回退 todo。
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t"}) // archived_from 将是 todo
	if _, err := svc.Archive(context.Background(), wd, task.ID); err != nil {
		t.Fatal(err)
	}
	// 删除 todo 状态（任务已归档，无占用）。
	sm := config.StateMachine{
		States:      []config.State{{Key: "doing"}, {Key: "done"}},
		Transitions: []config.Transition{{From: "doing", To: []string{"done"}}},
	}
	if _, err := svc.UpdateStateMachine(context.Background(), wd, sm); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Restore(context.Background(), wd, task.ID, RestoreOptions{})
	if !errors.Is(err, ErrStatusNotFound) {
		t.Errorf("目标状态已删除默认应 STATUS_NOT_FOUND，got %v", err)
	}
	restored, err := svc.Restore(context.Background(), wd, task.ID, RestoreOptions{FallbackTodo: true})
	if err != nil {
		t.Fatalf("FallbackTodo 应回退 todo：%v", err)
	}
	if restored.Status != StatusTodo {
		t.Errorf("FallbackTodo 后 status 应为 todo，got %q", restored.Status)
	}
}

func TestRestore_NotFound(t *testing.T) {
	svc, wd := newTestEnv(t)
	_, err := svc.Restore(context.Background(), wd, "ghost", RestoreOptions{})
	if !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("应返回 TASK_NOT_FOUND，got %v", err)
	}
}

// ---- 物理删除（Delete） ----

func TestDelete_Ok(t *testing.T) {
	// Q9：回收站任务物理删除成功，返回被删快照，之后不可查询。
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "删我"})
	if _, err := svc.Archive(context.Background(), wd, task.ID); err != nil {
		t.Fatal(err)
	}

	snapshot, err := svc.Delete(context.Background(), wd, task.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if snapshot.ID != task.ID || snapshot.Status != StatusArchived {
		t.Errorf("快照不符：%+v", snapshot)
	}
	if _, err := svc.Get(context.Background(), wd, task.ID); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("物理删除后应不可查询，got %v", err)
	}
}

func TestDelete_NotArchived(t *testing.T) {
	// Q9：非归档任务物理删除 → DELETE_NOT_ALLOWED。
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	_, err := svc.Delete(context.Background(), wd, task.ID)
	if !errors.Is(err, ErrDeleteNotAllowed) {
		t.Errorf("应返回 DELETE_NOT_ALLOWED，got %v", err)
	}
	// 任务仍存在。
	if _, err := svc.Get(context.Background(), wd, task.ID); err != nil {
		t.Errorf("拒绝删除后任务应保留：%v", err)
	}
}

func TestDelete_CascadeClearsChildren(t *testing.T) {
	// Q8-A：物理删除父任务时子任务不可一并删除，仅级联置空保留为顶层。
	svc, wd := newTestEnv(t)
	parent := mustCreate(t, svc, wd, CreateInput{Title: "父"})
	child := mustCreate(t, svc, wd, CreateInput{Title: "子", ParentID: strPtr(parent.ID)})
	if _, err := svc.Archive(context.Background(), wd, parent.ID); err != nil {
		t.Fatal(err)
	}
	// 归档时子已置空；重新挂载子到已归档父（TF-005 允许挂载 archived 父），验证删除时的级联。
	if _, err := svc.Update(context.Background(), wd, child.ID, UpdateInput{ParentID: strPtrPtr(strPtr(parent.ID))}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Delete(context.Background(), wd, parent.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// 子任务保留为顶层（不可一并删除）。
	gotChild, err := svc.Get(context.Background(), wd, child.ID)
	if err != nil {
		t.Fatalf("子任务应保留：%v", err)
	}
	if gotChild.ParentID != nil {
		t.Errorf("子任务应置空为顶层，got parent=%v", *gotChild.ParentID)
	}
}

func TestDelete_NotFound(t *testing.T) {
	svc, wd := newTestEnv(t)
	_, err := svc.Delete(context.Background(), wd, "ghost")
	if !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("应返回 TASK_NOT_FOUND，got %v", err)
	}
}

// ---- 钩子动作 ----

func TestWriteHook_ArchiveRestoreDelete(t *testing.T) {
	var mu sync.Mutex
	actions := []string{}
	svc := NewService(Options{
		OnWrite: func(_ context.Context, a, _ string) { mu.Lock(); actions = append(actions, a); mu.Unlock() },
	})
	wd := t.TempDir()
	initEnv(t, wd)
	t.Cleanup(func() { _ = svc.Close() })

	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	_, _ = svc.Archive(context.Background(), wd, task.ID)
	_, _ = svc.Restore(context.Background(), wd, task.ID, RestoreOptions{})
	_, _ = svc.Archive(context.Background(), wd, task.ID)
	_, _ = svc.Delete(context.Background(), wd, task.ID)

	mu.Lock()
	defer mu.Unlock()
	want := []string{"task.created", "task.archived", "task.restored", "task.archived", "task.deleted"}
	if len(actions) != len(want) {
		t.Fatalf("钩子序列：got %v want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Errorf("钩子序列不匹配：got %v want %v", actions, want)
			break
		}
	}
}

// ---- 辅助 ----

// initEnv 初始化临时目录的项目库（供自定义 Options 的 service 测试用）。
func initEnv(t *testing.T, workdir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workdir, ".taskboard"), 0o755); err != nil {
		t.Fatalf("mkdir .taskboard: %v", err)
	}
	conn, err := db.EnsureProject(context.Background(), db.MetaDBPath(workdir))
	if err != nil {
		t.Fatalf("init project db: %v", err)
	}
	_ = conn.Close()
}
