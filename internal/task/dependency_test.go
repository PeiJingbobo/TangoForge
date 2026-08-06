package task

import (
	"context"
	"errors"
	"testing"
)

// ---- Create 依赖校验 ----

func TestCreate_DependencyValid(t *testing.T) {
	svc, wd := newTestEnv(t)
	a := mustCreate(t, svc, wd, CreateInput{Title: "A"})
	b := mustCreate(t, svc, wd, CreateInput{Title: "B", DependsOn: []string{a.ID}})
	if len(b.DependsOn) != 1 || b.DependsOn[0] != a.ID {
		t.Errorf("B 应依赖 A，got %v", b.DependsOn)
	}
}

func TestCreate_DependencyNotFound(t *testing.T) {
	svc, wd := newTestEnv(t)
	_, err := svc.Create(context.Background(), wd, CreateInput{Title: "B", DependsOn: []string{"ghost"}})
	if !errors.Is(err, ErrDependencyNotFound) {
		t.Fatalf("依赖不存在应 DEPENDENCY_NOT_FOUND，got %v", err)
	}
	// 任务不落库（Q4-A：拒绝不产生脏数据）。
	res, _ := svc.List(context.Background(), wd, ListFilter{})
	if len(res.Tree) != 0 {
		t.Errorf("创建被拒后不应落库，got %d 个任务", len(res.Tree))
	}
}

// ---- Update 依赖校验 ----

func TestUpdate_DependencyNotFound(t *testing.T) {
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	_, err := svc.Update(context.Background(), wd, task.ID, UpdateInput{DependsOn: &[]string{"ghost"}})
	if !errors.Is(err, ErrDependencyNotFound) {
		t.Fatalf("应 DEPENDENCY_NOT_FOUND，got %v", err)
	}
	// 状态不脏：depends_on 保持原值（空）。
	got, _ := svc.Get(context.Background(), wd, task.ID)
	if len(got.DependsOn) != 0 {
		t.Errorf("更新被拒后 depends_on 不应改变，got %v", got.DependsOn)
	}
}

func TestUpdate_CycleRejected(t *testing.T) {
	// A 依赖 B；更新 B 依赖 A → 环拒绝。
	svc, wd := newTestEnv(t)
	b := mustCreate(t, svc, wd, CreateInput{Title: "B"})
	a := mustCreate(t, svc, wd, CreateInput{Title: "A", DependsOn: []string{b.ID}})

	_, err := svc.Update(context.Background(), wd, b.ID, UpdateInput{DependsOn: &[]string{a.ID}})
	if !errors.Is(err, ErrCircularDependency) {
		t.Fatalf("A→B→A 应 CIRCULAR_DEPENDENCY，got %v", err)
	}
	// B 的 depends_on 不脏（仍为空）。
	got, _ := svc.Get(context.Background(), wd, b.ID)
	if len(got.DependsOn) != 0 {
		t.Errorf("环拒绝后 B.depends_on 不应改变，got %v", got.DependsOn)
	}
}

func TestUpdate_SelfDependencyRejected(t *testing.T) {
	// 自依赖：X 依赖 X → 环拒绝（Q7-A，Update 路径可达）。
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	_, err := svc.Update(context.Background(), wd, task.ID, UpdateInput{DependsOn: &[]string{task.ID}})
	if !errors.Is(err, ErrCircularDependency) {
		t.Fatalf("自依赖应 CIRCULAR_DEPENDENCY，got %v", err)
	}
}

func TestUpdate_MultiHopCycleRejected(t *testing.T) {
	// 多跳环（Q5-A）：A→B→C，更新 C 依赖 A → C→A→B→C 环。
	svc, wd := newTestEnv(t)
	c := mustCreate(t, svc, wd, CreateInput{Title: "C"})
	b := mustCreate(t, svc, wd, CreateInput{Title: "B", DependsOn: []string{c.ID}})
	a := mustCreate(t, svc, wd, CreateInput{Title: "A", DependsOn: []string{b.ID}})

	_, err := svc.Update(context.Background(), wd, c.ID, UpdateInput{DependsOn: &[]string{a.ID}})
	if !errors.Is(err, ErrCircularDependency) {
		t.Fatalf("多跳环 C→A→B→C 应 CIRCULAR_DEPENDENCY，got %v", err)
	}
	// 合法场景对照：C 依赖 A 本身成环，但若 C 依赖不构成环的节点则放行。
	d := mustCreate(t, svc, wd, CreateInput{Title: "D"})
	updated, err := svc.Update(context.Background(), wd, c.ID, UpdateInput{DependsOn: &[]string{d.ID}})
	if err != nil {
		t.Fatalf("合法依赖应放行：%v", err)
	}
	if len(updated.DependsOn) != 1 || updated.DependsOn[0] != d.ID {
		t.Errorf("depends_on 更新错误：%v", updated.DependsOn)
	}
}

func TestUpdate_DependsOnClearAndNoop(t *testing.T) {
	svc, wd := newTestEnv(t)
	a := mustCreate(t, svc, wd, CreateInput{Title: "A"})
	task := mustCreate(t, svc, wd, CreateInput{Title: "t", DependsOn: []string{a.ID}})

	// &[] = 清空（Q4.2 指针语义）。
	cleared, err := svc.Update(context.Background(), wd, task.ID, UpdateInput{DependsOn: &[]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared.DependsOn) != 0 {
		t.Errorf("清空失败：%v", cleared.DependsOn)
	}
	// nil = 不更新。
	noop, err := svc.Update(context.Background(), wd, task.ID, UpdateInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(noop.DependsOn) != 0 {
		t.Errorf("nil 不应更新 depends_on：%v", noop.DependsOn)
	}
}

// ---- 依赖已归档任务（Q3-A） ----

func TestDependency_ArchivedAllowed(t *testing.T) {
	svc, wd := newTestEnv(t)
	a := mustCreate(t, svc, wd, CreateInput{Title: "A"})
	if _, err := svc.Archive(context.Background(), wd, a.ID); err != nil {
		t.Fatal(err)
	}
	// 依赖已归档任务 → 允许。
	b := mustCreate(t, svc, wd, CreateInput{Title: "B", DependsOn: []string{a.ID}})
	if len(b.DependsOn) != 1 {
		t.Errorf("依赖 archived 任务应允许，got %v", b.DependsOn)
	}
}

// ---- validateDependencies 白盒 ----

func TestValidateDependencies_WhiteBox(t *testing.T) {
	svc, wd := newTestEnv(t)
	conn, err := openTestConn(wd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	repo := newSQLRepo(conn)
	ctx := context.Background()

	// 空集合直接通过。
	if err := svc.(*service).validateDependencies(ctx, repo, "x", nil); err != nil {
		t.Errorf("空集合应通过：%v", err)
	}
	// Create 场景（taskID=""）：仅存在性，无环（新任务无入边）。
	a := mustCreate(t, svc, wd, CreateInput{Title: "A"})
	if err := svc.(*service).validateDependencies(ctx, repo, "", []string{a.ID}); err != nil {
		t.Errorf("新任务依赖已存在任务应通过：%v", err)
	}
	if err := svc.(*service).validateDependencies(ctx, repo, "", []string{"ghost"}); !errors.Is(err, ErrDependencyNotFound) {
		t.Errorf("新任务依赖不存在应拒绝，got %v", err)
	}
	// 自依赖（白盒直调，Create 路径 id 不可预知，由 Update 覆盖）。
	if err := svc.(*service).validateDependencies(ctx, repo, a.ID, []string{a.ID}); !errors.Is(err, ErrCircularDependency) {
		t.Errorf("自依赖应拒绝，got %v", err)
	}
}
