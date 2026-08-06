package task

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"tangoforge/internal/db"
)

// ---- 测试辅助 ----

// newTestEnv 创建临时工作目录并初始化项目库（meta.db），返回 service 与 workdir。
// 符合 TECHNICAL.md §3.8：sqlite 隔离，仅依赖 t.TempDir()（自动清理）。
func newTestEnv(t *testing.T) (Service, string) {
	t.Helper()
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".taskboard"), 0o755); err != nil {
		t.Fatalf("mkdir .taskboard: %v", err)
	}
	conn, err := db.EnsureProject(context.Background(), db.MetaDBPath(workdir))
	if err != nil {
		t.Fatalf("init project db: %v", err)
	}
	_ = conn.Close()
	svc := NewService(Options{})
	t.Cleanup(func() { _ = svc.Close() })
	return svc, workdir
}

// mustCreate 创建任务并断言成功。
func mustCreate(t *testing.T, svc Service, workdir string, in CreateInput) Task {
	t.Helper()
	task, err := svc.Create(context.Background(), workdir, in)
	if err != nil {
		t.Fatalf("Create(%+v): %v", in, err)
	}
	return task
}

// strPtr / strPtrPtr 构造指针（测试 UpdateInput 指针语义用）。
func strPtr(s string) *string      { return &s }
func strPtrPtr(s *string) **string { return &s }

// ---- Create ----

func TestCreate_Basic(t *testing.T) {
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "写单元测试"})

	if task.ID == "" || len(task.ID) != 36 {
		t.Errorf("ID 应为 UUID v4 字符串，got %q", task.ID)
	}
	if task.ProjectID != 1 {
		t.Errorf("ProjectID 应固定为 1（项目库内冗余），got %d", task.ProjectID)
	}
	if task.ParentID != nil {
		t.Errorf("ParentID 应为 nil，got %v", *task.ParentID)
	}
	if task.Title != "写单元测试" {
		t.Errorf("Title = %q", task.Title)
	}
	if task.Status != StatusTodo {
		t.Errorf("默认状态应为 todo，got %q", task.Status)
	}
	if task.Priority != 0 {
		t.Errorf("默认优先级应为 0，got %d", task.Priority)
	}
	if task.CreatedAt.IsZero() || task.UpdatedAt.IsZero() {
		t.Error("时间戳不应为零值")
	}
}

func TestCreate_WithFields(t *testing.T) {
	svc, wd := newTestEnv(t)
	parent := mustCreate(t, svc, wd, CreateInput{Title: "父任务"})

	task := mustCreate(t, svc, wd, CreateInput{
		ParentID:    strPtr(parent.ID),
		Title:       "子任务",
		Description: "详情",
		Status:      strPtr("doing"),
		Priority:    "high",
		Tags:        []string{"bug", "v2", "bug", "", " ui "},
		Assignee:    "张三",
		DependsOn:   []string{parent.ID, ""},
	})
	if task.ParentID == nil || *task.ParentID != parent.ID {
		t.Errorf("ParentID = %v，期望 %s", task.ParentID, parent.ID)
	}
	if task.Status != "doing" {
		t.Errorf("Status = %q", task.Status)
	}
	if task.Priority != 4 {
		t.Errorf("high 应归一为 4，got %d", task.Priority)
	}
	if len(task.Tags) != 3 || task.Tags[0] != "bug" || task.Tags[1] != "v2" || task.Tags[2] != "ui" {
		t.Errorf("tags 应去重/去空/保序/去空白，got %v", task.Tags)
	}
	if task.Assignee != "张三" {
		t.Errorf("Assignee = %q", task.Assignee)
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0] != parent.ID {
		t.Errorf("depends_on 应去空串保序，got %v", task.DependsOn)
	}
}

func TestCreate_TitleRequired(t *testing.T) {
	svc, wd := newTestEnv(t)
	for _, title := range []string{"", "   ", "\t\n"} {
		_, err := svc.Create(context.Background(), wd, CreateInput{Title: title})
		if !errors.Is(err, ErrTaskInvalid) {
			t.Errorf("空 title %q 应返回 TASK_INVALID，got %v", title, err)
		}
	}
}

