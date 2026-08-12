// Package parser 负责 Markdown → 结构化任务的 LLM 解析（TF-018 导入草稿流）。
//
// 职责边界（REQUIREMENTS.md §3 / TASK-SEMANTICS.md §17，QA P4-1）：
//   - 解析全部由 LLM 语义理解完成（不依赖固定语法/正则）；
//   - 结果先入 import_drafts 草稿（不直接入库），用户确认后按 source_file 文件级全量覆盖入库；
//   - 缺 title / 缺 status / JSON 不合规 / LLM 超时 → 整次失败不落库（禁止补默认值），
//     返回错误 + LLM 原始输出供人工排查；
//   - 本包只负责 LLM 解析与草稿表管理；确认入库复用 task.Service.ImportTasks（事务原子）。
//
// 分层铁律（AGENTS.md §3.2）：本包为业务层，禁止引用 api / mcp / cmd。
package parser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"tangoforge/internal/config"
	"tangoforge/internal/knowledge"
)

// ParseResult LLM 解析结果（规范化后，作为草稿 parsed_json 持久化）。
type ParseResult struct {
	// Tasks 嵌套结构（Markdown 标题层级 → children），确认时递归展平。
	Tasks []ParsedTask `json:"tasks"`
	// KnowledgeFiles LLM 建议的应引用知识库文件（TF-049，QA-K11）。
	// path 必填；kb 为库名（可省略 = 默认库；不存在 → 整次导入失败 KNOWLEDGE_INVALID）。
	KnowledgeFiles []knowledge.KnowledgeFile `json:"knowledge_files,omitempty"`
}

// KnowledgeInput 导入入参的 knowledge 节（TF-049，QA-K11 扩展草稿流）。
type KnowledgeInput struct {
	// FilePaths 候选知识库文档（合并解析，字典序）。
	FilePaths []string `json:"file_paths"`
	// Directory 候选知识库目录（递归扫描 *.md/*.markdown/*.txt 等文本，字典序）。
	Directory string `json:"directory"`
	// KBID 目标库 id（可选；不传 = 由 LLM 输出的 kb 名决定）。
	KBID int64 `json:"kb_id"`
	// Copy 拷贝语义：none / copy / auto（QA-K2）。
	Copy string `json:"copy"`
}

// ParsedTask 单个解析任务（字段语义对齐 task.Task 的子集）。
type ParsedTask struct {
	// ID 草稿内临时唯一编号（LLM 生成，如 T1/T2；缺失/重复由 normalize 自动补齐修正）。
	// 草稿依赖 depends_on 通过该 ID 引用，与任务标题解耦（后续修改任务标题不影响依赖关系）。
	ID          string       `json:"id"`
	Number      string       `json:"number"` // 简短任务编号（TF-040）：文档自带编号（如 P0），无则空 → 入库自动分配
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Status      string       `json:"status"`   // 已映射为项目状态机 key（§17.2）
	Priority    int          `json:"priority"` // 已归一化 0-5
	Tags        []string     `json:"tags"`     // 去重去空保序
	Assignee    string       `json:"assignee"`
	DependsOn   []string     `json:"depends_on"` // 临时 ID 引用（确认时映射为任务 ID，§17.3）
	Children    []ParsedTask `json:"children,omitempty"`
}

// LLM 输出原始结构（严格 JSON Schema 约束，§17.1）。
type rawParseOutput struct {
	Tasks          []rawTask                 `json:"tasks"`
	KnowledgeFiles []knowledge.KnowledgeFile `json:"knowledge_files"`
}

type rawTask struct {
	ID          string    `json:"id"`     // 临时唯一编号（LLM 生成，如 T1/T2；可选，缺失自动补）
	Number      string    `json:"number"` // 简短任务编号（TF-040）：文档自带编号（如标题前 P0），无则空
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      any       `json:"status"` // 状态机 key 或 label
	Priority    any       `json:"priority"`
	Tags        []string  `json:"tags"`
	Assignee    string    `json:"assignee"`
	DependsOn   []string  `json:"depends_on"` // 被依赖任务的临时 ID
	Children    []rawTask `json:"children"`
}

// buildSystemPrompt 构造 LLM 系统提示（角色 + 规则）。
func buildSystemPrompt() string {
	return `你是 TangoForge 的任务解析器。用户会提供一份 Markdown 任务文档，
你需要将其中的任务语义化解析为结构化 JSON。

规则：
1. 只输出 JSON 本身，禁止输出解释、Markdown 代码围栏或任何多余文本。
2. 每个任务必须有 title（标题）与 status（状态）字段；无法确定的 status 也必须从文档上下文推断。
3. 层级：Markdown 标题层级映射为嵌套 children（## 为顶层，其下 ### 为子任务）。
4. status 只能输出给定的状态 key 或其 label（label 会由系统自动映射）；不得自定义状态。
5. priority 输出 0-5 整数或字符串别名（lowest/low/normal/high/highest/critical/urgent，以及 P0~P5——P0=5 最高、P5=0 最低，大小写不敏感）。
6. 每个任务必须输出一个唯一的 id（建议 T1、T2、T3… 形式，简短、全局唯一；子任务也在同一编号体系内递增）。
7. depends_on 输出被依赖任务的【id】（必须引用本文档中已经定义过的任务 id），不要输出任务标题或文档序号。
8. number 输出文档中任务自带的简短编号（如标题前缀 "TF-001"、"T01"），与 priority 区分：表示"任务序号/编号"的值进 number，表示"优先级/紧急程度"的值（如 P0/P1/P2、高/中/低）进 priority；文档没有编号则输出空字符串 ""。
9. 保持任务顺序与文档一致。`
}

