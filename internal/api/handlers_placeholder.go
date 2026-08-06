package api

import (
	"net/http"
)

// NOT_IMPLEMENTED 占位错误码：端点依赖的 P4 业务层（parser/exporter/skill）尚未落地。
// 路由先行注册保证端点可发现（QA P3-3），TF-018/019/020 完成后替换为真实 handler。

// handleImportPlaceholder POST /api/import（TF-018 落地）。
func (s *Server) handleImportPlaceholder(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"Markdown 导入（草稿流）随 TF-018 落地", "")
}

// handleImportDraftsPlaceholder GET /api/import/drafts（TF-018 落地）。
func (s *Server) handleImportDraftsPlaceholder(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"导入草稿列表随 TF-018 落地", "")
}

// handleImportDraftConfirmPlaceholder POST /api/import/drafts/:id/confirm（TF-018 落地）。
func (s *Server) handleImportDraftConfirmPlaceholder(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"草稿确认入库随 TF-018 落地", "")
}

// handleImportDraftDiscardPlaceholder DELETE /api/import/drafts/:id（TF-018 落地）。
func (s *Server) handleImportDraftDiscardPlaceholder(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"草稿丢弃随 TF-018 落地", "")
}

// handleExportPlaceholder POST /api/export（TF-019 落地）。
func (s *Server) handleExportPlaceholder(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"Markdown 导出随 TF-019 落地", "")
}

// handleExportTemplatePlaceholder POST /api/export/template/generate（TF-019 落地）。
func (s *Server) handleExportTemplatePlaceholder(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"LLM 生成模板随 TF-019 落地", "")
}
