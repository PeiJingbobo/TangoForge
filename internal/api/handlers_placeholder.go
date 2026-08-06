package api

import (
	"net/http"
)

// NOT_IMPLEMENTED 占位错误码：端点依赖的 P4 业务层尚未落地。
// 路由先行注册保证端点可发现（QA P3-3）；skill（TF-020）/ import（TF-018）已替换，
// export 端点待 TF-019 落地时替换为真实 handler。

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
