// Package skill 负责 AI Skill 技能包的获取、编辑与宿主安装管理（TF-033 重设计）。
//
// 职责边界（docs/task/SKILLS-REDESIGN.md，用户确认 QA-S1~S9）：
//   - 彻底废弃 .taskboard/skills/ 文件扫描与 skills 表缓存（v3 迁移 drop）；
//   - 技能包来源：内置 embed（packages/）+ 全局技能库（{home}/.taskboard-app/skills/<name>/），
//     无项目库依赖、无状态（安装状态实时扫描宿主位置）；
//   - SKILL.md 格式（Anthropic Agent Skills 规范靠拢）：YAML frontmatter
//     （name/description/version/hosts/when_to_use）+ 正文 instructions；
//   - 安装 = 把 SKILL.md（及可选资源）复制到宿主约定位置（目录型 .xxx/skills：
//     .claude/skills / .cursor/skills / .github/skills / ~/.claude/skills / ~/.workbuddy/skills），
//     建立可发现性；全部宿主均为目录形态（<宿主根>/<name>/SKILL.md），整包卸载。
//   - 状态检测：missing / current / stale（按 version 比对，实时扫描）。
//
// 分层铁律（AGENTS.md §3.2）：本包为业务层，禁止引用 api / mcp / cmd。
package skill

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrSkillNotFound 表示指定名称的 Skill 不存在（内置 + 全局库均无）。
var ErrSkillNotFound = errors.New("skill: not found")

// ErrInvalidPackage 表示 SKILL.md 内容无效（frontmatter 缺失 / name 缺失或不一致）。
var ErrInvalidPackage = errors.New("skill: invalid package")

// ErrUnknownHost 表示宿主 key 不在 v1 宿主矩阵内。
var ErrUnknownHost = errors.New("skill: unknown host")

// Package 单个 Skill 技能包（SKILL.md 解析结果）。
type Package struct {
	// Name 唯一标识（frontmatter name）。
	Name string `json:"name"`
	// Version 版本号（安装状态比对依据）。
	Version string `json:"version"`
	// Description 一句话描述（AI 选择技能用）。
	Description string `json:"description"`
	// Hosts 适用宿主 key 列表（frontmatter hosts，空 = 全部）。
	Hosts []string `json:"hosts"`
	// WhenToUse 触发场景（frontmatter when_to_use）。
	WhenToUse string `json:"when_to_use"`
	// Instructions 正文（frontmatter 之后的内容，给 Agent 的操作指引）。
	Instructions string `json:"instructions"`
	// Content SKILL.md 原文（安装/编辑回显用）。
	Content string `json:"content"`
	// Source 来源：builtin（内置 embed）/ user（全局技能库）。
	Source string `json:"source"`
	// UpdatedAt 全局库文件修改时间（RFC3339）；内置包为空。
	UpdatedAt string `json:"updated_at"`
}

// Source 取值。
const (
	SourceBuiltin = "builtin"
	SourceUser    = "user"
)

// TemplateDirName 全局技能库默认模板目录名（全局设置页编辑；QA-S4）。
const TemplateDirName = "_template"

// Service Skill 业务服务：内置包 embed + 全局技能库读写 + 宿主安装/卸载/状态。
type Service struct {
	logger *slog.Logger
	// homeDir 用户主目录（全局技能库 {home}/.taskboard-app/skills 与宿主用户级位置的基础）。
	homeDir string
}

// NewService 构造 Skill 服务。
// homeDir 为 os.UserHomeDir() 结果（测试可注入临时目录）。
func NewService(logger *slog.Logger, homeDir string) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	return &Service{logger: logger, homeDir: homeDir}
}

// HomeDir 返回服务持有的用户主目录（API 层组装 Deps 用）。
func (s *Service) HomeDir() string { return s.homeDir }

// Close 兼容旧接口：本服务无连接资源，空实现。
func (s *Service) Close() error { return nil }

