package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"tangoforge/internal/db"
)

// TestInit_OnlyInitializesMeta（QA P4-1 Q6：project_init 语义）。
func TestInit_OnlyInitializesMeta(t *testing.T) {
	svc := newService(t)
	workdir := t.TempDir()

	if err := svc.Init(context.Background(), workdir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// 元数据已建。
	if _, err := os.Stat(filepath.Join(workdir, ".taskboard", "meta.db")); err != nil {
		t.Fatalf("meta.db 未创建: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workdir, ".taskboard", "config.yaml")); err != nil {
		t.Fatalf("config.yaml 未创建: %v", err)
	}
	// 但未注册。
	projects, err := svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Fatalf("Init 不应注册项目: %+v", projects)
	}
	// 幂等：二次 Init 不报错。
	if err := svc.Init(context.Background(), workdir); err != nil {
		t.Fatalf("二次 Init 应幂等: %v", err)
	}
}

// TestImportExisting_RequiresInitialized（QA P4-1 Q6：project_import 仅导入）。
func TestImportExisting_RequiresInitialized(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	// 未初始化目录 → 报错。
	raw := t.TempDir()
	if _, err := svc.ImportExisting(ctx, raw); err == nil {
		t.Fatal("未初始化目录 ImportExisting 应报错")
	}

	// 先 Init 再 ImportExisting → 成功注册（幂等）。
	workdir := t.TempDir()
	if err := svc.Init(ctx, workdir); err != nil {
		t.Fatal(err)
	}
	p, err := svc.ImportExisting(ctx, workdir)
	if err != nil {
		t.Fatalf("ImportExisting: %v", err)
	}
	if p.Workdir != workdir {
		t.Fatalf("workdir: %s", p.Workdir)
	}
	if _, err := svc.ImportExisting(ctx, workdir); err != nil {
		t.Fatalf("重复 ImportExisting 应幂等: %v", err)
	}
}

// TestCreate_InitThenImport（QA P4-1 Q6：project_create 语义）。
func TestCreate_InitThenImport(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	workdir := t.TempDir()

	p, err := svc.Create(ctx, workdir)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Workdir != workdir {
		t.Fatalf("workdir: %s", p.Workdir)
	}
	projects, err := svc.List(ctx)
	if err != nil || len(projects) != 1 {
		t.Fatalf("Create 后应注册 1 个项目: %v %d", err, len(projects))
	}
	if _, err := os.Stat(filepath.Join(workdir, ".taskboard", "meta.db")); err != nil {
		t.Fatalf("meta.db 未创建: %v", err)
	}
	if _, err := db.EnsureProject(ctx, db.MetaDBPath(workdir)); err != nil {
		t.Fatal(err)
	}
}
