package task

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"tangoforge/internal/config"
	"tangoforge/internal/db"
)

// openTestConn 打开已初始化的项目库连接（测试直接访问 repo 用）。
func openTestConn(workdir string) (*sql.DB, error) {
	return db.EnsureProject(context.Background(), db.MetaDBPath(workdir))
}

// saveSM 将自定义状态机写入项目 config.yaml（保留其它配置节，模拟编辑持久化）。
func saveSM(t *testing.T, workdir string, sm config.StateMachine) {
	t.Helper()
	cfg, err := config.LoadProject(workdir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.StateMachine = sm
	if err := config.SaveProject(workdir, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

// customSM 构造一个自定义状态机（todo→[doing]，doing→[done]，done→[todo]）。
func customSM() config.StateMachine {
	return config.StateMachine{
		States: []config.State{
			{Key: "todo", Label: "待办", Color: "#9aa0a6"},
			{Key: "doing", Label: "进行中", Color: "#1a73e8"},
			{Key: "done", Label: "已完成", Color: "#34a853"},
		},
		Transitions: []config.Transition{
			{From: "todo", To: []string{"doing"}},
			{From: "doing", To: []string{"done"}},
			{From: "done", To: []string{"todo"}},
		},
	}
}

// ---- 状态机加载 ----

func TestLoadStateMachine_Default(t *testing.T) {
	_, wd := newTestEnv(t) // 无 config.yaml → 默认四态
	sm, err := loadStateMachine(wd)
	if err != nil {
		t.Fatalf("loadStateMachine: %v", err)
	}
	if len(sm.States) != 3 {
		t.Fatalf("默认应含 3 态（todo/doing/done），got %d", len(sm.States))
	}
	if !stateExists(sm, StatusTodo) || !stateExists(sm, StatusDoing) || !stateExists(sm, StatusDone) {
		t.Errorf("默认状态机缺状态：%+v", sm.States)
	}
}

func TestLoadStateMachine_Custom(t *testing.T) {
	_, wd := newTestEnv(t)
	saveSM(t, wd, customSM())
	sm, err := loadStateMachine(wd)
	if err != nil {
		t.Fatal(err)
	}
	if len(sm.Transitions) != 3 || sm.Transitions[0].From != "todo" {
		t.Errorf("自定义状态机加载失败：%+v", sm.Transitions)
	}
}

// ---- validateTransition 白盒：合法/非法矩阵 ----

func TestValidateTransition_Matrix(t *testing.T) {
	// 默认状态机：三态互转（回退允许）。
	def := config.DefaultStateMachine()
	valid := [][2]string{{"todo", "doing"}, {"todo", "done"}, {"doing", "todo"}, {"doing", "done"}, {"done", "doing"}, {"done", "todo"}}
	for _, pair := range valid {
		if err := validateTransition(def, pair[0], pair[1]); err != nil {
			t.Errorf("默认状态机合法流转 %s→%s 被拒绝：%v", pair[0], pair[1], err)
		}
	}

	// 自定义：todo→[doing]，则 todo→done 非法。
	custom := config.StateMachine{
		States:      []config.State{{Key: "todo"}, {Key: "doing"}, {Key: "done"}},
		Transitions: []config.Transition{{From: "todo", To: []string{"doing"}}},
	}
	if err := validateTransition(custom, "todo", "doing"); err != nil {
		t.Errorf("todo→doing 应合法：%v", err)
	}
	if err := validateTransition(custom, "todo", "done"); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("todo→done 应非法（INVALID_TRANSITION），got %v", err)
	}
}

func TestValidateTransition_EmptyTransitionsRejectsAll(t *testing.T) {
	// Q3-A：states 自定义 + transitions 空 → 拒绝一切流转。
	sm := config.StateMachine{States: []config.State{{Key: "todo"}, {Key: "doing"}}}
	if err := validateTransition(sm, "todo", "doing"); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("transitions 空应拒绝一切流转，got %v", err)
	}
}

func TestValidateTransition_UndefinedFromAllows(t *testing.T) {
	// Q1-B：transitions 非空但 from 未定义 → 放行任意流转。
	sm := config.StateMachine{
		States:      []config.State{{Key: "todo"}, {Key: "doing"}, {Key: "done"}},
		Transitions: []config.Transition{{From: "todo", To: []string{"doing"}}},
	}
	// doing 无规则 → doing→done 放行。
	if err := validateTransition(sm, "doing", "done"); err != nil {
		t.Errorf("from 未定义应放行（Q1-B），got %v", err)
	}
}

// ---- ChangeStatus 流转校验（服务层） ----

func TestChangeStatus_TransitionEnforced(t *testing.T) {
	svc, wd := newTestEnv(t)
	saveSM(t, wd, customSM()) // todo→doing→done→todo 链

	task := mustCreate(t, svc, wd, CreateInput{Title: "t"}) // todo
	// 合法：todo→doing。
	if _, err := svc.ChangeStatus(context.Background(), wd, task.ID, "doing"); err != nil {
		t.Fatalf("todo→doing 应合法：%v", err)
	}
	// 非法：todo→done（todo 规则只允许 doing）——用新任务验证（当前任务已是 doing，doing→done 合法）。
	task2 := mustCreate(t, svc, wd, CreateInput{Title: "t2"})
	_, err := svc.ChangeStatus(context.Background(), wd, task2.ID, "done")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("todo→done 应 INVALID_TRANSITION，got %v", err)
	}
	// 任务状态保持 todo（拒绝时不产生脏数据）。
	got, _ := svc.Get(context.Background(), wd, task2.ID)
	if got.Status != "todo" {
		t.Errorf("非法流转后状态应保持 todo，got %q", got.Status)
	}
}

