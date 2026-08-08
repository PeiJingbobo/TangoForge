package skill

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestService 构造 Skill 服务（临时 homeDir 隔离全局技能库与用户级宿主）。
func newTestService(t *testing.T) (*Service, string) {
	t.Helper()
	home := t.TempDir()
	return NewService(nil, home), home
}

const sampleSkill = `---
name: taskboard-basic
description: 任务操作指引
version: "1.0.0"
hosts: [.claude/skills, .cursor/skills]
when_to_use: 需要管理任务时
---
# 正文

使用 task_read 工具完成操作。
`

// --- 解析 ---

func TestParseSKILLMD(t *testing.T) {
	pkg, ok := parseSKILLMD(sampleSkill)
	if !ok {
		t.Fatal("解析失败")
	}
	if pkg.Name != "taskboard-basic" || pkg.Version != "1.0.0" ||
		pkg.Description != "任务操作指引" || pkg.WhenToUse != "需要管理任务时" {
		t.Fatalf("字段不符: %+v", pkg)
	}
	if len(pkg.Hosts) != 2 || pkg.Hosts[0] != ".claude/skills" || pkg.Hosts[1] != ".cursor/skills" {
		t.Fatalf("hosts 不符: %+v", pkg.Hosts)
	}
	if !strings.Contains(pkg.Instructions, "task_read") {
		t.Fatalf("正文不符: %q", pkg.Instructions)
	}
	if pkg.Content != sampleSkill {
		t.Fatalf("content 应保留原文")
	}
}

func TestParseSKILLMD_Invalid(t *testing.T) {
	cases := []string{
		"无 frontmatter 的纯文本",
		"---\nno-name: x\n---\n正文\n",
		"---\nname: \n---\n",
		"---\nunclosed",
	}
	for _, c := range cases {
		if _, ok := parseSKILLMD(c); ok {
			t.Fatalf("应解析失败: %q", c)
		}
	}
}

// --- 内置包 / 全局库 ---

func TestListPackages_Builtin(t *testing.T) {
	svc, _ := newTestService(t)
	defer func() { _ = svc.Close() }()
	pkgs, err := svc.ListPackages(context.Background())
	if err != nil {
		t.Fatalf("ListPackages: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "taskboard-basic" {
		t.Fatalf("内置包不符: %+v", pkgs)
	}
	if pkgs[0].Source != SourceBuiltin || pkgs[0].Version == "" {
		t.Fatalf("内置包字段: %+v", pkgs[0])
	}
}

func TestWriteUserPackage_OverridesBuiltin(t *testing.T) {
	svc, _ := newTestService(t)
	defer func() { _ = svc.Close() }()
	ctx := context.Background()

	// 写自定义包。
	content := strings.Replace(sampleSkill, "taskboard-basic", "my-skill", 1)
	pkg, err := svc.WriteUserPackage(ctx, "my-skill", content)
	if err != nil {
		t.Fatalf("WriteUserPackage: %v", err)
	}
	if pkg.Source != SourceUser || pkg.Name != "my-skill" {
		t.Fatalf("写入结果: %+v", pkg)
	}
	// 文件已落盘。
	path := filepath.Join(svc.GlobalSkillsDir(), "my-skill", "SKILL.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("文件缺失: %v", err)
	}

	// 同名覆盖内置（编辑语义）：写 taskboard-basic → Info 返回 user 源。
	override := strings.Replace(sampleSkill, "version: \"1.0.0\"", "version: \"2.0.0\"", 1)
	if _, err := svc.WriteUserPackage(ctx, "taskboard-basic", override); err != nil {
		t.Fatalf("override: %v", err)
	}
	info, err := svc.Info(ctx, "taskboard-basic")
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Source != SourceUser || info.Version != "2.0.0" {
		t.Fatalf("覆盖后应为 user/2.0.0: %+v", info)
	}
	// 列表应无重复（同名合并）。
	pkgs, _ := svc.ListPackages(ctx)
	names := map[string]bool{}
	for _, p := range pkgs {
		names[p.Name] = true
	}
	if len(pkgs) != 2 || !names["taskboard-basic"] || !names["my-skill"] {
		t.Fatalf("列表去重不符: %+v", pkgs)
	}
}

func TestWriteUserPackage_Invalid(t *testing.T) {
	svc, _ := newTestService(t)
	defer func() { _ = svc.Close() }()
	ctx := context.Background()

	// frontmatter name 与路径不一致 → 拒绝。
	if _, err := svc.WriteUserPackage(ctx, "path-name", sampleSkill); !errors.Is(err, ErrInvalidPackage) {
		t.Fatalf("应拒绝 name 不一致: %v", err)
	}
	// 无 frontmatter → 拒绝。
	if _, err := svc.WriteUserPackage(ctx, "x", "纯文本"); !errors.Is(err, ErrInvalidPackage) {
		t.Fatalf("应拒绝无 frontmatter: %v", err)
	}
}

func TestInfo_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	defer func() { _ = svc.Close() }()
	if _, err := svc.Info(context.Background(), "no-such"); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("期望 ErrSkillNotFound, got %v", err)
	}
}