func TestCreate_PriorityNormalize(t *testing.T) {
	svc, wd := newTestEnv(t)
	cases := []struct {
		in   any
		want int
	}{
		{nil, 0}, {0, 0}, {5, 5}, {3, 3}, {int64(2), 2}, {4.0, 4},
		{"lowest", 0}, {"none", 0}, {"low", 1}, {"normal", 3}, {"default", 3},
		{"high", 4}, {"highest", 5}, {"critical", 5}, {"urgent", 5}, {"3", 3},
	}
	for _, c := range cases {
		task := mustCreate(t, svc, wd, CreateInput{Title: "t", Priority: c.in})
		if task.Priority != c.want {
			t.Errorf("NormalizePriority(%v) = %d，期望 %d", c.in, task.Priority, c.want)
		}
	}
}

func TestCreate_PriorityInvalid(t *testing.T) {
	svc, wd := newTestEnv(t)
	for _, in := range []any{6, -1, 2.5, "超高", "7"} {
		_, err := svc.Create(context.Background(), wd, CreateInput{Title: "t", Priority: in})
		if !errors.Is(err, ErrTaskInvalid) {
			t.Errorf("非法 priority %v 应返回 TASK_INVALID，got %v", in, err)
		}
	}
}

func TestCreate_StatusValidation(t *testing.T) {
	svc, wd := newTestEnv(t)
	// 显式合法状态（默认状态机含 doing）。
	mustCreate(t, svc, wd, CreateInput{Title: "ok", Status: strPtr("doing")})
	// 不存在的状态。
	if _, err := svc.Create(context.Background(), wd, CreateInput{Title: "x", Status: strPtr("nonexistent")}); !errors.Is(err, ErrStatusNotFound) {
		t.Errorf("未知状态应返回 STATUS_NOT_FOUND，got %v", err)
	}
	// archived 为系统保留态，不可创建。
	if _, err := svc.Create(context.Background(), wd, CreateInput{Title: "x", Status: strPtr(StatusArchived)}); !errors.Is(err, ErrStatusNotFound) {
		t.Errorf("archived 保留态应被拒绝（STATUS_NOT_FOUND），got %v", err)
	}
}

func TestCreate_ParentNotFound(t *testing.T) {
	svc, wd := newTestEnv(t)
	_, err := svc.Create(context.Background(), wd, CreateInput{Title: "t", ParentID: strPtr("no-such-id")})
	if !errors.Is(err, ErrParentNotFound) {
		t.Errorf("父任务不存在应返回 PARENT_NOT_FOUND，got %v", err)
	}
}

// ---- Get ----

func TestGet_Ok(t *testing.T) {
	svc, wd := newTestEnv(t)
	created := mustCreate(t, svc, wd, CreateInput{Title: "详情测试", Description: "desc"})
	got, err := svc.Get(context.Background(), wd, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID || got.Title != "详情测试" || got.Description != "desc" {
		t.Errorf("Get 返回不一致：%+v", got)
	}
}

func TestGet_NotFound(t *testing.T) {
	svc, wd := newTestEnv(t)
	_, err := svc.Get(context.Background(), wd, "missing-id")
	if !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("应返回 TASK_NOT_FOUND，got %v", err)
	}
}

// ---- List：树形 ----

func TestList_Tree(t *testing.T) {
	svc, wd := newTestEnv(t)
	// priority 制造确定顺序：B(5) > A(1)；children 集合断言（同秒 created_at 下按 id 排序，顺序不强制）。
	a := mustCreate(t, svc, wd, CreateInput{Title: "A", Priority: 1})
	a1 := mustCreate(t, svc, wd, CreateInput{Title: "A1", ParentID: strPtr(a.ID), Priority: 1})
	a2 := mustCreate(t, svc, wd, CreateInput{Title: "A2", ParentID: strPtr(a.ID), Priority: 1})
	b := mustCreate(t, svc, wd, CreateInput{Title: "B", Priority: 5})

	res, err := svc.List(context.Background(), wd, ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res.Tree) != 2 {
		t.Fatalf("顶层应为 [B, A]（priority DESC），got %d 个：%v", len(res.Tree), names(res.Tree))
	}
	if res.Tree[0].Task.ID != b.ID || res.Tree[1].Task.ID != a.ID {
		t.Errorf("顶层顺序错误（priority DESC）：%v", names(res.Tree))
	}
	// A 的 children 应为 {A1, A2}（集合断言，同秒同优先级时按 id ASC，顺序不强制）。
	children := res.Tree[1].Children
	if len(children) != 2 {
		t.Fatalf("A.children 应为 2 个，got %d", len(children))
	}
	childIDs := map[string]bool{children[0].Task.ID: true, children[1].Task.ID: true}
	if !childIDs[a1.ID] || !childIDs[a2.ID] {
		t.Errorf("A.children 集合错误：%v", names(children))
	}
}

