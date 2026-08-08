package task

import (
	"context"
	"os"
	"path/filepath"
	"tangoforge/internal/config"
	"tangoforge/internal/db"
	"testing"
	"time"
)

// TF-009 覆盖率收口——错误分支注入测试（QA Q9-A 策略，不引入新依赖）：
//
//  1. projectDB 打开失败：把 meta.db 建成目录（os.Stat 通过但 SQLite Open 失败）
//     → 覆盖全部 Service 方法的 projectDB 错误分支；
//  2. repo 层错误：关闭连接后调用（sql.ErrClosed）→ 覆盖 TaskRepo 各方法错误分支；
//  3. 项目配置损坏：config.yaml 写入非法 YAML → 覆盖 loadStateMachine 错误分支。

func TestServiceMethods_ProjectDBError(t *testing.T) {
	// meta.db 为目录 → Stat 存在但 SQLite 无法打开 → 所有 Service 方法返回错误。
	wd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wd, ".taskboard", "meta.db"), 0o755); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Options{})
	ctx := context.Background()

	cases := []struct {
		name string
		run  func() error
	}{
		{"Create", func() error { _, err := svc.Create(ctx, wd, CreateInput{Title: "t"}); return err }},
		{"Get", func() error { _, err := svc.Get(ctx, wd, "x"); return err }},
		{"List", func() error { _, err := svc.List(ctx, wd, ListFilter{}); return err }},
		{"Update", func() error { _, err := svc.Update(ctx, wd, "x", UpdateInput{}); return err }},
		{"ChangeStatus", func() error { _, err := svc.ChangeStatus(ctx, wd, "x", "doing"); return err }},
		{"GetStateMachine", func() error { _, err := svc.GetStateMachine(ctx, wd); return err }},
		{"UpdateStateMachine", func() error { _, err := svc.UpdateStateMachine(ctx, wd, config.StateMachine{}); return err }},
		{"Archive", func() error { _, err := svc.Archive(ctx, wd, "x"); return err }},
		{"Restore", func() error { _, err := svc.Restore(ctx, wd, "x", RestoreOptions{}); return err }},
		{"Delete", func() error { _, err := svc.Delete(ctx, wd, "x"); return err }},
	}
	for _, c := range cases {
		if err := c.run(); err == nil {
			t.Errorf("%s: meta.db 为目录时应返回错误", c.name)
		}
	}
}

func TestRepoErrors_ClosedConnection(t *testing.T) {
	wd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wd, ".taskboard"), 0o755); err != nil {
		t.Fatal(err)
	}
	conn, err := db.EnsureProject(context.Background(), db.MetaDBPath(wd))
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close() // 关闭连接 → 后续操作返回 sql.ErrClosed
	repo := newSQLRepo(conn)
	ctx := context.Background()
	now := time.Now()
	task := &Task{ID: "x", Title: "t", Status: "todo", CreatedAt: now, UpdatedAt: now}

	if err := repo.Create(ctx, task); err == nil {
		t.Error("Create 关闭连接应失败")
	}
	if _, err := repo.GetByID(ctx, "x"); err == nil {
		t.Error("GetByID 关闭连接应失败")
	}
	if _, err := repo.List(ctx); err == nil {
		t.Error("List 关闭连接应失败")
	}
	if err := repo.Update(ctx, task); err == nil {
		t.Error("Update 关闭连接应失败")
	}
	if err := repo.Delete(ctx, "x"); err == nil {
		t.Error("Delete 关闭连接应失败")
	}
	if _, err := repo.ClearParentsByParentID(ctx, "x", now); err == nil {
		t.Error("ClearParentsByParentID 关闭连接应失败")
	}
}

func TestService_ConfigCorrupt(t *testing.T) {
	// config.yaml 非法 YAML（tab 缩进）→ loadStateMachine 解析失败 → Create/ChangeStatus 状态校验错误路径。
	svc, wd := newTestEnv(t)
	corrupt := []byte("\tbad: indent\n\t- a\n")
	if err := os.WriteFile(filepath.Join(wd, ".taskboard", "config.yaml"), corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := svc.Create(ctx, wd, CreateInput{Title: "t", Status: strPtr("doing")}); err == nil {
		t.Error("Create: config 损坏应返回错误")
	}
	task := mustCreate(t, svc, wd, CreateInput{Title: "t2"}) // 默认 todo 不读状态机
	if _, err := svc.ChangeStatus(ctx, wd, task.ID, "doing"); err == nil {
		t.Error("ChangeStatus: config 损坏应返回错误")
	}
}