func TestChangeStatus_EmptyTransitionsRejects(t *testing.T) {
	svc, wd := newTestEnv(t)
	// Q3-A：states 自定义 + transitions 空 → 拒绝一切流转。
	saveSM(t, wd, config.StateMachine{States: []config.State{{Key: "todo"}, {Key: "doing"}}})

	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	_, err := svc.ChangeStatus(context.Background(), wd, task.ID, "doing")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("transitions 空应拒绝流转（Q3-A），got %v", err)
	}
}

func TestChangeStatus_UndefinedFromAllows(t *testing.T) {
	svc, wd := newTestEnv(t)
	// Q1-B：doing 无流转规则 → doing→done 放行。
	sm := config.StateMachine{
		States:      []config.State{{Key: "todo"}, {Key: "doing"}, {Key: "done"}},
		Transitions: []config.Transition{{From: "todo", To: []string{"doing"}}},
	}
	saveSM(t, wd, sm)

	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	if _, err := svc.ChangeStatus(context.Background(), wd, task.ID, "doing"); err != nil {
		t.Fatal(err)
	}
	updated, err := svc.ChangeStatus(context.Background(), wd, task.ID, "done") // doing 无规则 → 放行
	if err != nil {
		t.Fatalf("doing→done 应放行（Q1-B），got %v", err)
	}
	if updated.Status != "done" {
		t.Errorf("Status = %q", updated.Status)
	}
}

func TestChangeStatus_SameStatusIdempotent(t *testing.T) {
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t", Status: strPtr("doing")})

	updated, err := svc.ChangeStatus(context.Background(), wd, task.ID, "doing")
	if err != nil {
		t.Fatalf("同态流转应幂等成功（Q2-A），got %v", err)
	}
	if updated.Status != "doing" {
		t.Errorf("Status = %q", updated.Status)
	}
	// 秒级精度比较：DB 存 RFC3339 文本（无亚秒），Create 返回对象含纳秒，此处仅验证同态不刷新。
	if updated.UpdatedAt.Unix() != task.UpdatedAt.Unix() {
		t.Errorf("同态流转不应刷新 updated_at")
	}
}

func TestChangeStatus_ArchivedStillExcluded(t *testing.T) {
	svc, wd := newTestEnv(t)
	task := mustCreate(t, svc, wd, CreateInput{Title: "t"})
	// archived 保留态：即使出现在自定义 transitions 中也不可普通流转（TF-006 保持 TF-005 语义）。
	saveSM(t, wd, config.StateMachine{
		States:      []config.State{{Key: "todo"}, {Key: "doing"}},
		Transitions: []config.Transition{{From: "todo", To: []string{"doing"}}},
	})
	if _, err := svc.ChangeStatus(context.Background(), wd, task.ID, StatusArchived); !errors.Is(err, ErrStatusNotFound) {
		t.Errorf("archived 仍应被拒绝（STATUS_NOT_FOUND），got %v", err)
	}
}

