package api

import (
	"encoding/json"
	"net/http"
	"tangoforge/internal/exporter"
)

// handleExport POST /api/export（export.run，TF-019）：重新生成 Markdown。
// body: {template_mode: default|llm, target: overwrite|copy, path}
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	var opts exporter.RenderOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体必须为 JSON", err.Error())
		return
	}
	res, err := s.exporterSvc.Render(r.Context(), workdir, opts)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": res})
}

// handleExportTemplateGenerate POST /api/export/template/generate（export.run，TF-019）：
// 依据示例文档由 LLM 生成导出模板（校验后写盘 + 更新项目配置）。
func (s *Server) handleExportTemplateGenerate(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	var in struct {
		Example string `json:"example"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体必须为 JSON", err.Error())
		return
	}
	tmpl, err := s.exporterSvc.GenerateTemplate(r.Context(), workdir, in.Example)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]string{
		"template": tmpl,
		"path":     workdir + "/.taskboard/generated-template.tmpl",
	}})
}

// handleExportTemplateContent GET /api/export/template?mode=default|llm（export.run，TF-038）：
// 返回当前模板文本供导出对话框预览。llm 未生成 → TEMPLATE_INVALID（前端引导生成）。
func (s *Server) handleExportTemplateContent(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	mode := r.URL.Query().Get("mode")
	tmpl, err := s.exporterSvc.TemplateContent(r.Context(), workdir, mode)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]string{
		"template": tmpl,
		"mode":     mode,
	}})
}