func TestList_Sort(t *testing.T) {
	svc, wd := newTestEnv(t)
	low := mustCreate(t, svc, wd, CreateInput{Title: "低", Priority: 1})
	high := mustCreate(t, svc, wd, CreateInput{Title: "高", Priority: 5})
	mid := mustCreate(t, svc, wd, CreateInput{Title: "中", Priority: 3})

	res, _ := svc.List(context.Background(), wd, ListFilter{})
	// priority DESC：高(5) > 中(3) > 低(1)。
	got := []string{res.Tree[0].Task.ID, res.Tree[1].Task.ID, res.Tree[2].Task.ID}
	want := []string{high.ID, mid.ID, low.ID}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("排序错误（priority DESC）：got %v want %v", got, want)
		}
	}
}

func TestList_ExcludesArchivedByDefault(t *testing.T) {
	svc, wd := newTestEnv(t)
	mustCreate(t, svc, wd, CreateInput{Title: "正常"})
	// 直接构造一条 archived 任务（走 repo 直写，模拟归档后状态）。
	conn, err := db.EnsureProject(context.Background(), db.MetaDBPath(wd))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	arch := mustCreate(t, svc, wd, CreateInput{Title: "已归档"})
	if err := newSQLRepo(conn).Update(context.Background(), &Task{
		ID: arch.ID, ProjectID: 1, Title: "已归档", Status: StatusArchived,
		ArchivedFrom: StatusTodo, CreatedAt: arch.CreatedAt, UpdatedAt: arch.UpdatedAt,
	}); err != nil {
		t.Fatal(err)
	}

	res, _ := svc.List(context.Background(), wd, ListFilter{})
	if len(res.Tree) != 1 {
		t.Fatalf("默认应排除 archived，got %d 个", len(res.Tree))
	}
	// 显式 filter[status]=archived 只返回归档任务。
	resArch, _ := svc.List(context.Background(), wd, ListFilter{Status: StatusArchived})
	if len(resArch.Tree) != 1 || resArch.Tree[0].Task.ID != arch.ID {
		t.Errorf("filter[status]=archived 应只返回归档任务，got %v", names(resArch.Tree))
	}
}

func TestList_FilterStatusAndQ(t *testing.T) {
	svc, wd := newTestEnv(t)
	mustCreate(t, svc, wd, CreateInput{Title: "待办甲", Description: "修复登录页"})
	mustCreate(t, svc, wd, CreateInput{Title: "待办乙", Status: strPtr("doing")})

	// 状态过滤。
	res, _ := svc.List(context.Background(), wd, ListFilter{Status: "doing"})
	if len(res.Tree) != 1 || res.Tree[0].Task.Title != "待办乙" {
		t.Errorf("status=doing 过滤错误：%v", names(res.Tree))
	}
	// q 搜索 description。
	resQ, _ := svc.List(context.Background(), wd, ListFilter{Q: "登录"})
	if len(resQ.Tree) != 1 || resQ.Tree[0].Task.Title != "待办甲" {
		t.Errorf("q 搜索错误：%v", names(resQ.Tree))
	}
	// q 搜索无匹配。
	resNone, _ := svc.List(context.Background(), wd, ListFilter{Q: "不存在的关键词"})
	if len(resNone.Tree) != 0 {
		t.Errorf("q 无匹配应返回空树，got %v", names(resNone.Tree))
	}
}

func TestList_AncestorRetention(t *testing.T) {
	svc, wd := newTestEnv(t)
	parent := mustCreate(t, svc, wd, CreateInput{Title: "容器父任务"})
	mustCreate(t, svc, wd, CreateInput{Title: "匹配子任务A", ParentID: strPtr(parent.ID)})
	mustCreate(t, svc, wd, CreateInput{Title: "无关子任务B", ParentID: strPtr(parent.ID)})

	// q 匹配 "匹配子"：父不匹配但作为容器保留，children 仅含匹配子。
	res, _ := svc.List(context.Background(), wd, ListFilter{Q: "匹配子"})
	if len(res.Tree) != 1 {
		t.Fatalf("祖先保留：应只有父容器在顶层，got %v", names(res.Tree))
	}
	if res.Tree[0].Task.ID != parent.ID {
		t.Errorf("容器应为父任务，got %v", res.Tree[0].Task.Title)
	}
	if len(res.Tree[0].Children) != 1 || res.Tree[0].Children[0].Task.Title != "匹配子任务A" {
		t.Errorf("children 应仅含匹配者，got %v", names(res.Tree[0].Children))
	}
}