// ---- GetStateMachine ----

func TestGetStateMachine(t *testing.T) {
	svc, wd := newTestEnv(t)
	// 缺失 → 默认三态。
	sm, err := svc.GetStateMachine(context.Background(), wd)
	if err != nil {
		t.Fatal(err)
	}
	if len(sm.States) != 3 || !stateExists(sm, "todo") {
		t.Errorf("默认状态机：%+v", sm.States)
	}
	// 自定义后读取一致。
	custom := customSM()
	saveSM(t, wd, custom)
	sm2, _ := svc.GetStateMachine(context.Background(), wd)
	if len(sm2.Transitions) != 3 || sm2.Transitions[0].To[0] != "doing" {
		t.Errorf("自定义状态机读取不一致：%+v", sm2)
	}
}

// ---- UpdateStateMachine ----

func TestUpdateStateMachine_OkAndPersist(t *testing.T) {
	svc, wd := newTestEnv(t)
	custom := customSM()
	got, err := svc.UpdateStateMachine(context.Background(), wd, custom)
	if err != nil {
		t.Fatalf("UpdateStateMachine: %v", err)
	}
	if len(got.States) != 3 {
		t.Errorf("返回规范化状态机：%+v", got.States)
	}
	// 持久化往返：重新 Get 一致。
	reloaded, _ := svc.GetStateMachine(context.Background(), wd)
	if len(reloaded.States) != 3 || reloaded.Transitions[1].From != "doing" {
		t.Errorf("持久化后重载不一致：%+v", reloaded)
	}
}

func TestUpdateStateMachine_KeepsOtherConfig(t *testing.T) {
	svc, wd := newTestEnv(t)
	// 先设置 export 自定义模板路径，编辑状态机后应保留（Q8-A）。
	cfg, err := config.LoadProject(wd)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Export.TemplatePath = "custom.tmpl"
	if err := config.SaveProject(wd, cfg); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.UpdateStateMachine(context.Background(), wd, customSM()); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := config.LoadProject(wd)
	if reloaded.Export.TemplatePath != "custom.tmpl" {
		t.Errorf("编辑状态机应保留 export 节，got %q", reloaded.Export.TemplatePath)
	}
}

func TestUpdateStateMachine_Validation(t *testing.T) {
	svc, wd := newTestEnv(t)
	cases := []struct {
		name string
		sm   config.StateMachine
	}{
		{"空状态集", config.StateMachine{}},
		{"key 重复", config.StateMachine{States: []config.State{{Key: "todo"}, {Key: "todo"}}}},
		{"key 为 archived", config.StateMachine{States: []config.State{{Key: StatusArchived}}}},
		{"from 不存在", config.StateMachine{
			States:      []config.State{{Key: "todo"}},
			Transitions: []config.Transition{{From: "ghost", To: []string{"todo"}}},
		}},
		{"to 不存在", config.StateMachine{
			States:      []config.State{{Key: "todo"}},
			Transitions: []config.Transition{{From: "todo", To: []string{"ghost"}}},
		}},
	}
	for _, c := range cases {
		_, err := svc.UpdateStateMachine(context.Background(), wd, c.sm)
		if !errors.Is(err, ErrTaskInvalid) {
			t.Errorf("%s：应返回 TASK_INVALID，got %v", c.name, err)
		}
	}
}

