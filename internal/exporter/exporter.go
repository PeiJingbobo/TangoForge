// Package exporter 负责从结构化任务数据重建 Markdown（TF-019，QA P4-1）。
//
// 职责边界（REQUIREMENTS.md §4 / TASK-SEMANTICS.md §18）：
//   - 默认模板渲染（内置 default.tmpl，Front Matter + 层级标题 + 状态/优先级/标签/负责人元数据行，
//     可被 TF-018 parser 重新导入，往返一致）；
//   - 项目配置 export.template_path 自定义模板覆盖（Go text/template，禁止引入其他模板语言）；
//   - template_mode：default（配置模板或内置默认）/ llm（项目已生成的模板）；
//   - target：overwrite（写指定 path）/ copy（写 path 或默认 {workdir}/.taskboard/export.md）；
//   - LLM 生成模板：示例文档 → LLM 输出 Go text/template → template.Parse 校验 →
//     写 {workdir}/.taskboard/generated-template.tmpl → 更新项目 config.yaml export.template_path。
//
// 分层铁律（AGENTS.md §3.2）：本包为业务层，禁止引用 api / mcp / cmd。
package exporter

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"tangoforge/internal/config"
	"tangoforge/internal/knowledge"
	"tangoforge/internal/llm"
	"tangoforge/internal/task"
	"text/template"
	"time"
)

// 业务错误码（docs/TASK-SEMANTICS.md §18.4）。
const (
	CodeExportFailed    = "EXPORT_FAILED"
	CodeTemplateInvalid = "TEMPLATE_INVALID"
)

// ErrExportFailed 渲染/写盘失败。
var ErrExportFailed = errors.New("exporter: export failed")

// ErrTemplateInvalid 模板非法（LLM 生成物未通过 template.Parse 或模板缺失）。
var ErrTemplateInvalid = errors.New("exporter: template invalid")

// ErrProjectNotFound 项目未导入。
var ErrProjectNotFound = errors.New("exporter: project not found")

// defaultTmpl 内置默认模板（go:embed）。
//
//go:embed templates/default.tmpl
var defaultTmpl string

// Options Service 构造选项。
type Options struct {
	Logger *slog.Logger
	// Tasks 任务业务服务（导出取任务树）。
	Tasks task.Service
	// LLM 配置提供者（LLM 生成模板用；每次调用取最新支持热重载）。
	LLM func() config.LLMConfig
	// Knowledge 知识库业务服务（TF-049：任务关联文档路径输出；nil = 不输出资料行）。
	Knowledge KnowledgeLister
	// OnExport 导出完成事件（export.complete，audit + WS 由调用方注入）。
	OnExport func(ctx context.Context, workdir, action, target string)
}

// KnowledgeLister 知识库只读接口（exporter 查询任务关联文档用）。
type KnowledgeLister interface {
	// TaskDocuments 返回任务关联的文档（RelPath/AbsPath 供资料行输出）。
	TaskDocuments(ctx context.Context, workdir, taskID string) ([]knowledge.Document, error)
}

// KnowledgeDoc 知识库文档路径信息（导出资料行用，rel_path 优先）。
type KnowledgeDoc struct {
	RelPath string `json:"rel_path"`
	AbsPath string `json:"abs_path"`
}

