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
hosts: [AGENTS.md, CLAUDE.md]
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
	if len(pkg.Hosts) != 2 || pkg.Hosts[0] != "AGENTS.md" || pkg.Hosts[1] != "CLAUDE.md" {
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

func TestInstallMarkerHost(t *testing.T) {
	svc, home := newTestService(t)
	defer func() { _ = svc.Close() }()
	ctx := context.Background()
	workdir := t.TempDir()

	// 安装到 AGENTS.md（marker）。
	results, err := svc.Install(ctx, workdir, "AGENTS.md", []string{"taskboard-basic"})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(results) != 1 || !results[0].Ok || results[0].Action != "install" {
		t.Fatalf("结果: %+v", results)
	}
	agentsPath := filepath.Join(workdir, "AGENTS.md")
	data, _ := os.ReadFile(agentsPath)
	if !strings.Contains(string(data), "tangoforge:skill:taskboard-basic:begin") {
		t.Fatalf("标记段缺失:\n%s", string(data))
	}

	// 再装一个自定义包 → 多包共存。
	myContent := strings.Replace(sampleSkill, "taskboard-basic", "my-skill", 1)
	_, _ = svc.WriteUserPackage(ctx, "my-skill", myContent)
	if _, err := svc.Install(ctx, workdir, "AGENTS.md", []string{"my-skill"}); err != nil {
		t.Fatalf("install second: %v", err)
	}
	data, _ = os.ReadFile(agentsPath)
	if !strings.Contains(string(data), "tangoforge:skill:taskboard-basic:begin") ||
		!strings.Contains(string(data), "tangoforge:skill:my-skill:begin") {
		t.Fatalf("多包共存失败:\n%s", string(data))
	}

	// 重复安装 → update 语义 + 不产生重复段。
	results, _ = svc.Install(ctx, workdir, "AGENTS.md", []string{"taskboard-basic"})
	if results[0].Action != "update" {
		t.Fatalf("重复安装应为 update: %+v", results[0])
	}
	data, _ = os.ReadFile(agentsPath)
	if strings.Count(string(data), "tangoforge:skill:taskboard-basic:begin") != 1 {
		t.Fatalf("重复段出现:\n%s", string(data))
	}

	// 状态 → current。
	status, err := svc.Status(ctx, workdir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	found := false
	for _, hs := range status {
		if hs.Key == "AGENTS.md" {
			for _, inst := range hs.Installed {
				if inst.Name == "taskboard-basic" && inst.State == StateCurrent {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("AGENTS.md 应有 current 状态: %+v", status)
	}

	// 卸载 taskboard-basic → 仅 my-skill 保留。
	results, err = svc.Uninstall(ctx, workdir, "AGENTS.md", []string{"taskboard-basic"})
	if err != nil || !results[0].Ok {
		t.Fatalf("Uninstall: %v %+v", err, results)
	}
	data, _ = os.ReadFile(agentsPath)
	if strings.Contains(string(data), "tangoforge:skill:taskboard-basic") {
		t.Fatalf("卸载后 taskboard-basic 段残留:\n%s", string(data))
	}
	if !strings.Contains(string(data), "tangoforge:skill:my-skill:begin") {
		t.Fatalf("my-skill 段被误删:\n%s", string(data))
	}
	_ = home
}

func TestInstallDirAndFileHost(t *testing.T) {
	svc, home := newTestService(t)
	defer func() { _ = svc.Close() }()
	ctx := context.Background()
	workdir := t.TempDir()

	// dir 宿主 user-claude：{home}/.claude/skills/<name>/SKILL.md。
	if _, err := svc.Install(ctx, workdir, "user-claude", []string{"taskboard-basic"}); err != nil {
		t.Fatalf("install user-claude: %v", err)
	}
	claudePath := filepath.Join(home, ".claude", "skills", "taskboard-basic", "SKILL.md")
	if _, err := os.Stat(claudePath); err != nil {
		t.Fatalf("user-claude 安装文件缺失: %v", err)
	}

	// file 宿主 .cursor/rules：{workdir}/.cursor/rules/tangoforge-<name>.mdc。
	if _, err := svc.Install(ctx, workdir, ".cursor/rules", []string{"taskboard-basic"}); err != nil {
		t.Fatalf("install cursor: %v", err)
	}
	cursorPath := filepath.Join(workdir, ".cursor", "rules", "tangoforge-taskboard-basic.mdc")
	if _, err := os.Stat(cursorPath); err != nil {
		t.Fatalf("cursor 安装文件缺失: %v", err)
	}

	// 卸载 dir 宿主 → 删除目录。
	if _, err := svc.Uninstall(ctx, workdir, "user-claude", []string{"taskboard-basic"}); err != nil {
		t.Fatalf("uninstall user-claude: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(claudePath)); !os.IsNotExist(err) {
		t.Fatalf("dir 宿主卸载后目录应删除: %v", err)
	}

	// 卸载 file 宿主 → 删除文件。
	if _, err := svc.Uninstall(ctx, workdir, ".cursor/rules", []string{"taskboard-basic"}); err != nil {
		t.Fatalf("uninstall cursor: %v", err)
	}
	if _, err := os.Stat(cursorPath); !os.IsNotExist(err) {
		t.Fatalf("file 宿主卸载后文件应删除: %v", err)
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

	// 自定义包声明 hosts 仅 [AGENTS.md]：安装到 user-claude 应拒绝（逐包结果 Error）。
	limited := "---\nname: limited-skill\nversion: \"1.0.0\"\nhosts: [AGENTS.md]\n---\n# Limited\n"
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
	results, _ = svc.Install(ctx, workdir, "AGENTS.md", []string{"no-such"})
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
	_, _ = svc.Install(ctx, workdir, "AGENTS.md", []string{"taskboard-basic"})
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
		if hs.Key == "AGENTS.md" {
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