// ---- List：分页 ----

func TestList_Pagination(t *testing.T) {
	svc, wd := newTestEnv(t)
	for i := 0; i < 7; i++ {
		mustCreate(t, svc, wd, CreateInput{Title: "任务" + string(rune('A'+i))})
	}

	res, _ := svc.List(context.Background(), wd, ListFilter{Page: 1, Size: 3})
	if len(res.Items) != 3 || res.Total != 7 || res.Page != 1 || res.Size != 3 {
		t.Errorf("第 1 页错误：%+v", res)
	}
	res2, _ := svc.List(context.Background(), wd, ListFilter{Page: 3, Size: 3})
	if len(res2.Items) != 1 {
		t.Errorf("第 3 页应剩 1 条，got %d", len(res2.Items))
	}
	// 越界页返回空 items、total 保留。
	res3, _ := svc.List(context.Background(), wd, ListFilter{Page: 99, Size: 3})
	if len(res3.Items) != 0 || res3.Total != 7 {
		t.Errorf("越界页应返回空 items 且 total=7，got %+v", res3)
	}
	// 分页模式返回扁平 items（无 Tree）。
	if res.Tree != nil {
		t.Error("分页模式不应返回 Tree")
	}
}

func TestList_PaginationDefaults(t *testing.T) {
	svc, wd := newTestEnv(t)
	for i := 0; i < 3; i++ {
		mustCreate(t, svc, wd, CreateInput{Title: "t"})
	}
	res, _ := svc.List(context.Background(), wd, ListFilter{Page: 1})
	if res.Size != defaultPageSize {
		t.Errorf("默认 size 应为 %d，got %d", defaultPageSize, res.Size)
	}
	resBig, _ := svc.List(context.Background(), wd, ListFilter{Page: 1, Size: 9999})
	if resBig.Size != maxPageSize {
		t.Errorf("size 应截断为 %d，got %d", maxPageSize, resBig.Size)
	}
}

// ---- List：项目隔离 ----

func TestList_ProjectIsolation(t *testing.T) {
	svc, wdA := newTestEnv(t)
	svcB, wdB := newTestEnv(t)

	mustCreate(t, svc, wdA, CreateInput{Title: "A 项目任务"})
	mustCreate(t, svcB, wdB, CreateInput{Title: "B 项目任务"})

	// 各自 List 只看到各自数据（一项目一库文件隔离）。
	resA, _ := svc.List(context.Background(), wdA, ListFilter{})
	if len(resA.Tree) != 1 || resA.Tree[0].Task.Title != "A 项目任务" {
		t.Errorf("A 项目应只见自身任务：%v", names(resA.Tree))
	}
	resB, _ := svcB.List(context.Background(), wdB, ListFilter{})
	if len(resB.Tree) != 1 || resB.Tree[0].Task.Title != "B 项目任务" {
		t.Errorf("B 项目应只见自身任务：%v", names(resB.Tree))
	}
}

// ---- Update ----