// knowledgeDocPaths 提取文档路径列表（rel_path 优先，缺省 abs_path）。
func knowledgeDocPaths(docs []knowledge.Document) []string {
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		p := d.RelPath
		if p == "" {
			p = d.AbsPath
		}
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Service 导出业务服务。
type Service struct {
	logger    *slog.Logger
	tasks     task.Service
	llmCfg    func() config.LLMConfig
	knowledge KnowledgeLister
	onExport  func(ctx context.Context, workdir, action, target string)
}

// NewService 构造导出服务。
func NewService(opts Options) *Service {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.LLM == nil {
		opts.LLM = func() config.LLMConfig { return config.DefaultLLMConfig() }
	}
	return &Service{
		logger: opts.Logger, tasks: opts.Tasks, llmCfg: opts.LLM,
		knowledge: opts.Knowledge, onExport: opts.OnExport,
	}
}

// RenderOptions 导出参数（QA P4-1 §18.2）。
type RenderOptions struct {
	TemplateMode string `json:"template_mode"` // default | llm
	Target       string `json:"target"`        // overwrite | copy
	Path         string `json:"path"`
}

// RenderResult 导出结果（服务端写盘 + 响应带 content 供预览）。
type RenderResult struct {
	Content string `json:"content"`
	Path    string `json:"path"`
}

// FlatTask 展平任务（渲染用，带层级）。
type FlatTask struct {
	task.Task
	Level int `json:"-"`
	// DepTitles 依赖任务的标题（与 DependsOn 一一对应；TF-039 可读性：
	// 导出显示标题而非 UUID，parser 导入按标题解析，往返一致）。
	DepTitles []string `json:"dep_titles,omitempty"`
	// Docs 任务关联的知识库文档路径列表（TF-049，QA-K17/K19；渲染 `- 资料:` 行）。
	Docs []string `json:"docs,omitempty"`
}

// Render 从任务库渲染 Markdown 并写盘（QA P4-1 §18.2）。
func (s *Service) Render(ctx context.Context, workdir string, opts RenderOptions) (RenderResult, error) {
	if err := validateWorkdir(workdir); err != nil {
		return RenderResult{}, err
	}
	mode := opts.TemplateMode
	if mode == "" {
		mode = "default"
	}
	target := opts.Target
	if target == "" {
		target = "copy"
	}
	if mode != "default" && mode != "llm" {
		return RenderResult{}, fmt.Errorf("%w: 未知 template_mode %q", ErrExportFailed, mode)
	}
	if target != "overwrite" && target != "copy" {
		return RenderResult{}, fmt.Errorf("%w: 未知 target %q", ErrExportFailed, target)
	}

	// 1. 任务树（默认排除 archived）。
	list, err := s.tasks.List(ctx, workdir, task.ListFilter{})
	if err != nil {
		return RenderResult{}, err
	}
	flat := make([]FlatTask, 0)
	flattenTree(list.Tree, 0, &flat)
	// TF-039：依赖 ID → 标题映射（parser 导入按标题解析，往返一致且可读）。
	resolveDepTitles(flat)
	// TF-049：任务关联知识库文档路径（资料行；knowledge 未接入 → 空）。
	if s.knowledge != nil {
		for i := range flat {
			docs, err := s.knowledge.TaskDocuments(ctx, workdir, flat[i].ID)
			if err != nil {
				// 关联查询失败不阻断导出（资料行为可选信息）。
				continue
			}
			flat[i].Docs = knowledgeDocPaths(docs)
		}
	}

	// 2. 模板选择与渲染。
	tmplText, err := s.loadTemplate(workdir, mode)
	if err != nil {
		return RenderResult{}, err
	}
	projectName := filepath.Base(workdir)
	data := struct {
		Project     struct{ Name string }
		GeneratedAt string
		Tasks       []FlatTask
	}{
		Project:     struct{ Name string }{Name: projectName},
		GeneratedAt: time.Now().Format(time.RFC3339),
		Tasks:       flat,
	}
	tpl, err := template.New("export").Funcs(templateFuncs()).Parse(tmplText)
	if err != nil {
		return RenderResult{}, fmt.Errorf("%w: %v", ErrTemplateInvalid, err)
	}
	var b strings.Builder
	if err := tpl.Execute(&b, data); err != nil {
		return RenderResult{}, fmt.Errorf("%w: 渲染失败: %v", ErrExportFailed, err)
	}
	content := b.String()

	// 3. 写盘。
	path, err := resolveTargetPath(workdir, target, opts.Path)
	if err != nil {
		return RenderResult{}, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return RenderResult{}, fmt.Errorf("%w: 写入 %s: %v", ErrExportFailed, path, err)
	}

	s.logger.Info("markdown exported", "workdir", workdir, "mode", mode, "target", target, "path", path, "tasks", len(flat))
	if s.onExport != nil {
		s.onExport(ctx, workdir, "export.complete", path)
	}
	return RenderResult{Content: content, Path: path}, nil
}

// GenerateTemplate 依据示例文档由 LLM 生成导出模板（QA P4-1 §18.3）。
//
// LLM 输出必须通过 Go text/template 校验（非法 → TEMPLATE_INVALID，不写盘）；
// 校验通过 → 写 {workdir}/.taskboard/generated-template.tmpl → 更新项目
// config.yaml 的 export.template_path → 返回模板内容。
func (s *Service) GenerateTemplate(ctx context.Context, workdir string, example string) (string, error) {
	if err := validateWorkdir(workdir); err != nil {
		return "", err
	}
	if strings.TrimSpace(example) == "" {
		return "", fmt.Errorf("%w: 示例文档为空", ErrTemplateInvalid)
	}
	client, err := llm.New(llm.FromConfig(s.llmCfg()), s.logger)
	if err != nil {
		return "", err // LLM_NOT_CONFIGURED
	}
	// 客户端断开不取消 LLM 请求（context.WithoutCancel；超时由 llm.Client http.Timeout 控制）。
	tmplText, err := client.Complete(context.WithoutCancel(ctx), llm.Request{
		System: `你是 TangoForge 的 Markdown 导出模板生成器。用户提供一份示例 Markdown 文档，
请生成一份等价的 Go text/template 模板，用于从结构化任务数据渲染出同风格文档。

模板数据：
- .Project.Name（项目名）、.GeneratedAt（时间戳 RFC3339）
- .Tasks：[]FlatTask 展平列表，每项含 Task 全部字段（Title/Description/Status/Priority/Tags/Assignee/CreatedAt/UpdatedAt）与 Level（层级，顶层 0）
- 依赖可读性（重要）：.DependsOn 已格式化为 Markdown 锚点链接数组（如「[子任务](#子任务)」，锚点=标题 slug），渲染依赖时请用 {{join .DependsOn ", "}}（可点击跳转文档内标题）；.DepTitles 为纯标题数组
- 知识库资料（TF-049）：.Docs 为任务关联文档路径数组（可空），有值时输出「- 资料: {{join .Docs ", "}}」行

可用函数（只能使用以下函数，不得发明其他函数名）：
- header level title —— 输出 2+level 个 # 的标题
- join list sep —— 连接字符串数组
- dateFormat time [layout] —— 格式化时间（接受 RFC3339 字符串或时间对象，默认输出 2006-01-02；可用第二参自定义 Go 布局）
- now —— 当前时间 RFC3339
- upper / lower / trim / title s —— 字符串大小写与去空白
- hasPrefix / hasSuffix s prefix —— 前后缀判断

要求：
1. 只输出模板内容本身，禁止解释或代码围栏。
2. 标题层级用 {{header .Level .Title}} 表达（顶层 ##，子任务 ###）。
3. 每任务必须包含"状态: {{.Status}}"元数据行（保证可被重新导入）。
4. 可用 {{range .Tasks}}...{{end}} 遍历。`,
		User: "示例文档：\n\n" + example,
	})
	if err != nil {
		return "", fmt.Errorf("%w: LLM 生成模板失败: %v", ErrTemplateInvalid, err)
	}

	// 校验（使用与渲染一致的 funcs）。
	if _, err := template.New("export").Funcs(templateFuncs()).Parse(tmplText); err != nil {
		return "", fmt.Errorf("%w: 生成的模板非法: %v", ErrTemplateInvalid, err)
	}

	// 写盘 + 更新项目配置。
	path := filepath.Join(workdir, ".taskboard", "generated-template.tmpl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("%w: mkdir: %v", ErrExportFailed, err)
	}
	if err := os.WriteFile(path, []byte(tmplText), 0o644); err != nil {
		return "", fmt.Errorf("%w: 写入模板: %v", ErrExportFailed, err)
	}
	cfg, err := config.LoadProject(workdir)
	if err != nil {
		return "", fmt.Errorf("%w: 读取项目配置: %v", ErrExportFailed, err)
	}
	cfg.Export.TemplatePath = path
	if err := config.SaveProject(workdir, cfg); err != nil {
		return "", fmt.Errorf("%w: 更新项目配置: %v", ErrExportFailed, err)
	}
	s.logger.Info("export template generated", "workdir", workdir, "path", path)
	return tmplText, nil
}

// loadTemplate 按模式加载模板文本。
//   - default：项目配置 export.template_path（非空）→ 文件；空 → 内置 default.tmpl；
//   - llm：export.template_path 必须已生成（否则 TEMPLATE_INVALID）。
func (s *Service) loadTemplate(workdir, mode string) (string, error) {
	cfg, err := config.LoadProject(workdir)
	if err != nil {
		return "", fmt.Errorf("%w: 读取项目配置: %v", ErrExportFailed, err)
	}
	path := strings.TrimSpace(cfg.Export.TemplatePath)
	if mode == "llm" {
		if path == "" {
			return "", fmt.Errorf("%w: 尚未生成 LLM 模板（请先调用 template/generate）", ErrTemplateInvalid)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("%w: 读取模板 %s: %v", ErrTemplateInvalid, path, err)
		}
		return string(data), nil
	}
	if path == "" {
		return defaultTmpl, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%w: 读取自定义模板 %s: %v", ErrTemplateInvalid, path, err)
	}
	return string(data), nil
}

// TemplateContent 返回指定模式的模板文本（导出对话框模板预览用；TF-038）。
// llm 模式未生成 → ErrTemplateInvalid（前端据此自动引导「用 LLM 生成模板」）。
func (s *Service) TemplateContent(_ context.Context, workdir, mode string) (string, error) {
	if err := validateWorkdir(workdir); err != nil {
		return "", err
	}
	if mode == "" {
		mode = "default"
	}
	if mode != "default" && mode != "llm" {
		return "", fmt.Errorf("%w: 未知 template_mode %q", ErrExportFailed, mode)
	}
	return s.loadTemplate(workdir, mode)
}

// flattenTree 深度优先展平任务树（Level 从 0 起）。
func flattenTree(nodes []*task.TaskTreeNode, level int, out *[]FlatTask) {
	for _, n := range nodes {
		*out = append(*out, FlatTask{Task: n.Task, Level: level})
		flattenTree(n.Children, level+1, out)
	}
}

// resolveDepTitles 将每个任务的 DependsOn（UUID 列表）映射为可读引用（TF-039/TF-040）。
//
// 引用格式为 Markdown 锚点链接 `[标题](#锚点)`：
// - 可读：导出文档中依赖直接显示任务标题；
// - 可点击：支持锚点的渲染器（GitHub/Typora/VS Code 等）点击跳转到文档内对应标题；
// - 可往返：parser 解析依赖时提取链接文本（标题）按标题匹配回 UUID。
//
// 关键：**就地覆盖 DependsOn**（同时填充 DepTitles 纯标题）——既有 LLM 生成模板 /
// 自定义模板普遍使用 {{join .DependsOn ", "}} 渲染依赖，就地覆盖后老模板同样生效。
// 依赖任务不在当前导出集合（已归档等）→ 保留原 ID 兜底，不丢失信息。
func resolveDepTitles(flat []FlatTask) {
	titleByID := make(map[string]string, len(flat))
	for _, f := range flat {
		if f.Title != "" {
			titleByID[f.ID] = f.Title
		}
	}
	for i := range flat {
		if len(flat[i].DependsOn) == 0 {
			continue
		}
		links := make([]string, 0, len(flat[i].DependsOn))
		titles := make([]string, 0, len(flat[i].DependsOn))
		for _, id := range flat[i].DependsOn {
			if t, ok := titleByID[id]; ok {
				links = append(links, "["+t+"]("+anchorOf(t)+")")
				titles = append(titles, t)
			} else {
				links = append(links, id) // 依赖不在集合内：原样保留 ID
				titles = append(titles, id)
			}
		}
		flat[i].DepTitles = titles
		flat[i].DependsOn = links // 就地替换：老模板用 .DependsOn 也输出锚点链接
	}
}

// anchorOf 生成标题的 Markdown 锚点（GitHub 风格 slug）：
// 小写化、空格转连字符、移除标题中常见的锚点干扰字符（#、[]() 等）。
// 中文标题保持原样（主流渲染器支持中文锚点）。
func anchorOf(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t':
			b.WriteByte('-')
		case strings.ContainsRune("#[]()<>`&,.!?;:'\"\\/|{}", r):
			// 跳过锚点干扰字符
		default:
			b.WriteRune(r)
		}
	}
	return "#" + b.String()
}

