package skill

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Host 一个 Agent 宿主的安装位置定义（QA-S1 多宿主 v1）。
type Host struct {
	// Key 唯一标识（API / MCP / CLI 均用此值）。
	Key string
	// Label 中文显示名（前端矩阵表头）。
	Label string
	// Scope project（项目级，路径基于 workdir）/ user（用户级，路径基于 homeDir）。
	Scope string
	// Kind marker（单文件标记段，多包共存）/ file（每包一文件）/ dir（每包一目录复制）。
	Kind string
	// PathFn 返回该宿主对某技能包的落盘路径。
	// 参数：workdir（项目级宿主）/ homeDir（用户级宿主）/ name（技能包名）。
	PathFn func(workdir, homeDir, name string) string
}

// 宿主 Scope / Kind 取值。
const (
	ScopeProject = "project"
	ScopeUser    = "user"
	KindMarker   = "marker"
	KindFile     = "file"
	KindDir      = "dir"
)

// Hosts v1 宿主矩阵（QA-S1 用户确认：4 项目级 + 2 用户级）。
var Hosts = []Host{
	{
		Key: "AGENTS.md", Label: "AGENTS.md（CodeBuddy/通用）", Scope: ScopeProject, Kind: KindMarker,
		PathFn: func(workdir, _, _ string) string {
			return filepath.Join(workdir, "AGENTS.md")
		},
	},
	{
		Key: "CLAUDE.md", Label: "CLAUDE.md（Claude Code）", Scope: ScopeProject, Kind: KindMarker,
		PathFn: func(workdir, _, _ string) string {
			return filepath.Join(workdir, "CLAUDE.md")
		},
	},
	{
		Key: ".cursor/rules", Label: ".cursor/rules（Cursor）", Scope: ScopeProject, Kind: KindFile,
		PathFn: func(workdir, _, name string) string {
			return filepath.Join(workdir, ".cursor", "rules", "tangoforge-"+sanitizeName(name)+".mdc")
		},
	},
	{
		Key: "copilot", Label: ".github/copilot-instructions.md（Copilot）", Scope: ScopeProject, Kind: KindMarker,
		PathFn: func(workdir, _, _ string) string {
			return filepath.Join(workdir, ".github", "copilot-instructions.md")
		},
	},
	{
		Key: "user-claude", Label: "~/.claude/skills（Claude 全局）", Scope: ScopeUser, Kind: KindDir,
		PathFn: func(_, homeDir, name string) string {
			return filepath.Join(homeDir, ".claude", "skills", sanitizeName(name), "SKILL.md")
		},
	},
	{
		Key: "user-codebuddy", Label: "~/.workbuddy/skills（WorkBuddy 全局）", Scope: ScopeUser, Kind: KindDir,
		PathFn: func(_, homeDir, name string) string {
			return filepath.Join(homeDir, ".workbuddy", "skills", sanitizeName(name), "SKILL.md")
		},
	},
}

// HostByKey 按 key 查找宿主定义。
func HostByKey(key string) (Host, bool) {
	for _, h := range Hosts {
		if h.Key == key {
			return h, true
		}
	}
	return Host{}, false
}

// sanitizeName 技能包名 → 文件/目录名（仅保留 [A-Za-z0-9._-]，防路径注入）。
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	out := b.String()
	if out == "" || out == "." || out == ".." {
		return "skill"
	}
	return out
}

// 标记段（单文件宿主多包共存，QA-S1）：HTML 注释包裹，可识别/可撤销。
const (
	markerBeginTmpl = "<!-- tangoforge:skill:%s:begin -->\n"
	markerEndTmpl   = "<!-- tangoforge:skill:%s:end -->"
)

// InstallResult 单个技能包的安装/卸载结果。
type InstallResult struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Action  string `json:"action"` // install / update / uninstall
	Version string `json:"version"`
	Ok      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

// InstalledSkill 某宿主下单个技能包的安装状态。
type InstalledSkill struct {
	Name    string `json:"name"`
	Version string `json:"version"` // 宿主中已装版本（未装为空）
	State   string `json:"state"`   // missing / current / stale
}

// HostStatus 某宿主的安装状态（含库内全部包的比对结果）。
type HostStatus struct {
	Key       string           `json:"key"`
	Label     string           `json:"label"`
	Scope     string           `json:"scope"`
	Installed []InstalledSkill `json:"installed"`
}