// GlobalSkillsDir 返回全局技能库目录（{home}/.taskboard-app/skills）。
func (s *Service) GlobalSkillsDir() string {
	return filepath.Join(s.homeDir, ".taskboard-app", "skills")
}

// TemplatePath 返回全局默认模板文件路径（QA-S4：{home}/.taskboard-app/skills/_template/SKILL.md）。
func (s *Service) TemplatePath() string {
	return filepath.Join(s.GlobalSkillsDir(), TemplateDirName, "SKILL.md")
}

// ListPackages 返回全部技能包（内置 + 全局库，按名称升序）。
// 同名时全局库覆盖内置（用户可自定义编辑当前项目的 Skill，QA G5）。
func (s *Service) ListPackages(ctx context.Context) ([]Package, error) {
	builtin, err := s.listBuiltin()
	if err != nil {
		return nil, err
	}
	user, err := s.listUser(ctx)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]Package, len(builtin)+len(user))
	for _, p := range builtin {
		merged[p.Name] = p
	}
	for _, p := range user {
		merged[p.Name] = p // 覆盖内置同名包。
	}
	out := make([]Package, 0, len(merged))
	for _, p := range merged {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Info 返回单个技能包详情（全局库优先，内置兜底）；不存在 → ErrSkillNotFound。
func (s *Service) Info(ctx context.Context, name string) (Package, error) {
	if p, ok, err := s.readUserPackage(ctx, name); err != nil {
		return Package{}, err
	} else if ok {
		return p, nil
	}
	if p, ok, err := s.readBuiltinPackage(name); err != nil {
		return Package{}, err
	} else if ok {
		return p, nil
	}
	return Package{}, fmt.Errorf("%w: %s", ErrSkillNotFound, name)
}

// WriteUserPackage 将自定义 SKILL.md 写入全局技能库 {home}/.taskboard-app/skills/<name>/SKILL.md。
// 校验 frontmatter（name 必须存在且与参数一致）；返回写入后的包。
func (s *Service) WriteUserPackage(_ context.Context, name, content string) (Package, error) {
	pkg, ok := parseSKILLMD(content)
	if !ok {
		return Package{}, fmt.Errorf("%w: frontmatter 缺失或 name 为空", ErrInvalidPackage)
	}
	if pkg.Name != name {
		return Package{}, fmt.Errorf("%w: frontmatter name %q 与路径 %q 不一致", ErrInvalidPackage, pkg.Name, name)
	}
	dir := filepath.Join(s.GlobalSkillsDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Package{}, fmt.Errorf("skill: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return Package{}, fmt.Errorf("skill: write %s: %w", path, err)
	}
	pkg.Source = SourceUser
	pkg.UpdatedAt = fileModTime(path)
	return pkg, nil
}

// DefaultTemplate 返回全局默认模板内容；不存在时返回内置模板（_template 兜底）。
func (s *Service) DefaultTemplate(_ context.Context) (string, error) {
	path := s.TemplatePath()
	if data, err := os.ReadFile(path); err == nil {
		return string(data), nil
	}
	// 内置兜底模板。
	data, err := builtinFS.ReadFile("packages/taskboard-basic/SKILL.md")
	if err != nil {
		return "", fmt.Errorf("skill: read builtin template: %w", err)
	}
	return string(data), nil
}

// WriteTemplate 将自定义默认模板写入 {home}/.taskboard-app/skills/_template/SKILL.md。
// 模板允许占位符（name/description 等），不做 frontmatter 强校验（区别于普通包）。
func (s *Service) WriteTemplate(_ context.Context, content string) error {
	dir := filepath.Join(s.GlobalSkillsDir(), TemplateDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("skill: mkdir template: %w", err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("skill: write template %s: %w", path, err)
	}
	s.logger.Info("skill template updated", "path", path)
	return nil
}

// --- 内部：内置包 ---

func (s *Service) listBuiltin() ([]Package, error) {
	entries, err := builtinFS.ReadDir("packages")
	if err != nil {
		return nil, fmt.Errorf("skill: read builtin packages: %w", err)
	}
	out := make([]Package, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, ok, err := s.readBuiltinPackage(e.Name())
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *Service) readBuiltinPackage(name string) (Package, bool, error) {
	data, err := builtinFS.ReadFile(filepath.Join("packages", name, "SKILL.md"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Package{}, false, nil
		}
		return Package{}, false, fmt.Errorf("skill: read builtin %s: %w", name, err)
	}
	p, ok := parseSKILLMD(string(data))
	if !ok {
		return Package{}, false, nil
	}
	p.Source = SourceBuiltin
	return p, true, nil
}

// --- 内部：全局技能库 ---

func (s *Service) listUser(ctx context.Context) ([]Package, error) {
	dir := s.GlobalSkillsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("skill: read user skills dir %s: %w", dir, err)
	}
	out := make([]Package, 0)
	for _, e := range entries {
		if !e.IsDir() || e.Name() == TemplateDirName {
			continue
		}
		if p, ok, err := s.readUserPackage(ctx, e.Name()); err != nil {
			return nil, err
		} else if ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *Service) readUserPackage(_ context.Context, name string) (Package, bool, error) {
	path := filepath.Join(s.GlobalSkillsDir(), name, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Package{}, false, nil
		}
		return Package{}, false, fmt.Errorf("skill: read user package %s: %w", name, err)
	}
	p, ok := parseSKILLMD(string(data))
	if !ok {
		return Package{}, false, nil
	}
	p.Source = SourceUser
	p.UpdatedAt = fileModTime(path)
	return p, true, nil
}

// --- 内部：SKILL.md 解析 ---

// parseSKILLMD 解析 SKILL.md：YAML frontmatter（--- 分隔）+ 正文。
// frontmatter 必须含 name；version/description/hosts/when_to_use 可选。
func parseSKILLMD(content string) (Package, bool) {
	fm, body, ok := splitFrontmatter(content)
	if !ok {
		return Package{}, false
	}
	name := strings.TrimSpace(yamlScalar(fm, "name"))
	if name == "" {
		return Package{}, false
	}
	return Package{
		Name:         name,
		Version:      strings.TrimSpace(yamlScalar(fm, "version")),
		Description:  strings.TrimSpace(yamlScalar(fm, "description")),
		Hosts:        yamlStringList(fm, "hosts"),
		WhenToUse:    strings.TrimSpace(yamlScalar(fm, "when_to_use")),
		Instructions: strings.TrimSpace(body),
		Content:      content,
	}, true
}

// splitFrontmatter 拆分 SKILL.md：首行 --- 与第二个 --- 之间为 frontmatter，其后为正文。
func splitFrontmatter(content string) (fm, body string, ok bool) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n"), true
		}
	}
	return "", "", false
}

