package exporter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"tangoforge/internal/config"
	"tangoforge/internal/db"
	"tangoforge/internal/llm"
	"tangoforge/internal/task"
)

// newEnv 初始化临时项目（meta.db + 默认 config），返回 workdir 与任务服务。
func newEnv(t *testing.T) (string, task.Service) {
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
	ts := task.NewService(task.Options{})
	t.Cleanup(func() { _ = ts.Close() })
	return workdir, ts
}

// mockLLM 固定响应的 mock OpenAI 服务。
func mockLLM(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"content":%s}}]}`, strconv.Quote(body))))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newService(t *testing.T, llmURL string, ts task.Service) *Service {
	t.Helper()
	svc := NewService(Options{
		Tasks: ts,
		LLM: func() config.LLMConfig {
			return config.LLMConfig{BaseURL: llmURL, Model: "mock", APIKind: "openai", TimeoutSec: 5, Retries: 0}
		},
	})
	return svc
}

// seedTasks 建任务（父 + 子 + 依赖）。
func seedTasks(t *testing.T, ts task.Service, wd string) {
	t.Helper()
	ctx := context.Background()
	child, err := ts.Create(ctx, wd, task.CreateInput{Title: "子任务", Status: strPtr("doing")})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := ts.Create(ctx, wd, task.CreateInput{
		Title: "父任务", Priority: "high", Tags: []string{"release", "bug"},
		Assignee: "张三", DependsOn: []string{child.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ts.Update(ctx, wd, child.ID, task.UpdateInput{ParentID: strPtrPtr(&parent.ID)})
	if err != nil {
		t.Fatal(err)
	}
}

func strPtr(s string) *string      { return &s }
func strPtrPtr(s *string) **string { return &s }

func TestRender_DefaultTemplate(t *testing.T) {
	wd, ts := newEnv(t)
	seedTasks(t, ts, wd)
	svc := newService(t, "", ts)

	res, err := svc.Render(context.Background(), wd, RenderOptions{Target: "copy"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// 默认 copy 路径 = {workdir}/.taskboard/export.md。
	if res.Path != filepath.Join(wd, ".taskboard", "export.md") {
		t.Fatalf("path: %s", res.Path)
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Fatalf("导出文件不存在: %v", err)
	}
	// 往返格式断言：Front Matter + 层级标题 + 状态/优先级/标签/负责人元数据行。
	for _, want := range []string{
		"---", "generated_at:",
		"## 父任务", "### 子任务",
		"- 状态: todo", "- 状态: doing",
		"- 优先级: 4", "标签: release, bug", "负责人: 张三",
	} {
		if !strings.Contains(res.Content, want) {
			t.Fatalf("导出内容缺少 %q:\n%s", want, res.Content)
		}
	}
}

func TestRender_CustomTemplate(t *testing.T) {
	wd, ts := newEnv(t)
	seedTasks(t, ts, wd)
	svc := newService(t, "", ts)

	// 自定义模板 + config 指向。
	custom := "# {{.Project.Name}}\n\n{{range .Tasks}}* {{.Title}}/{{.Status}}\n{{end}}"
	customPath := filepath.Join(wd, ".taskboard", "custom.tmpl")
	if err := os.WriteFile(customPath, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.LoadProject(wd)
	cfg.Export.TemplatePath = customPath
	if err := config.SaveProject(wd, cfg); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Render(context.Background(), wd, RenderOptions{Target: "copy"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(res.Content, "* 父任务/todo") || !strings.Contains(res.Content, "* 子任务/doing") {
		t.Fatalf("自定义模板渲染: %s", res.Content)
	}
}

func TestRender_OverwriteRequiresPath(t *testing.T) {
	wd, ts := newEnv(t)
	svc := newService(t, "", ts)
	_, err := svc.Render(context.Background(), wd, RenderOptions{Target: "overwrite"})
	if !errors.Is(err, ErrExportFailed) {
		t.Fatalf("期望 EXPORT_FAILED, got %v", err)
	}
}

func TestRender_LLMModeWithoutTemplate(t *testing.T) {
	wd, ts := newEnv(t)
	svc := newService(t, "", ts)
	_, err := svc.Render(context.Background(), wd, RenderOptions{TemplateMode: "llm", Target: "copy"})
	if !errors.Is(err, ErrTemplateInvalid) {
		t.Fatalf("期望 TEMPLATE_INVALID, got %v", err)
	}
}

func TestRender_ProjectNotFound(t *testing.T) {
	_, ts := newEnv(t)
	svc := newService(t, "", ts)
	_, err := svc.Render(context.Background(), "/no/such/project", RenderOptions{Target: "copy"})
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("期望 ErrProjectNotFound, got %v", err)
	}
}

func TestGenerateTemplate_OK(t *testing.T) {
	validTmpl := "---\ntitle: \"{{.Project.Name}}\"\n---\n{{range .Tasks}}\n{{header .Level .Title}} [{{.Status}}]\n{{end}}"
	srv := mockLLM(t, validTmpl)
	wd, ts := newEnv(t)
	seedTasks(t, ts, wd)
	svc := newService(t, srv.URL, ts)

	tmpl, err := svc.GenerateTemplate(context.Background(), wd, "# 示例\n## 任务A\n")
	if err != nil {
		t.Fatalf("GenerateTemplate: %v", err)
	}
	if tmpl != validTmpl {
		t.Fatalf("返回模板不符: %s", tmpl)
	}
	// 文件已写 + config 已更新。
	genPath := filepath.Join(wd, ".taskboard", "generated-template.tmpl")
	if _, err := os.Stat(genPath); err != nil {
		t.Fatalf("生成模板文件不存在: %v", err)
	}
	cfg, err := config.LoadProject(wd)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Export.TemplatePath != genPath {
		t.Fatalf("config.template_path 未更新: %s", cfg.Export.TemplatePath)
	}

	// llm 模式现在可渲染。
	res, err := svc.Render(context.Background(), wd, RenderOptions{TemplateMode: "llm", Target: "copy"})
	if err != nil {
		t.Fatalf("Render(llm): %v", err)
	}
	if !strings.Contains(res.Content, " [todo]") || !strings.Contains(res.Content, "## 父任务") {
		t.Fatalf("llm 模板渲染: %s", res.Content)
	}
}

func TestGenerateTemplate_InvalidTemplate(t *testing.T) {
	srv := mockLLM(t, "{{.Tasks]") // 非法模板
	wd, ts := newEnv(t)
	svc := newService(t, srv.URL, ts)

	_, err := svc.GenerateTemplate(context.Background(), wd, "# 示例\n")
	if !errors.Is(err, ErrTemplateInvalid) {
		t.Fatalf("期望 TEMPLATE_INVALID, got %v", err)
	}
	// 不写盘。
	if _, err := os.Stat(filepath.Join(wd, ".taskboard", "generated-template.tmpl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("非法模板不应写盘")
	}
}

func TestGenerateTemplate_LLMNotConfigured(t *testing.T) {
	wd, ts := newEnv(t)
	svc := NewService(Options{
		Tasks: ts,
		LLM:   func() config.LLMConfig { return config.DefaultLLMConfig() },
	})
	_, err := svc.GenerateTemplate(context.Background(), wd, "# 示例\n")
	if !errors.Is(err, llm.ErrNotConfigured) {
		t.Fatalf("期望 LLM_NOT_CONFIGURED, got %v", err)
	}
}

// flattenTree 单测（level 正确）。
func TestFlattenTreeLevels(t *testing.T) {
	nodes := []*task.TaskTreeNode{
		{Task: task.Task{Title: "A"}, Children: []*task.TaskTreeNode{
			{Task: task.Task{Title: "A1"}},
			{Task: task.Task{Title: "A2"}, Children: []*task.TaskTreeNode{{Task: task.Task{Title: "A21"}}}},
		}},
		{Task: task.Task{Title: "B"}},
	}
	var out []FlatTask
	flattenTree(nodes, 0, &out)
	if len(out) != 5 {
		t.Fatalf("数量 %d", len(out))
	}
	want := []int{0, 1, 1, 2, 0}
	for i, w := range want {
		if out[i].Level != w {
			t.Fatalf("out[%d].Level=%d want=%d", i, out[i].Level, w)
		}
	}
}