// 状态取值。
const (
	StateMissing = "missing"
	StateCurrent = "current"
	StateStale   = "stale"
)

// Install 将指定技能包安装/更新到目标宿主（QA-S6）。
//   - 包不存在 → ErrSkillNotFound；宿主未知 → ErrUnknownHost；
//   - 包 frontmatter hosts 声明非空且不含目标宿主 → 拒绝（提示不适配）；
//   - marker 宿主：替换同名标记段或追加；file 宿主：覆盖写 .mdc；dir 宿主：写 <name>/SKILL.md。
//
// 返回每个包的安装结果（install=新装 / update=覆盖旧版）。
func (s *Service) Install(ctx context.Context, workdir, hostKey string, names []string) ([]InstallResult, error) {
	host, ok := HostByKey(hostKey)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownHost, hostKey)
	}
	out := make([]InstallResult, 0, len(names))
	for _, name := range names {
		pkg, err := s.Info(ctx, name)
		if err != nil {
			out = append(out, InstallResult{Name: name, Host: hostKey, Action: "install", Error: err.Error()})
			continue
		}
		if len(pkg.Hosts) > 0 && !containsStr(pkg.Hosts, hostKey) {
			out = append(out, InstallResult{
				Name: name, Host: hostKey, Action: "install",
				Error: fmt.Sprintf("技能包 %s 不适配宿主 %s（声明 hosts: %v）", name, hostKey, pkg.Hosts),
			})
			continue
		}
		path := host.PathFn(workdir, s.homeDir, name)
		action := "install"
		// 已存在 → 更新语义（状态比对在 API 层由前端决策，此处幂等覆盖）。
		if _, statErr := os.Stat(path); statErr == nil {
			action = "update"
		}
		if err := writeToHost(host, path, pkg); err != nil {
			out = append(out, InstallResult{Name: name, Host: hostKey, Action: action, Error: err.Error()})
			continue
		}
		s.logger.Info("skill installed", "host", hostKey, "name", name, "path", path)
		out = append(out, InstallResult{Name: name, Host: hostKey, Action: action, Version: pkg.Version, Ok: true})
	}
	return out, nil
}

// Uninstall 从目标宿主移除指定技能包（QA-S6，前端二次确认后调用）。
func (s *Service) Uninstall(ctx context.Context, workdir, hostKey string, names []string) ([]InstallResult, error) {
	host, ok := HostByKey(hostKey)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownHost, hostKey)
	}
	out := make([]InstallResult, 0, len(names))
	for _, name := range names {
		path := host.PathFn(workdir, s.homeDir, name)
		var err error
		switch host.Kind {
		case KindMarker:
			err = removeMarkerSegment(path, name)
		case KindFile:
			err = os.Remove(path)
			if err != nil && errors.Is(err, os.ErrNotExist) {
				err = nil // 未安装视为成功（幂等）。
			}
		case KindDir:
			err = os.RemoveAll(filepath.Dir(path))
		}
		if err != nil {
			out = append(out, InstallResult{Name: name, Host: hostKey, Action: "uninstall", Error: err.Error()})
			continue
		}
		s.logger.Info("skill uninstalled", "host", hostKey, "name", name)
		out = append(out, InstallResult{Name: name, Host: hostKey, Action: "uninstall", Ok: true})
	}
	return out, nil
}

// Status 检查宿主矩阵中各宿主的安装状态（QA-S6：实时扫描，无缓存）。
// 对每个宿主返回库内全部包的状态（missing/current/stale，按 version 比对）。
func (s *Service) Status(ctx context.Context, workdir string) ([]HostStatus, error) {
	packages, err := s.ListPackages(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]HostStatus, 0, len(Hosts))
	for _, host := range Hosts {
		hs := HostStatus{Key: host.Key, Label: host.Label, Scope: host.Scope,
			Installed: make([]InstalledSkill, 0, len(packages))}
		for _, pkg := range packages {
			installed := InstalledSkill{Name: pkg.Name, State: StateMissing}
			if ver, ok := readInstalledVersion(host, workdir, s.homeDir, pkg.Name); ok {
				installed.Version = ver
				if ver == pkg.Version {
					installed.State = StateCurrent
				} else {
					installed.State = StateStale
				}
			}
			hs.Installed = append(hs.Installed, installed)
		}
		sort.Slice(hs.Installed, func(i, j int) bool { return hs.Installed[i].Name < hs.Installed[j].Name })
		out = append(out, hs)
	}
	return out, nil
}