func TestDefaultTemplate(t *testing.T) {
	svc, _ := newTestService(t)
	defer func() { _ = svc.Close() }()
	tmpl, err := svc.DefaultTemplate(context.Background())
	if err != nil {
		t.Fatalf("DefaultTemplate: %v", err)
	}
	if !strings.Contains(tmpl, "name:") {
		t.Fatalf("模板应含 frontmatter: %q", tmpl)
	}
}

// --- 宿主安装 / 卸载 / 状态 ---

// 宿主矩阵回归保护（TF-042）：全部为目录型 .xxx/skills，禁止单文件 .md 宿主。
func TestHosts_AllDirNoMd(t *testing.T) {
	if len(Hosts) != 5 {
		t.Fatalf("宿主矩阵应为 5 个（3 项目级 + 2 用户级），got %d: %+v", len(Hosts), Hosts)
	}
	for _, h := range Hosts {
		if h.Kind != KindDir {
			t.Fatalf("宿主 %s 应为目录型 KindDir，got %s", h.Key, h.Kind)
		}
		if strings.HasSuffix(h.Key, ".md") || strings.Contains(h.Key, ".md") {
			t.Fatalf("宿主 %s 不得为 .md 单文件形式", h.Key)
		}
		// 目录型路径必须以 /<name>/SKILL.md 结尾（整包目录语义）。
		p := h.PathFn("/work", "/home", "pkg-a")
		if !strings.HasSuffix(p, "/pkg-a/SKILL.md") {
			t.Fatalf("宿主 %s 路径应为目录形态 <根>/pkg-a/SKILL.md，got %s", h.Key, p)
		}
	}
}

