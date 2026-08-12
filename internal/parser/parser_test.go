package parser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"tangoforge/internal/config"
	"tangoforge/internal/db"
	"tangoforge/internal/llm"
	"tangoforge/internal/task"
	"testing"
)

// initParserProject 初始化临时项目（meta.db + 默认 config.yaml，含状态机 label）。
func initParserProject(t *testing.T) string {
	t.Helper()
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".taskboard"), 0o755); err != nil {
		t.Fatal(err)
	}
	conn, err := db.EnsureProject(context.Background(), db.MetaDBPath(workdir))
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if err := config.SaveProject(workdir, config.DefaultProjectConfig()); err != nil {
		t.Fatal(err)
	}
	return workdir
}

// mockLLM 返回固定解析内容的 mock OpenAI 服务。
func mockLLM(t *testing.T, content string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := fmt.Sprintf(`{"choices":[{"message":{"content":%s}}]}`, strconv.Quote(content))
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newParser 构造 parser 服务（mock LLM + 事件收集）。
func newParser(t *testing.T, llmURL string) (*Service, *[]string) {
	t.Helper()
	taskSvc := task.NewService(task.Options{})
	t.Cleanup(func() { _ = taskSvc.Close() })
	events := &[]string{}
	svc := NewService(Options{
		LLM: func() config.LLMConfig {
			return config.LLMConfig{BaseURL: llmURL, Model: "mock", APIKind: "openai", TimeoutSec: 5, Retries: 0}
		},
		Tasks: taskSvc,
		OnEvent: func(_ context.Context, _ string, action, _ string) {
			*events = append(*events, action)
		},
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc, events
}

// nestedDoc LLM 解析输出的嵌套任务（2 顶层 + 1 子任务 + 标题依赖）。
const nestedDoc = `{"tasks":[
  {"title":"发布 v1","status":"doing","priority":"high","tags":["release"],"assignee":"张三",
   "children":[{"title":"写发布说明","status":"todo"}]},
  {"title":"修复登录页","status":"todo","priority":5,"depends_on":["写发布说明"]}
]}`

func TestParse_DraftCreated(t *testing.T) {
	srv := mockLLM(t, nestedDoc)
	svc, events := newParser(t, srv.URL)
	workdir := initParserProject(t)

	draft, err := svc.Parse(context.Background(), workdir, ParseInput{Content: "# 文档\n", SourceFile: "docs/tasks.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if draft.Status != "pending" || draft.TaskCount != 3 || draft.SourceFile != "docs/tasks.md" {
		t.Fatalf("draft: %+v", draft)
	}
	if len(*events) != 1 || (*events)[0] != "import.draft_ready" {
		t.Fatalf("事件: %v", *events)
	}
}

func TestParse_StatusLabelMapping(t *testing.T) {
	// status 用 label（进行中）→ 映射为 doing。
	srv := mockLLM(t, `{"tasks":[{"title":"任务A","status":"进行中"}]}`)
	svc, _ := newParser(t, srv.URL)
	workdir := initParserProject(t)

	draft, err := svc.Parse(context.Background(), workdir, ParseInput{Content: "x", SourceFile: "a.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if draft.TaskCount != 1 {
		t.Fatalf("task_count=%d", draft.TaskCount)
	}
}

func TestParse_PriorityP0Alias(t *testing.T) {
	// LLM 按文档原文输出 "P0" 优先级（项目文档常见写法），应归一化为 5 而非拒绝。
	srv := mockLLM(t, `{"tasks":[{"title":"数据库迁移脚本","status":"done","priority":"P0"}]}`)
	svc, _ := newParser(t, srv.URL)
	workdir := initParserProject(t)

	draft, err := svc.Parse(context.Background(), workdir, ParseInput{Content: "x", SourceFile: "a.md"})
	if err != nil {
		t.Fatalf("Parse(P0): %v", err)
	}
	detail, err := svc.Get(context.Background(), workdir, draft.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(detail.Tasks) != 1 || detail.Tasks[0].Priority != 5 {
		t.Fatalf("P0 应归一化为 5，got %+v", detail.Tasks)
	}
}

func TestParse_Failures(t *testing.T) {
	cases := []struct {
		name    string
		llmBody string
	}{
		{"缺 title", `{"tasks":[{"status":"todo"}]}`},
		{"缺 status", `{"tasks":[{"title":"x"}]}`},
		{"坏 JSON", `{"tasks": [broken`},
		{"status 不在状态机", `{"tasks":[{"title":"x","status":"no-such"}]}`},
		{"空任务", `{"tasks":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := mockLLM(t, tc.llmBody)
			svc, events := newParser(t, srv.URL)
			workdir := initParserProject(t)

			_, err := svc.Parse(context.Background(), workdir, ParseInput{Content: "x", SourceFile: "a.md"})
			if !errors.Is(err, ErrImportFailed) {
				t.Fatalf("期望 IMPORT_FAILED, got %v", err)
			}
			// 失败事件。
			if len(*events) != 1 || (*events)[0] != "import.failed" {
				t.Fatalf("事件: %v", *events)
			}
		})
	}
}

func TestParse_LLMNotConfigured(t *testing.T) {
	taskSvc := task.NewService(task.Options{})
	t.Cleanup(func() { _ = taskSvc.Close() })
	svc := NewService(Options{
		LLM:   func() config.LLMConfig { return config.DefaultLLMConfig() }, // base_url/model 空
		Tasks: taskSvc,
	})
	t.Cleanup(func() { _ = svc.Close() })
	workdir := initParserProject(t)

	_, err := svc.Parse(context.Background(), workdir, ParseInput{Content: "x", SourceFile: "a.md"})
	if !errors.Is(err, llm.ErrNotConfigured) {
		t.Fatalf("期望 LLM_NOT_CONFIGURED, got %v", err)
	}
}

func TestConfirm_FullFlow(t *testing.T) {
	srv := mockLLM(t, nestedDoc)
	svc, events := newParser(t, srv.URL)
	workdir := initParserProject(t)
	ctx := context.Background()

	draft, err := svc.Parse(ctx, workdir, ParseInput{Content: "# 文档\n", SourceFile: "docs/tasks.md"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Confirm(ctx, workdir, draft.ID)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if res.Created != 3 || res.Archived != 0 {
		t.Fatalf("ConfirmResult: %+v", res)
	}
	if (*events)[1] != "import.draft_confirmed" {
		t.Fatalf("事件: %v", *events)
	}

	// 验证任务树：2 顶层 + 1 子任务。
	ts := task.NewService(task.Options{})
	t.Cleanup(func() { _ = ts.Close() })
	list, err := ts.List(ctx, workdir, task.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tree) != 2 {
		t.Fatalf("顶层任务 %d != 2", len(list.Tree))
	}
	// 发布 v1：doing + high→4 + tags + assignee + section。
	v1 := findTask(list.Tree, "发布 v1")
	if v1 == nil {
		t.Fatal("找不到 发布 v1")
	}
	if v1.Status != "doing" || v1.Priority != 4 || len(v1.Tags) != 1 || v1.Tags[0] != "release" || v1.Assignee != "张三" {
		t.Fatalf("发布v1 字段: %+v", v1)
	}
	if v1.SourceFile != "docs/tasks.md" || v1.SourceSection != "1" {
		t.Fatalf("source: %+v", v1)
	}
	if len(v1.Children) != 1 {
		t.Fatalf("子任务数 %d", len(v1.Children))
	}
	child := v1.Children[0]
	if child.Title != "写发布说明" || child.SourceSection != "1.1" {
		t.Fatalf("子任务: %+v", child)
	}
	// 修复登录页：priority 5 + depends_on 标题解析为 写发布说明 ID。
	fix := findTask(list.Tree, "修复登录页")
	if fix == nil || fix.Priority != 5 || len(fix.DependsOn) != 1 || fix.DependsOn[0] != child.ID {
		t.Fatalf("修复登录页: %+v", fix)
	}
}

func TestConfirm_ReimportArchivesOld(t *testing.T) {
	srv := mockLLM(t, nestedDoc)
	svc, _ := newParser(t, srv.URL)
	workdir := initParserProject(t)
	ctx := context.Background()

	// 第一次导入确认。
	d1, err := svc.Parse(ctx, workdir, ParseInput{Content: "v1", SourceFile: "docs/tasks.md"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(ctx, workdir, d1.ID); err != nil {
		t.Fatal(err)
	}
	// 第二次同文件导入（不同内容）。
	srv2 := mockLLM(t, `{"tasks":[{"title":"新版本","status":"todo"}]}`)
	svc2, _ := newParser(t, srv2.URL)
	d2, err := svc2.Parse(ctx, workdir, ParseInput{Content: "v2", SourceFile: "docs/tasks.md"})
	if err != nil {
		t.Fatal(err)
	}
	res2, err := svc2.Confirm(ctx, workdir, d2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Archived != 3 {
		t.Fatalf("旧文件任务应全部归档, Archived=%d", res2.Archived)
	}
}

func TestConfirm_UnknownDraft(t *testing.T) {
	srv := mockLLM(t, nestedDoc)
	svc, _ := newParser(t, srv.URL)
	workdir := initParserProject(t)

	_, err := svc.Confirm(context.Background(), workdir, "no-such-draft")
	if !errors.Is(err, ErrDraftNotFound) {
		t.Fatalf("期望 DRAFT_NOT_FOUND, got %v", err)
	}
}

func TestDiscard_NoTaskChanges(t *testing.T) {
	srv := mockLLM(t, nestedDoc)
	svc, events := newParser(t, srv.URL)
	workdir := initParserProject(t)
	ctx := context.Background()

	draft, err := svc.Parse(ctx, workdir, ParseInput{Content: "x", SourceFile: "a.md"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Discard(ctx, workdir, draft.ID); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if (*events)[1] != "import.draft_discarded" {
		t.Fatalf("事件: %v", *events)
	}

	ts := task.NewService(task.Options{})
	t.Cleanup(func() { _ = ts.Close() })
	list, err := ts.List(ctx, workdir, task.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tree) != 0 {
		t.Fatalf("丢弃后不应有任务: %d", len(list.Tree))
	}
	// 重复丢弃 → 404。
	if err := svc.Discard(ctx, workdir, draft.ID); !errors.Is(err, ErrDraftNotFound) {
		t.Fatalf("二次丢弃应 404, got %v", err)
	}
}

func TestList_PendingOnly(t *testing.T) {
	srv := mockLLM(t, nestedDoc)
	svc, _ := newParser(t, srv.URL)
	workdir := initParserProject(t)
	ctx := context.Background()

	d1, _ := svc.Parse(ctx, workdir, ParseInput{Content: "x", SourceFile: "a.md"})
	d2, _ := svc.Parse(ctx, workdir, ParseInput{Content: "y", SourceFile: "b.md"})
	_ = svc.Discard(ctx, workdir, d1.ID)

	drafts, err := svc.List(ctx, workdir)
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 1 || drafts[0].ID != d2.ID {
		t.Fatalf("pending 列表: %+v", drafts)
	}
}

func TestParse_FilePathMode(t *testing.T) {
	srv := mockLLM(t, nestedDoc)
	svc, _ := newParser(t, srv.URL)
	workdir := initParserProject(t)

	// 相对路径（相对 workdir）+ 绝对路径。
	doc := filepath.Join(workdir, "tasks.md")
	if err := os.WriteFile(doc, []byte("# 任务\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	draft, err := svc.Parse(context.Background(), workdir, ParseInput{FilePath: "tasks.md"})
	if err != nil {
		t.Fatalf("Parse(file_path): %v", err)
	}
	if draft.SourceFile != doc {
		t.Fatalf("source_file 应规范化为绝对路径: %s", draft.SourceFile)
	}
}

// findTask 按标题在树中查找（含子树）。
func findTask(tree []*task.TaskTreeNode, title string) *task.TaskTreeNode {
	for _, n := range tree {
		if n.Title == title {
			return n
		}
		if found := findTask(n.Children, title); found != nil {
			return found
		}
	}
	return nil
}

// flatten 单测：section 路径与 parent_id 正确性。
func TestFlattenTasks(t *testing.T) {
	tasks := []ParsedTask{
		{Title: "A", Status: "todo", Children: []ParsedTask{{Title: "A1", Status: "todo"}, {Title: "A2", Status: "todo"}}},
		{Title: "B", Status: "todo"},
	}
	flat, err := flattenTasks(tasks, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(flat) != 4 {
		t.Fatalf("展平数 %d", len(flat))
	}
	if flat[0].Section != "1" || flat[1].Section != "1.1" || flat[2].Section != "1.2" || flat[3].Section != "2" {
		t.Fatalf("section: %+v", flat)
	}
	if flat[1].ParentID == nil || *flat[1].ParentID != flat[0].ID {
		t.Fatalf("A1 parent: %v", flat[1].ParentID)
	}
	if flat[3].ParentID != nil {
		t.Fatalf("B 应为顶层")
	}
}

// 依赖标题解析：重复标题报错。
func TestResolveDependsOn_DuplicateTitle(t *testing.T) {
	flat := []flattenResult{
		{ID: "a", Title: "重复"},
		{ID: "b", Title: "重复"},
	}
	if _, _, err := resolveDependsOn(flat); err == nil {
		t.Fatal("重复标题应报错")
	}
}

// JSON round-trip：ParseResult 可序列化（草稿 parsed_json 持久化）。
func TestParseResult_JSONRoundTrip(t *testing.T) {
	pr := ParseResult{Tasks: []ParsedTask{{Title: "A", Status: "todo", Tags: []string{"x"}, Children: []ParsedTask{{Title: "A1", Status: "doing"}}}}}
	data, err := json.Marshal(pr)
	if err != nil {
		t.Fatal(err)
	}
	var back ParseResult
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Tasks) != 1 || len(back.Tasks[0].Children) != 1 || back.Tasks[0].Children[0].Status != "doing" {
		t.Fatalf("round-trip: %+v", back)
	}
	if strings.Contains(string(data), "\"children\":null") {
		t.Fatalf("children 不应序列化为 null: %s", data)
	}
}

// TestNormalize_AssignsIDs：LLM 缺 id → 自动补 T{n}；给定 id 保留；重复 id 修正。
func TestNormalize_AssignsIDs(t *testing.T) {
	svc, _ := newParser(t, "http://127.0.0.1:1")
	sm := config.DefaultStateMachine()
	raw := json.RawMessage(`{"tasks":[
	  {"title":"A","status":"todo"},
	  {"title":"B","status":"doing","id":"X9"},
	  {"id":"T1","title":"C","status":"todo"},
	  {"id":"T1","title":"D","status":"todo","children":[{"title":"D1","status":"done"}]}
	]}`)
	pr, err := svc.normalizeOutput(sm, raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	ids := []string{pr.Tasks[0].ID, pr.Tasks[1].ID, pr.Tasks[2].ID, pr.Tasks[3].ID}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			t.Fatalf("ID 缺失或重复: %v", ids)
		}
		seen[id] = true
	}
	// A 无 id → T1；B 保留 X9；C 的 T1 已被占 → T2；D 的 T1 重复 → T3；D1 → T4。
	if ids[0] != "T1" || ids[1] != "X9" || ids[2] != "T2" || ids[3] != "T3" {
		t.Fatalf("分配结果: %v", ids)
	}
	if pr.Tasks[3].Children[0].ID != "T4" {
		t.Fatalf("子任务 ID: %v", pr.Tasks[3].Children[0].ID)
	}
}

// TestResolveDependsOn_ByID：临时 ID 引用优先、标题兜底（旧草稿兼容）、未知引用宽容跳过并计数。
func TestResolveDependsOn_ByID(t *testing.T) {
	flat := []flattenResult{
		{RefID: "T1", ID: "u1", Title: "配置加载与热重载"},
		{RefID: "T2", ID: "u2", Title: "数据库迁移脚本"},
		{RefID: "T3", ID: "u3", Title: "修复登录页"},
	}
	flat[0].DependsOn = []string{"T2"}      // 新格式：临时 ID 引用
	flat[2].DependsOn = []string{"数据库迁移脚本"} // 旧格式：标题引用兜底
	out, dropped, err := resolveDependsOn(flat)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if dropped != 0 {
		t.Fatalf("dropped: %d", dropped)
	}
	if len(out["u1"]) != 1 || out["u1"][0] != "u2" {
		t.Fatalf("ID 引用: %v", out["u1"])
	}
	if len(out["u3"]) != 1 || out["u3"][0] != "u2" {
		t.Fatalf("标题兜底: %v", out["u3"])
	}
	// 未知引用（标题已修改/失效）：跳过并计数，不中断。
	flat[0].DependsOn = []string{"T9", "T2", "已改名的标题"}
	out, dropped, err = resolveDependsOn(flat)
	if err != nil {
		t.Fatalf("宽容模式不应报错: %v", err)
	}
	if dropped != 2 {
		t.Fatalf("dropped = %d, want 2", dropped)
	}
	if len(out["u1"]) != 1 || out["u1"][0] != "u2" {
		t.Fatalf("保留可解析引用: %v", out["u1"])
	}
}

// TestResolveDependsOn_LinkAnchor（TF-040）：导出格式为 Markdown 锚点链接
// `[标题](#锚点)`——解析时提取链接文本（标题）按标题匹配。
func TestResolveDependsOn_LinkAnchor(t *testing.T) {
	flat := []flattenResult{
		{RefID: "T1", ID: "u1", Title: "配置加载与热重载"},
		{RefID: "T2", ID: "u2", Title: "数据库迁移脚本"},
	}
	// 锚点链接引用（TF-040 导出格式）：提取「数据库迁移脚本」按标题匹配。
	flat[0].DependsOn = []string{"[数据库迁移脚本](#数据库迁移脚本)"}
	out, dropped, err := resolveDependsOn(flat)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if dropped != 0 {
		t.Fatalf("dropped: %d", dropped)
	}
	if len(out["u1"]) != 1 || out["u1"][0] != "u2" {
		t.Fatalf("锚点链接引用: %v", out["u1"])
	}
}

// TestExtractLinkText（TF-040）：链接文本提取纯函数。
func TestExtractLinkText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"[子任务](#子任务)", "子任务"},
		{"[付款流程](/docs/付款流程)", "付款流程"},
		{"T2", "T2"}, // 临时 ID 原样
		{"数据库迁移脚本", "数据库迁移脚本"}, // 纯标题原样
		{"[缺失括号]", "[缺失括号]"},
		{"", ""},
	}
	for _, c := range cases {
		if got := extractLinkText(c.in); got != c.want {
			t.Fatalf("extractLinkText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestConfirm_DropsBadDep：旧草稿标题引用在标题修改后失效 → 确认导入成功且 dropped 计数。
func TestConfirm_DropsBadDep(t *testing.T) {
	doc := `{"tasks":[
	  {"id":"T1","title":"配置加载与热重载","status":"doing","depends_on":["数据库迁移脚本"]},
	  {"id":"T2","title":"数据库迁移脚本001","status":"todo"}
	]}`
	srv := mockLLM(t, doc)
	svc, _ := newParser(t, srv.URL)
	workdir := initParserProject(t)
	ctx := context.Background()
	draft, err := svc.Parse(ctx, workdir, ParseInput{Content: "# 文档\n", SourceFile: "docs/bad-dep.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// 模拟用户修改标题后草稿（依赖仍是旧标题引用）。
	updated := []ParsedTask{
		{ID: "T1", Title: "配置加载与热重载", Status: "doing", DependsOn: []string{"数据库迁移脚本"}},
		{ID: "T2", Title: "数据库迁移脚本001", Status: "todo"},
	}
	if err := svc.UpdateTasks(ctx, workdir, draft.ID, updated); err != nil {
		t.Fatalf("UpdateTasks: %v", err)
	}
	res, err := svc.Confirm(ctx, workdir, draft.ID)
	if err != nil {
		t.Fatalf("Confirm 应宽容成功: %v", err)
	}
	if res.Created != 2 || res.DroppedDeps != 1 {
		t.Fatalf("ConfirmResult: %+v", res)
	}
	ts := task.NewService(task.Options{})
	t.Cleanup(func() { _ = ts.Close() })
	list, err := ts.List(ctx, workdir, task.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	hot := findTask(list.Tree, "配置加载与热重载")
	if hot == nil {
		t.Fatal("任务缺失")
	}
	if len(hot.DependsOn) != 0 {
		t.Fatalf("坏引用应被忽略: %v", hot.DependsOn)
	}
}

// TestConfirm_DepsByID：LLM 输出带 id、depends_on 用 id 引用（与标题解耦）→ 确认导入依赖映射正确。
func TestConfirm_DepsByID(t *testing.T) {
	doc := `{"tasks":[
	  {"id":"T1","title":"配置加载与热重载","status":"doing",
	   "children":[{"id":"T2","title":"数据库迁移脚本","status":"todo"}]},
	  {"id":"T3","title":"修复登录页","status":"todo","priority":5,"depends_on":["T2"]}
	]}`
	srv := mockLLM(t, doc)
	svc, _ := newParser(t, srv.URL)
	workdir := initParserProject(t)
	ctx := context.Background()
	draft, err := svc.Parse(ctx, workdir, ParseInput{Content: "# 文档\n", SourceFile: "docs/id-deps.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := svc.Confirm(ctx, workdir, draft.ID); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	ts := task.NewService(task.Options{})
	t.Cleanup(func() { _ = ts.Close() })
	list, err := ts.List(ctx, workdir, task.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	child := findTask(list.Tree, "数据库迁移脚本")
	fix := findTask(list.Tree, "修复登录页")
	if child == nil || fix == nil {
		t.Fatalf("任务缺失: child=%v fix=%v", child, fix)
	}
	if len(fix.DependsOn) != 1 || fix.DependsOn[0] != child.ID {
		t.Fatalf("修复登录页 depends_on: %v, want %s", fix.DependsOn, child.ID)
	}
}