// buildJSONSchema 构造 JSON Schema 描述（追加进 user 提示，约束 LLM 输出）。
func buildJSONSchema() string {
	return `{
  "type": "object",
  "required": ["tasks"],
  "properties": {
    "tasks": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["title", "status"],
        "properties": {
          "id": {"type": "string", "description": "临时唯一编号（如 T1/T2，仅草稿内依赖引用）"},
          "number": {"type": "string", "description": "简短任务编号：文档自带编号（如 TF-001/T01），无则空字符串"},
          "title": {"type": "string"},
          "description": {"type": "string"},
          "status": {"type": "string"},
          "priority": {"type": ["integer", "string"], "description": "0-5 整数或别名（lowest/low/normal/high/highest/critical/urgent/P0~P5，P0=5 最高）"},
          "tags": {"type": "array", "items": {"type": "string"}},
          "assignee": {"type": "string"},
          "depends_on": {"type": "array", "items": {"type": "string"}, "description": "被依赖任务的临时 id"},
          "children": {"type": "array", "items": {"$ref": "#"}}
        }
      }
    }
  }
}`
}

// buildUserPrompt 构造用户提示：状态机 states 注入 + 候选知识库树 + 待解析文档。
func buildUserPrompt(sm config.StateMachine, content, knowledgeTree string) string {
	var b strings.Builder
	b.WriteString("项目状态机状态列表（仅可使用这些状态）：\n")
	for _, st := range sm.States {
		line := "- " + st.Key
		if st.Label != "" {
			line += "（label: " + st.Label + "）"
		}
		b.WriteString(line + "\n")
	}
	if knowledgeTree != "" {
		b.WriteString("\n候选知识库文件（路径 + 类型 + 摘要；请结合任务内容判断哪些应引用入库，\n")
		b.WriteString("在输出中给出 knowledge_files，path 用候选清单中的路径，kb 用库名或省略）：\n")
		b.WriteString(knowledgeTree)
	}
	b.WriteString("\n请解析以下 Markdown 任务文档：\n\n")
	b.WriteString(content)
	return b.String()
}

// scanTextFiles 递归扫描目录下的文本文件（*.md/*.markdown/*.txt 等），字典序。
func scanTextFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".md", ".markdown", ".txt", ".text":
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// buildKnowledgeTree 构建候选知识库文件树（QA-K4：路径/层级/大小/类型 + 摘要缓存，不拼全文）。
// 目录递归扫描文本文件；单文件列表原样保留；每文件附摘要（读取内容 → 生成摘要 ≤200 字，
// 失败降级为仅文件名）。
func (s *Service) buildKnowledgeTree(workdir string, k *KnowledgeInput) (string, error) {
	var files []string
	switch {
	case k.Directory != "":
		dir := filepath.Clean(k.Directory)
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(workdir, dir)
		}
		fs, err := scanTextFiles(dir)
		if err != nil {
			return "", fmt.Errorf("扫描知识库目录 %s: %v", dir, err)
		}
		files = fs
	case len(k.FilePaths) > 0:
		files = make([]string, 0, len(k.FilePaths))
		for _, f := range k.FilePaths {
			p := filepath.Clean(f)
			if !filepath.IsAbs(p) {
				p = filepath.Join(workdir, p)
			}
			files = append(files, p)
		}
	default:
		return "", nil
	}

	var b strings.Builder
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue // 不可达文件跳过（QA-K17：仅警告不阻断）
		}
		rel := f
		if r, err := filepath.Rel(workdir, f); err == nil && !strings.HasPrefix(r, "..") {
			rel = filepath.ToSlash(r)
		}
		typ := "text"
		if !isTextExt(f) {
			typ = "binary"
		}
		summary := ""
		if typ == "text" {
			summary = s.knowledgeSummary(f, info.Size())
		}
		line := fmt.Sprintf("- %s（%s, %d B, %s）", rel, typ, info.Size(), info.ModTime().Format("2006-01-02"))
		if summary != "" {
			line += " 摘要: " + summary
		}
		b.WriteString(line + "\n")
	}
	return b.String(), nil
}