func TestInstallDirHost(t *testing.T) {
	svc, home := newTestService(t)
	defer func() { _ = svc.Close() }()
	ctx := context.Background()
	workdir := t.TempDir()

	// 安装到 .claude/skills（目录型）：{workdir}/.claude/skills/<name>/SKILL.md。
	results, err := svc.Install(ctx, workdir, ".claude/skills", []string{"taskboard-basic"})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(results) != 1 || !results[0].Ok || results[0].Action != "install" {
		t.Fatalf("结果: %+v", results)
	}
	skillPath := filepath.Join(workdir, ".claude", "skills", "taskboard-basic", "SKILL.md")
	data, _ := os.ReadFile(skillPath)
	if !strings.Contains(string(data), "name: taskboard-basic") {
		t.Fatalf("SKILL.md 内容缺失:\n%s", string(data))
	}

	// 再装一个自定义包 → 多包共存（同宿主不同包目录）。
	myContent := strings.Replace(sampleSkill, "taskboard-basic", "my-skill", 1)
	_, _ = svc.WriteUserPackage(ctx, "my-skill", myContent)
	if _, err := svc.Install(ctx, workdir, ".claude/skills", []string{"my-skill"}); err != nil {
		t.Fatalf("install second: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workdir, ".claude", "skills", "my-skill", "SKILL.md")); err != nil {
		t.Fatalf("第二包目录缺失: %v", err)
	}

	// 重复安装 → update 语义（覆盖写同一文件）。
	results, _ = svc.Install(ctx, workdir, ".claude/skills", []string{"taskboard-basic"})
	if results[0].Action != "update" {
		t.Fatalf("重复安装应为 update: %+v", results[0])
	}

	// 状态 → current。
	status, err := svc.Status(ctx, workdir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	found := false
	for _, hs := range status {
		if hs.Key == ".claude/skills" {
			for _, inst := range hs.Installed {
				if inst.Name == "taskboard-basic" && inst.State == StateCurrent {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf(".claude/skills 应有 current 状态: %+v", status)
	}

	// 卸载 taskboard-basic → 目录删除；my-skill 保留。
	results, err = svc.Uninstall(ctx, workdir, ".claude/skills", []string{"taskboard-basic"})
	if err != nil || !results[0].Ok {
		t.Fatalf("Uninstall: %v %+v", err, results)
	}
	if _, err := os.Stat(filepath.Dir(skillPath)); !os.IsNotExist(err) {
		t.Fatalf("卸载后包目录应删除: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workdir, ".claude", "skills", "my-skill", "SKILL.md")); err != nil {
		t.Fatalf("my-skill 被误删: %v", err)
	}
	_ = home
}

func TestInstallAllDirHosts(t *testing.T) {
	svc, home := newTestService(t)
	defer func() { _ = svc.Close() }()
	ctx := context.Background()
	workdir := t.TempDir()

	// 遍历全部 5 个宿主：安装 → 包目录存在 → 卸载 → 目录删除。
	for _, h := range Hosts {
		if _, err := svc.Install(ctx, workdir, h.Key, []string{"taskboard-basic"}); err != nil {
			t.Fatalf("install %s: %v", h.Key, err)
		}
		path := h.PathFn(workdir, home, "taskboard-basic")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s 安装文件缺失 %s: %v", h.Key, path, err)
		}
		if _, err := svc.Uninstall(ctx, workdir, h.Key, []string{"taskboard-basic"}); err != nil {
			t.Fatalf("uninstall %s: %v", h.Key, err)
		}
		if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
			t.Fatalf("%s 卸载后包目录应删除: %v", h.Key, err)
		}
	}
}

func TestInstall_UnknownHostAndHostsFilter(t *testing.T) {
	svc, _ := newTestService(t)
	defer func() { _ = svc.Close() }()
	ctx := context.Background()
	workdir := t.TempDir()

	// 未知宿主 → ErrUnknownHost。
	if _, err := svc.Install(ctx, workdir, "no-such", []string{"taskboard-basic"}); !errors.Is(err, ErrUnknownHost) {
		t.Fatalf("期望 ErrUnknownHost, got %v", err)
	}

	// 自定义包声明 hosts 仅 [.claude/skills]：安装到 user-claude 应拒绝（逐包结果 Error）。
	limited := "---\nname: limited-skill\nversion: \"1.0.0\"\nhosts: [.claude/skills]\n---\n# Limited\n"
	if _, err := svc.WriteUserPackage(ctx, "limited-skill", limited); err != nil {
		t.Fatalf("WriteUserPackage: %v", err)
	}
	results, err := svc.Install(ctx, workdir, "user-claude", []string{"limited-skill"})
	if err != nil {
		t.Fatalf("Install 不应整体失败（逐包结果）: %v", err)
	}
	if results[0].Ok || !strings.Contains(results[0].Error, "不适配") {
		t.Fatalf("应拒绝不适配宿主: %+v", results[0])
	}

	// 不存在包 → 结果含错误。
	results, _ = svc.Install(ctx, workdir, ".claude/skills", []string{"no-such"})
	if results[0].Ok {
		t.Fatalf("不存在包应失败: %+v", results[0])
	}
}

func TestStatus_StaleAfterUserEdit(t *testing.T) {
	svc, _ := newTestService(t)
	defer func() { _ = svc.Close() }()
	ctx := context.Background()
	workdir := t.TempDir()

	// 安装内置 v1.0.0 → current。
	_, _ = svc.Install(ctx, workdir, ".claude/skills", []string{"taskboard-basic"})
	// 用户自定义覆盖为 2.0.0 → stale（宿主仍 1.0.0）。
	override := strings.Replace(sampleSkill, "version: \"1.0.0\"", "version: \"2.0.0\"", 1)
	if _, err := svc.WriteUserPackage(ctx, "taskboard-basic", override); err != nil {
		t.Fatalf("override: %v", err)
	}
	status, err := svc.Status(ctx, workdir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	for _, hs := range status {
		if hs.Key == ".claude/skills" {
			for _, inst := range hs.Installed {
				if inst.Name == "taskboard-basic" && inst.State != StateStale {
					t.Fatalf("应为 stale: %+v", inst)
				}
			}
		}
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"taskboard-basic": "taskboard-basic",
		"a/b/c":           "a-b-c",
		"..":              "skill",
		"":                "skill",
		"中文包名":            "----",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Fatalf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}
