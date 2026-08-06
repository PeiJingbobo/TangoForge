package skill

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tangoforge/internal/db"
)

// newTestProject 初始化临时项目：{tmp}/.taskboard/meta.db + skills/ 目录。
func newTestProject(t *testing.T) string {
	t.Helper()
	workdir := t.TempDir()
	metaDir := filepath.Join(workdir, ".taskboard")
	if err := os.MkdirAll(filepath.Join(metaDir, "skills"), 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	conn, err := db.EnsureProject(context.Background(), db.MetaDBPath(workdir))
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	_ = conn.Close()
	return workdir
}

// writeSkill 写入 Skill 文件。
func writeSkill(t *testing.T, workdir, name, content string) {
	t.Helper()
	path := filepath.Join(workdir, ".taskboard", "skills", name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill %s: %v", name, err)
	}
}

func newService() *Service { return NewService(nil) }

// --- 解析 ---

func TestParse_YAML(t *testing.T) {
	data := []byte("name: taskboard-basic\nversion: 1\ndescription: 任务操作指引\ninstructions: |\n  使用 task_read 工具\n")
	sk, ok := parseSkill("basic.yaml", data)
	if !ok {
		t.Fatal("解析失败")
	}
	if sk.Name != "taskboard-basic" || sk.Version != "1" || sk.Description != "任务操作指引" {
		t.Fatalf("字段不符: %+v", sk)
	}
	if !strings.Contains(sk.Instructions, "task_read") {
		t.Fatalf("instructions 不符: %q", sk.Instructions)
	}
	if sk.Content != string(data) {
		t.Fatalf("content 应为原文")
	}
}

func TestParse_YAML_MissingName(t *testing.T) {
	if _, ok := parseSkill("bad.yaml", []byte("version: 1\n")); ok {
		t.Fatal("缺 name 应解析失败")
	}
	if _, ok := parseSkill("bad.yml", []byte("not: [valid")); ok {
		t.Fatal("坏 YAML 应解析失败")
	}
}

func TestParse_Markdown(t *testing.T) {
	md := "# 导入指南\n\n使用 import_preview / import_confirm 完成草稿流。\n"
	sk, ok := parseSkill("import-guide.md", []byte(md))
	if !ok {
		t.Fatal("解析失败")
	}
	if sk.Name != "导入指南" {
		t.Fatalf("name %q", sk.Name)
	}
	if sk.Instructions != md || sk.Content != md {
		t.Fatalf("MD 内容应全文保留")
	}
}

func TestParse_Markdown_NoTitle(t *testing.T) {
	if _, ok := parseSkill("plain.md", []byte("没有标题的正文\n")); ok {
		t.Fatal("无 # 标题应解析失败")
	}
}

// --- 扫描 / 缓存同步 / 查询 ---

func TestList_YAMLAndMarkdown(t *testing.T) {
	workdir := newTestProject(t)
	writeSkill(t, workdir, "basic.yaml", "name: taskboard-basic\nversion: 1\ndescription: 基础指引\ninstructions: 用 task_read\n")
	writeSkill(t, workdir, "guide.md", "# 导入指南\n\n草稿流说明\n")
	// 非支持扩展名与子目录文件应忽略。
	writeSkill(t, workdir, "notes.txt", "ignore me")
	sub := filepath.Join(workdir, ".taskboard", "skills", "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "deep.yaml"), []byte("name: deep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := newService()
	defer func() { _ = svc.Close() }()
	items, err := svc.List(context.Background(), workdir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("应返回 2 个 skill（忽略 txt 与子目录），got %d: %+v", len(items), items)
	}
	if items[0].Name != "taskboard-basic" || items[1].Name != "导入指南" {
		t.Fatalf("排序或名称不符: %+v", items)
	}
}

func TestList_DeletionSyncsCache(t *testing.T) {
	workdir := newTestProject(t)
	writeSkill(t, workdir, "a.yaml", "name: skill-a\n")
	writeSkill(t, workdir, "b.yaml", "name: skill-b\n")

	svc := newService()
	defer func() { _ = svc.Close() }()
	ctx := context.Background()

	items, err := svc.List(ctx, workdir)
	if err != nil || len(items) != 2 {
		t.Fatalf("首扫 %d items, err=%v", len(items), err)
	}

	// 删除文件 → 再次 List 索引同步。
	if err := os.Remove(filepath.Join(workdir, ".taskboard", "skills", "a.yaml")); err != nil {
		t.Fatal(err)
	}
	items, err = svc.List(ctx, workdir)
	if err != nil {
		t.Fatalf("重扫: %v", err)
	}
	if len(items) != 1 || items[0].Name != "skill-b" {
		t.Fatalf("删除后未同步: %+v", items)
	}

	// 修改文件内容 → 内容同步（Instructions 更新）。
	writeSkill(t, workdir, "b.yaml", "name: skill-b\nversion: 2\n")
	items, err = svc.List(ctx, workdir)
	if err != nil || len(items) != 1 {
		t.Fatalf("修改后: %d items err=%v", len(items), err)
	}
	if items[0].Version != "2" {
		t.Fatalf("版本未同步: %+v", items[0])
	}
}

func TestList_ParseFailureSkipped(t *testing.T) {
	workdir := newTestProject(t)
	writeSkill(t, workdir, "bad.yaml", "name: [unclosed")
	writeSkill(t, workdir, "notitle.md", "无标题")
	writeSkill(t, workdir, "good.yaml", "name: good-skill\n")

	svc := newService()
	defer func() { _ = svc.Close() }()
	items, err := svc.List(context.Background(), workdir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Name != "good-skill" {
		t.Fatalf("仅 good-skill 应保留: %+v", items)
	}
}

func TestInfo_NotFound(t *testing.T) {
	workdir := newTestProject(t)
	writeSkill(t, workdir, "a.yaml", "name: skill-a\ninstructions: 指引\n")

	svc := newService()
	defer func() { _ = svc.Close() }()
	ctx := context.Background()

	sk, err := svc.Info(ctx, workdir, "skill-a")
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if sk.Instructions != "指引" || sk.Content == "" {
		t.Fatalf("Info 字段: %+v", sk)
	}
	if _, err := svc.Info(ctx, workdir, "no-such"); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("期望 ErrSkillNotFound, got %v", err)
	}
}

func TestProjectNotFound(t *testing.T) {
	svc := newService()
	defer func() { _ = svc.Close() }()
	if _, err := svc.List(context.Background(), "/no/such/project"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("期望 ErrProjectNotFound, got %v", err)
	}
}

func TestSkillsDirMissing_Empty(t *testing.T) {
	workdir := newTestProject(t)
	// 删除 skills/ 目录模拟异常环境。
	if err := os.RemoveAll(filepath.Join(workdir, ".taskboard", "skills")); err != nil {
		t.Fatal(err)
	}
	svc := newService()
	defer func() { _ = svc.Close() }()
	items, err := svc.List(context.Background(), workdir)
	if err != nil {
		t.Fatalf("目录缺失应视为空: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("应为空列表: %+v", items)
	}
}