// knowledgeSummary 读取文件并生成摘要（≤200 字；LLM 不可用/失败 → 空串降级）。
func (s *Service) knowledgeSummary(path string, size int64) string {
	if size > 1<<20 { // 1MB 以上文件不做摘要（防爆 token）
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	// 二进制内容（含 NUL）→ 不摘要。
	if strings.ContainsRune(string(data[:min(len(data), 512)]), '\x00') {
		return ""
	}
	text := string(data)
	if len(text) > 20000 {
		text = text[:20000]
	}
	if s.llmClient != nil {
		return knowledge.GenerateSummary(context.Background(), s.llmClient, text)
	}
	return ""
}

// isTextExt 判断扩展名是否为文本（与 knowledge 包类型判定一致的简化版）。
func isTextExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".txt", ".text", ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs",
		".yaml", ".yml", ".json", ".toml", ".html", ".css", ".sql", ".sh", ".bash", ".c", ".h",
		".cpp", ".hpp", ".java", ".rb", ".php", ".vue", ".svelte", ".proto", ".mod", ".sum",
		".tmpl", ".env", ".gitignore", ".lock", ".log", ".diff", ".patch", ".properties",
		".gradle", ".swift":
		return true
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".pdf", ".doc", ".docx",
		".xls", ".xlsx", ".ppt", ".pptx", ".zip", ".tar", ".gz", ".bz2", ".7z", ".rar",
		".exe", ".dll", ".so", ".dylib", ".bin", ".mp3", ".mp4", ".mov", ".wav":
		return false
	}
	return true // 未知扩展名按文本
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// normalizeTags 去重 + 去空串 + 保序（与 task 包语义一致）。
func normalizeTags(tags []string) []string {
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}

// mapStatus 将 LLM 输出状态映射为状态机 key（key 或 label 匹配，大小写不敏感）。
func mapStatus(sm config.StateMachine, raw any) (string, bool) {
	str, ok := raw.(string)
	if !ok {
		return "", false
	}
	str = strings.TrimSpace(str)
	lower := strings.ToLower(str)
	for _, st := range sm.States {
		if strings.ToLower(st.Key) == lower {
			return st.Key, true
		}
		if strings.ToLower(st.Label) == lower {
			return st.Key, true
		}
	}
	return "", false
}

// flatten 递归展平嵌套任务：生成 UUID、parent_id 与 section 路径。
// 返回展平后的任务列表（含待解析的 depends_on 临时 ID 引用）。
type flattenResult struct {
	RefID       string   `json:"ref_id"` // 草稿内临时唯一 ID（LLM 编号，依赖解析主索引）
	ID          string   `json:"id"`
	Number      string   `json:"number"` // 简短任务编号（TF-040）：文档编号或空 → 入库自动分配
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Priority    int      `json:"priority"`
	Tags        []string `json:"tags"`
	Assignee    string   `json:"assignee"`
	DependsOn   []string `json:"depends_on"`
	ParentID    *string  `json:"parent_id,omitempty"`
	Section     string   `json:"section"`
}

// resolveDependsOn 将 depends_on 引用解析为任务 ID：
// 优先按草稿内临时 ID 匹配（LLM 解析新格式）；未命中再按标题匹配（兼容旧草稿标题引用）。
// 引用支持 Markdown 锚点链接 `[标题](#锚点)`（TF-040 导出格式）——先提取链接文本再匹配。
// 无法解析的引用（如被依赖任务标题已被修改、旧草稿标题引用失效）**跳过并计数**——
// 确认导入不因坏引用整次失败（宽容降级），由返回的 dropped 数量提示用户（草稿中间态可交互修复）。
// ID/标题不唯一仍视为结构性错误。
func resolveDependsOn(flattened []flattenResult) (map[string][]string, int, error) {
	idIndex := make(map[string]string)    // 临时 ID → UUID
	titleIndex := make(map[string]string) // 标题(trim) → UUID
	for _, f := range flattened {
		if f.RefID != "" {
			if prev, ok := idIndex[f.RefID]; ok && prev != f.ID {
				return nil, 0, fmt.Errorf("草稿任务 ID 不唯一: %q", f.RefID)
			}
			idIndex[f.RefID] = f.ID
		}
		t := strings.TrimSpace(f.Title)
		if prev, ok := titleIndex[t]; ok && prev != f.ID {
			return nil, 0, fmt.Errorf("依赖标题不唯一: %q", t)
		}
		titleIndex[t] = f.ID
	}
	out := make(map[string][]string, len(flattened))
	dropped := 0
	for _, f := range flattened {
		var ids []string
		for _, dep := range f.DependsOn {
			ref := extractLinkText(strings.TrimSpace(dep))
			id, ok := idIndex[ref]
			if !ok {
				id, ok = titleIndex[ref]
			}
			if !ok {
				// 无法解析（标题被修改/引用失效）：跳过并计数，导入仍继续。
				dropped++
				continue
			}
			ids = append(ids, id)
		}
		out[f.ID] = ids
	}
	return out, dropped, nil
}

// extractLinkText 从依赖引用中提取可匹配文本（TF-040）：
//   - `[标题](#锚点)`（导出锚点链接格式）→ 链接文本「标题」；
//   - 纯文本（临时 ID / 旧标题引用）→ 原样返回。
func extractLinkText(ref string) string {
	if strings.HasPrefix(ref, "[") {
		if end := strings.Index(ref, "]("); end > 0 {
			return strings.TrimSpace(ref[1:end])
		}
	}
	return ref
}