// resolveTargetPath 解析输出路径（QA P4-1 §18.2）。
func resolveTargetPath(workdir, target, path string) (string, error) {
	if target == "overwrite" {
		if strings.TrimSpace(path) == "" {
			return "", fmt.Errorf("%w: overwrite 模式必须提供 path", ErrExportFailed)
		}
		return filepath.Clean(path), nil
	}
	// copy：path 缺省 {workdir}/.taskboard/export-<时间戳>.md——
	// 每次导出独立文件；同秒已存在时追加序号（-2、-3…）确保绝不覆盖（TF-039）。
	if strings.TrimSpace(path) == "" {
		stamp := time.Now().Format("20060102-150405")
		base := filepath.Join(workdir, ".taskboard", "export-"+stamp)
		path = base + ".md"
		for i := 2; ; i++ {
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				break
			}
			path = fmt.Sprintf("%s-%d.md", base, i)
		}
	}
	return filepath.Clean(path), nil
}

// validateWorkdir 校验项目目录（meta.db 存在）。
func validateWorkdir(workdir string) error {
	if _, err := os.Stat(filepath.Join(workdir, ".taskboard", "meta.db")); err != nil {
		return fmt.Errorf("%w: %s", ErrProjectNotFound, workdir)
	}
	return nil
}

