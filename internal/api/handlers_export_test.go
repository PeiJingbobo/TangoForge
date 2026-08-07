package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tangoforge/internal/config"
)

// exportTestServer 构造带任务与 mock LLM 的 API Server。
func exportTestServer(t *testing.T, llmBody string) *Server {
	t.Helper()
	var llmURL string
	if llmBody != "" {
		srv := mockLLMResponse(t, llmBody)
		llmURL = srv.URL
	}
	s := newAPIServer(t, func(cfg *config.GlobalConfig) {
		if llmURL != "" {
			cfg.LLM = config.LLMConfig{BaseURL: llmURL, Model: "mock", APIKind: "openai", TimeoutSec: 5, Retries: 0}
		}
	})
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// seedTaskViaAPI 建一个任务（UI）。
func seedTaskViaAPI(t *testing.T, srv *Server, dir, title string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"title": title, "priority": "high"})
	rec := uiReq(t, srv, http.MethodPost, "/api/tasks", dir, string(body))
	mustCode(t, rec, http.StatusCreated, "create task")
}

func TestExport_CopyDefault(t *testing.T) {
	srv := exportTestServer(t, "")
	dir := importProjectViaAPI(t, srv)
	seedTaskViaAPI(t, srv, dir, "导出任务")

	rec := uiReq(t, srv, http.MethodPost, "/api/export", dir, `{"target":"copy"}`)
	out := mustCode(t, rec, http.StatusOK, "export copy")
	if !strings.Contains(out, `"code":0`) || !strings.Contains(out, "导出任务") {
		t.Fatalf("导出响应: %s", out)
	}
	// 缺省 copy 路径已写盘。
	if _, err := os.Stat(filepath.Join(dir, ".taskboard", "export.md")); err != nil {
		t.Fatalf("export.md 未生成: %v", err)
	}
}

func TestExport_OverwriteWithoutPath(t *testing.T) {
	srv := exportTestServer(t, "")
	dir := importProjectViaAPI(t, srv)
	seedTaskViaAPI(t, srv, dir, "x")

	rec := uiReq(t, srv, http.MethodPost, "/api/export", dir, `{"target":"overwrite"}`)
	out := mustCode(t, rec, http.StatusUnprocessableEntity, "overwrite no path")
	if apiCode(t, out) != "EXPORT_FAILED" {
		t.Fatalf("code=%s", apiCode(t, out))
	}
}

func TestExport_AgentDefaultDenied(t *testing.T) {
	srv := exportTestServer(t, "")
	dir := importProjectViaAPI(t, srv)

	rec := agentReq(t, srv, http.MethodPost, "/api/export", dir, `{"target":"copy"}`)
	mustCode(t, rec, http.StatusForbidden, "agent export denied")
}

func TestExport_TemplateGenerate(t *testing.T) {
	validTmpl := "# {{.Project.Name}}\n{{range .Tasks}}\n{{header .Level .Title}} [{{.Status}}]\n{{end}}"
	srv := exportTestServer(t, validTmpl)
	dir := importProjectViaAPI(t, srv)
	seedTaskViaAPI(t, srv, dir, "模板任务")

	body, _ := json.Marshal(map[string]any{"example": "# 示例\n## 任务A\n"})
	rec := uiReq(t, srv, http.MethodPost, "/api/export/template/generate", dir, string(body))
	out := mustCode(t, rec, http.StatusOK, "template generate")
	if !strings.Contains(out, "generated-template.tmpl") {
		t.Fatalf("响应: %s", out)
	}
	// config.template_path 已更新。
	cfg, err := config.LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Export.TemplatePath == "" {
		t.Fatalf("config.template_path 未更新")
	}
	// llm 模式渲染走新模板。
	rec = uiReq(t, srv, http.MethodPost, "/api/export", dir, `{"template_mode":"llm","target":"copy"}`)
	out = mustCode(t, rec, http.StatusOK, "export llm mode")
	if !strings.Contains(out, "模板任务") {
		t.Fatalf("llm 模式导出: %s", out)
	}
}

func TestExport_TemplateGenerateInvalid(t *testing.T) {
	srv := exportTestServer(t, "{{.broken]")
	dir := importProjectViaAPI(t, srv)

	body, _ := json.Marshal(map[string]any{"example": "# 示例\n"})
	rec := uiReq(t, srv, http.MethodPost, "/api/export/template/generate", dir, string(body))
	out := mustCode(t, rec, http.StatusUnprocessableEntity, "invalid template")
	if apiCode(t, out) != "TEMPLATE_INVALID" {
		t.Fatalf("code=%s body=%s", apiCode(t, out), out)
	}
}

// TF-038：GET /api/export/template 返回模板内容（default=内置；llm 未生成 → TEMPLATE_INVALID）。
func TestExport_TemplateContent(t *testing.T) {
	srv := exportTestServer(t, "")
	dir := importProjectViaAPI(t, srv)

	// default：返回内置模板（含 Front Matter 特征）。
	rec := uiReq(t, srv, http.MethodGet, "/api/export/template?mode=default", dir, "")
	out := mustCode(t, rec, http.StatusOK, "default template")
	if !strings.Contains(out, "generated_at") || !strings.Contains(out, "header") {
		t.Fatalf("default 模板内容: %s", out)
	}

	// llm 未生成 → TEMPLATE_INVALID。
	rec = uiReq(t, srv, http.MethodGet, "/api/export/template?mode=llm", dir, "")
	out = mustCode(t, rec, http.StatusUnprocessableEntity, "llm not generated")
	if apiCode(t, out) != "TEMPLATE_INVALID" {
		t.Fatalf("code=%s body=%s", apiCode(t, out), out)
	}

	// 生成 LLM 模板后再查 → 返回生成的模板。
	srv2 := exportTestServer(t, "LLM-TPL: {{header .Level .Title}}")
	dir2 := importProjectViaAPI(t, srv2)
	body, _ := json.Marshal(map[string]any{"example": "# 示例\n"})
	rec = uiReq(t, srv2, http.MethodPost, "/api/export/template/generate", dir2, string(body))
	mustCode(t, rec, http.StatusOK, "generate")
	rec = uiReq(t, srv2, http.MethodGet, "/api/export/template?mode=llm", dir2, "")
	out = mustCode(t, rec, http.StatusOK, "llm template")
	if !strings.Contains(out, "LLM-TPL") {
		t.Fatalf("llm 模板内容: %s", out)
	}
}