func TestUpdate_Partial(t *testing.T) {
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "原标题", Description: "原描述", Tags: []string{"a", "b"}})

	// 只更新 title，其余字段不变。
	updated, err := svc.Update(context.Background(), wd, task.ID, UpdateInput{Title: strPtr("新标题")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != "新标题" {
		t.Errorf("Title 未更新：%q", updated.Title)
	}
	if updated.Description != "原描述" {
		t.Errorf("Description 不应被影响：%q", updated.Description)
	}
	if updated.Status != StatusTodo {
		t.Errorf("Status 不应被影响：%q", updated.Status)
	}
	if len(updated.Tags) != 2 {
		t.Errorf("Tags 不应被影响：%v", updated.Tags)
	}
}

func TestUpdate_AllFields(t *testing.T) {
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})

	priority := any(5)
	empty := []string{}
	updated, err := svc.Update(context.Background(), wd, task.ID, UpdateInput{
		Title:       strPtr("改标题"),
		Description: strPtr(""),
		Priority:    &priority,
		Tags:        &empty, // &[] = 清空
		Assignee:    strPtr("李四"),
		DependsOn:   &empty,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != "改标题" || updated.Description != "" || updated.Priority != 5 {
		t.Errorf("字段更新错误：%+v", updated)
	}
	if len(updated.Tags) != 0 || len(updated.DependsOn) != 0 {
		t.Errorf("tags/depends_on 应清空：%v / %v", updated.Tags, updated.DependsOn)
	}
	if updated.Assignee != "李四" {
		t.Errorf("Assignee = %q", updated.Assignee)
	}
	if !updated.UpdatedAt.After(task.UpdatedAt) && !updated.UpdatedAt.Equal(task.UpdatedAt) {
		t.Errorf("updated_at 应刷新")
	}
}

func TestUpdate_PriorityInvalid(t *testing.T) {
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	bad := any("超高")
	_, err := svc.Update(context.Background(), wd, task.ID, UpdateInput{Priority: &bad})
	if !errors.Is(err, ErrTaskInvalid) {
		t.Errorf("非法 priority 应返回 TASK_INVALID，got %v", err)
	}
}

func TestUpdate_TitleEmpty(t *testing.T) {
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	_, err := svc.Update(context.Background(), wd, task.ID, UpdateInput{Title: strPtr("  ")})
	if !errors.Is(err, ErrTaskInvalid) {
		t.Errorf("空白 title 应返回 TASK_INVALID，got %v", err)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	svc, wd := newTestEnv(t)
	_, err := svc.Update(context.Background(), wd, "missing", UpdateInput{Title: strPtr("x")})
	if !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("应返回 TASK_NOT_FOUND，got %v", err)
	}
}

func TestUpdate_StatusNotAffected(t *testing.T) {
	// Q8 语义：UpdateInput 无 status 字段（编译期保证），此处验证 Update 不改动 status。
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t", Status: strPtr("doing")})
	updated, err := svc.Update(context.Background(), wd, task.ID, UpdateInput{Title: strPtr("新")})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "doing" {
		t.Errorf("Update 不得修改 status：got %q", updated.Status)
	}
}

func TestUpdate_ParentChange(t *testing.T) {
	svc, wd := newTestEnv(t)
	a := mustCreate(t, svc, wd, CreateInput{Title: "A"})
	b := mustCreate(t, svc, wd, CreateInput{Title: "B"})

	// 改父：A → B。
	updated, err := svc.Update(context.Background(), wd, a.ID, UpdateInput{ParentID: strPtrPtr(strPtr(b.ID))})
	if err != nil {
		t.Fatalf("Update parent: %v", err)
	}
	if updated.ParentID == nil || *updated.ParentID != b.ID {
		t.Errorf("parent 应指向 B，got %v", updated.ParentID)
	}

	// 置顶：ParentID = &nil。
	updated2, err := svc.Update(context.Background(), wd, a.ID, UpdateInput{ParentID: strPtrPtr(nil)})
	if err != nil {
		t.Fatalf("Update parent to nil: %v", err)
	}
	if updated2.ParentID != nil {
		t.Errorf("parent 应置空（顶层），got %v", *updated2.ParentID)
	}
}

func TestUpdate_ParentNotFound(t *testing.T) {
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	_, err := svc.Update(context.Background(), wd, task.ID, UpdateInput{ParentID: strPtrPtr(strPtr("ghost"))})
	if !errors.Is(err, ErrParentNotFound) {
		t.Errorf("应返回 PARENT_NOT_FOUND，got %v", err)
	}
}

func TestUpdate_ParentCycle(t *testing.T) {
	svc, wd := newTestEnv(t)
	a := mustCreate(t, svc, wd, CreateInput{Title: "A"})
	b := mustCreate(t, svc, wd, CreateInput{Title: "B", ParentID: strPtr(a.ID)})

	// A 的父设为 B → A 是 B 的祖先，环。B.parent=A 已存在，A.parent=B 会形成 A→B→A。
	_, err := svc.Update(context.Background(), wd, a.ID, UpdateInput{ParentID: strPtrPtr(strPtr(b.ID))})
	if !errors.Is(err, ErrParentCycle) {
		t.Errorf("父链环应返回 PARENT_CYCLE，got %v", err)
	}

	// 自引用：A 的父设为 A。
	_, err = svc.Update(context.Background(), wd, a.ID, UpdateInput{ParentID: strPtrPtr(strPtr(a.ID))})
	if !errors.Is(err, ErrParentCycle) {
		t.Errorf("自引用应返回 PARENT_CYCLE，got %v", err)
	}
}

// ---- ChangeStatus ----

func TestChangeStatus_Ok(t *testing.T) {
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	updated, err := svc.ChangeStatus(context.Background(), wd, task.ID, "doing")
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if updated.Status != "doing" {
		t.Errorf("Status = %q", updated.Status)
	}
}

func TestChangeStatus_Errors(t *testing.T) {
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})

	// 任务不存在。
	if _, err := svc.ChangeStatus(context.Background(), wd, "ghost", "doing"); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("应返回 TASK_NOT_FOUND，got %v", err)
	}
	// archived 保留态。
	if _, err := svc.ChangeStatus(context.Background(), wd, task.ID, StatusArchived); !errors.Is(err, ErrStatusNotFound) {
		t.Errorf("archived 应被拒绝（STATUS_NOT_FOUND），got %v", err)
	}
	// 未知状态。
	if _, err := svc.ChangeStatus(context.Background(), wd, task.ID, "nonexistent"); !errors.Is(err, ErrStatusNotFound) {
		t.Errorf("未知状态应返回 STATUS_NOT_FOUND，got %v", err)
	}
}