// templateFuncs 模板函数集（渲染与 LLM 模板校验共用）。
//
// TF-038 修复：LLM 生成模板使用 dateFormat 时报 "function not defined"——
// 扩充常用函数（时间格式化/字符串处理），并在 LLM prompt 中明确清单，
// 减少 LLM 输出未定义函数导致的 TEMPLATE_INVALID。
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"join": strings.Join,
		"header": func(level int, title string) string {
			return strings.Repeat("#", 2+level) + " " + title
		},
		// dateFormat 格式化时间：接受 time.Time 或 RFC3339 字符串；
		// 可选第二个参数为 Go 时间布局（默认 2006-01-02）。
		"dateFormat": func(v any, layouts ...string) string {
			layout := "2006-01-02"
			if len(layouts) > 0 && layouts[0] != "" {
				layout = layouts[0]
			}
			switch t := v.(type) {
			case time.Time:
				return t.Format(layout)
			case string:
				if t == "" {
					return ""
				}
				parsed, err := time.Parse(time.RFC3339, t)
				if err != nil {
					return t // 非 RFC3339 原样返回（不阻断渲染）
				}
				return parsed.Format(layout)
			default:
				return fmt.Sprintf("%v", v)
			}
		},
		// now 当前时间（RFC3339；模板顶部时间戳用）。
		"now": func() string { return time.Now().Format(time.RFC3339) },
		// 字符串处理（LLM 生成模板常用）。
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"trim":      strings.TrimSpace,
		"title":     func(s string) string { return strings.ToUpper(s[:1]) + s[1:] },
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
	}
}