// yamlScalar 从 frontmatter 文本提取标量字段值（极简 YAML 解析：`key: value` 行）。
// 支持 `key: value` 与 `key: "value"`；值含冒号时取整行冒号后部分。
func yamlScalar(fm, key string) string {
	for _, line := range strings.Split(fm, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		kv := strings.SplitN(trimmed, ":", 2)
		if len(kv) != 2 || strings.TrimSpace(kv[0]) != key {
			continue
		}
		val := strings.TrimSpace(kv[1])
		val = strings.Trim(val, `"'`)
		return val
	}
	return ""
}

// yamlStringList 从 frontmatter 提取字符串数组字段（`key: [a, b]` 行内数组）。
func yamlStringList(fm, key string) []string {
	for _, line := range strings.Split(fm, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, key+":") {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(trimmed, key+":"))
		val = strings.Trim(val, "[]")
		if val == "" {
			return nil
		}
		parts := strings.Split(val, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			v := strings.Trim(strings.TrimSpace(p), `"'`)
			if v != "" {
				out = append(out, v)
			}
		}
		return out
	}
	return nil
}

// fileModTime 返回文件修改时间 RFC3339；失败返回空串。
func fileModTime(path string) string {
	if info, err := os.Stat(path); err == nil {
		return info.ModTime().Format("2006-01-02T15:04:05-07:00")
	}
	return ""
}