// ---- 项目识别（Q2-B：不依赖全局注册表） ----

func TestProjectNotFound(t *testing.T) {
	svc, _ := newTestEnv(t)
	// 未初始化元数据的目录 → PROJECT_NOT_FOUND。
	missing := t.TempDir()
	_, err := svc.Get(context.Background(), missing, "x")
	if !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("无 meta.db 的目录应返回 PROJECT_NOT_FOUND，got %v", err)
	}
	// 非绝对路径 → PROJECT_NOT_FOUND。
	_, err = svc.Create(context.Background(), "relative/path", CreateInput{Title: "t"})
	if !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("非绝对路径应返回 PROJECT_NOT_FOUND，got %v", err)
	}
}

// ---- 写钩子（Q14-A：审计/WS 预留） ----

func TestWriteHook(t *testing.T) {
	var calls int32
	svc := NewService(Options{
		OnWrite: func(_ context.Context, action, target string) {
			atomic.AddInt32(&calls, 1)
			if action != "task.created" && action != "task.updated" && action != "task.status_changed" {
				t.Errorf("未知 action：%s", action)
			}
			if target == "" {
				t.Error("target 不应为空")
			}
		},
	})
	wd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wd, ".taskboard"), 0o755); err != nil {
		t.Fatal(err)
	}
	conn, err := db.EnsureProject(context.Background(), db.MetaDBPath(wd))
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	t.Cleanup(func() { _ = svc.Close() })

	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	_, _ = svc.Update(context.Background(), wd, task.ID, UpdateInput{Title: strPtr("x")})
	_, _ = svc.ChangeStatus(context.Background(), wd, task.ID, "done")
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("写钩子应触发 3 次，got %d", got)
	}
}

// ---- 内部工具（白盒） ----

func TestNormalizeTags(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{nil, []string{}},
		{[]string{}, []string{}},
		{[]string{"a", "a", "b"}, []string{"a", "b"}},
		{[]string{" ", "a", "", "b"}, []string{"a", "b"}},
		{[]string{"bug", "bug", "ui"}, []string{"bug", "ui"}},
	}
	for _, c := range cases {
		got := normalizeTags(c.in)
		if len(got) != len(c.want) {
			t.Errorf("normalizeTags(%v) = %v，期望 %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("normalizeTags(%v) = %v，期望 %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestCodeOf(t *testing.T) {
	if codeOf(ErrTaskNotFound) != CodeTaskNotFound {
		t.Error("codeOf(ErrTaskNotFound) 提取失败")
	}
	if codeOf(errors.New("普通错误")) != "" {
		t.Error("普通错误应返回空码")
	}
}

// names 提取树节点标题（测试断言辅助）。
func names(nodes []*TaskTreeNode) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Title)
	}
	return out
}