// writeToHost 按宿主 Kind 写技能包内容。
func writeToHost(host Host, path string, pkg Package) error {
	switch host.Kind {
	case KindMarker:
		return upsertMarkerSegment(path, pkg)
	case KindFile:
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("skill: mkdir %s: %w", filepath.Dir(path), err)
		}
		return os.WriteFile(path, []byte(pkg.Content), 0o644)
	case KindDir:
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("skill: mkdir %s: %w", filepath.Dir(path), err)
		}
		return os.WriteFile(path, []byte(pkg.Content), 0o644)
	default:
		return fmt.Errorf("skill: unknown host kind %q", host.Kind)
	}
}

// upsertMarkerSegment 在单文件宿主的标记段中写入/更新技能包内容（追加式，保留宿主其他内容）。
func upsertMarkerSegment(path string, pkg Package) error {
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("skill: read %s: %w", path, err)
	}
	text := string(existing)
	segment := fmt.Sprintf(markerBeginTmpl, pkg.Name) + pkg.Content + "\n" + fmt.Sprintf(markerEndTmpl, pkg.Name)

	// 按技能包名精确匹配 begin/end（避免误匹配其他技能包段落）。
	beginRe := regexp.MustCompile(`(?ms)^<!--\s*tangoforge:skill:` + regexp.QuoteMeta(pkg.Name) + `:begin\s*-->\s*$`)
	endRe := regexp.MustCompile(`(?ms)^<!--\s*tangoforge:skill:` + regexp.QuoteMeta(pkg.Name) + `:end\s*-->`)
	begin := beginRe.FindStringIndex(text)
	end := endRe.FindStringIndex(text)
	if begin != nil && end != nil && begin[0] < end[1] {
		// 替换整段（保留段外内容）。
		text = text[:begin[0]] + segment + text[end[1]:]
	} else {
		// 追加到末尾（文件可能无尾部换行，先补）。
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "\n" + segment + "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("skill: mkdir %s: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

// removeMarkerSegment 从单文件宿主移除指定技能包标记段；文件空后删除文件本身。
func removeMarkerSegment(path, name string) error {
	existing, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("skill: read %s: %w", path, err)
	}
	text := string(existing)
	beginRe := regexp.MustCompile(`(?ms)^<!--\s*tangoforge:skill:` + regexp.QuoteMeta(name) + `:begin\s*-->\s*$`)
	endRe := regexp.MustCompile(`(?ms)^<!--\s*tangoforge:skill:` + regexp.QuoteMeta(name) + `:end\s*-->`)
	begin := beginRe.FindStringIndex(text)
	end := endRe.FindStringIndex(text)
	if begin == nil || end == nil || begin[0] > end[1] {
		return nil // 未安装，幂等。
	}
	removed := text[:begin[0]] + text[end[1]:]
	removed = strings.TrimLeft(removed, "\n")
	if strings.TrimSpace(removed) == "" {
		// 宿主文件仅含本技能段 → 删除文件。
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("skill: remove %s: %w", path, err)
		}
		return nil
	}
	return os.WriteFile(path, []byte(removed), 0o644)
}

// readInstalledVersion 读取宿主中已安装技能包的版本（不存在返回 ok=false）。
func readInstalledVersion(host Host, workdir, homeDir, name string) (string, bool) {
	path := host.PathFn(workdir, homeDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	switch host.Kind {
	case KindMarker:
		// 从标记段内提取 SKILL.md 内容 → frontmatter version。
		re := regexp.MustCompile(`(?ms)<!--\s*tangoforge:skill:` + regexp.QuoteMeta(name) + `:begin\s*-->\s*(.*?)\s*<!--\s*tangoforge:skill:` + regexp.QuoteMeta(name) + `:end\s*-->`)
		m := re.FindStringSubmatch(string(data))
		if m == nil {
			return "", false
		}
		if pkg, ok := parseSKILLMD(m[1]); ok {
			return pkg.Version, true
		}
		return "", false
	case KindFile, KindDir:
		if pkg, ok := parseSKILLMD(string(data)); ok {
			return pkg.Version, true
		}
		return "", false
	default:
		return "", false
	}
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