func TestUpdateStateMachine_StatusInUse(t *testing.T) {
	svc, wd := newTestEnv(t)
	mustCreate(t, svc, wd, CreateInput{Title: "占用任务"}) // 默认 status=todo

	// 删除被占用的 todo → STATUS_IN_USE（Message 携带占用数 1）。
	sm := config.StateMachine{
		States: []config.State{{Key: "doing"}, {Key: "done"}},
		Transitions: []config.Transition{
			{From: "doing", To: []string{"done"}},
			{From: "done", To: []string{"doing"}},
		},
	}
	_, err := svc.UpdateStateMachine(context.Background(), wd, sm)
	if !errors.Is(err, ErrStatusInUse) {
		t.Fatalf("删除占用状态应 STATUS_IN_USE，got %v", err)
	}
	if !strings.Contains(err.Error(), "1 个任务") {
		t.Errorf("STATUS_IN_USE 应携带占用数，got %q", err.Error())
	}

	// 重命名（todo 消失 + todoX 新增）同样被拒。
	renameSM := config.StateMachine{
		States: []config.State{{Key: "todoX"}, {Key: "doing"}, {Key: "done"}},
		Transitions: []config.Transition{
			{From: "todoX", To: []string{"doing"}},
			{From: "doing", To: []string{"done"}},
			{From: "done", To: []string{"todoX"}},
		},
	}
	if _, err := svc.UpdateStateMachine(context.Background(), wd, renameSM); !errors.Is(err, ErrStatusInUse) {
		t.Errorf("重命名占用状态应 STATUS_IN_USE，got %v", err)
	}

	// label/color 修改（key 不变）→ 放行。
	labelSM := config.StateMachine{
		States: []config.State{{Key: "todo", Label: "待办改", Color: "#111111"}, {Key: "doing"}, {Key: "done"}},
		Transitions: []config.Transition{
			{From: "todo", To: []string{"doing"}},
			{From: "doing", To: []string{"done"}},
			{From: "done", To: []string{"todo"}},
		},
	}
	if _, err := svc.UpdateStateMachine(context.Background(), wd, labelSM); err != nil {
		t.Errorf("修改占用状态 label/color 应放行，got %v", err)
	}
}

func TestUpdateStateMachine_StatusInUseZeroAllowed(t *testing.T) {
	svc, wd := newTestEnv(t)
	// 无任务占用 todo → 删除 todo 允许。
	sm := config.StateMachine{
		States:      []config.State{{Key: "doing"}, {Key: "done"}},
		Transitions: []config.Transition{{From: "doing", To: []string{"done"}}},
	}
	if _, err := svc.UpdateStateMachine(context.Background(), wd, sm); err != nil {
		t.Errorf("无占用时可删除状态，got %v", err)
	}
}

func TestUpdateStateMachine_Normalize(t *testing.T) {
	svc, wd := newTestEnv(t)
	sm := config.StateMachine{
		States: []config.State{{Key: " todo ", Label: "待办"}, {Key: "doing"}, {Key: "done"}},
		Transitions: []config.Transition{
			{From: "todo", To: []string{"doing", "doing", "done"}}, // to 去重
		},
	}
	got, err := svc.UpdateStateMachine(context.Background(), wd, sm)
	if err != nil {
		t.Fatalf("UpdateStateMachine: %v", err)
	}
	if got.States[0].Key != "todo" {
		t.Errorf("key 应去空白，got %q", got.States[0].Key)
	}
	if len(got.Transitions[0].To) != 2 {
		t.Errorf("to 应去重，got %v", got.Transitions[0].To)
	}
}

func TestUpdateStateMachine_WriteHook(t *testing.T) {
	var calls int32
	var action string
	svc := NewService(Options{
		OnWrite: func(_ context.Context, a, _ string) {
			atomic.AddInt32(&calls, 1)
			action = a
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
	saveSM(t, wd, customSM())
	t.Cleanup(func() { _ = svc.Close() })

	if _, err := svc.UpdateStateMachine(context.Background(), wd, customSM()); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&calls) != 1 || action != "state_machine.changed" {
		t.Errorf("应触发 state_machine.changed 钩子，got calls=%d action=%q", calls, action)
	}
}

// ---- 状态占用统计口径（Q7-A：archived 不参与） ----

func TestStatusUsage_ExcludesArchived(t *testing.T) {
	svc, wd := newTestEnv(t)
	mustCreate(t, svc, wd, CreateInput{Title: "t"}) // 默认 status=todo
	conn, err := openTestConn(wd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	u, err := svc.(*service).statusUsage(context.Background(), newSQLRepo(conn))
	if err != nil {
		t.Fatal(err)
	}
	if u["todo"] != 1 {
		t.Errorf("todo 占用应为 1，got %d", u["todo"])
	}
	if _, ok := u["archived"]; ok {
		t.Error("archived 不应出现在占用统计中")
	}
}
