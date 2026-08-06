package api

import (
	"net/http"

	"tangoforge/internal/auth"
)

// handleGraph 全景图全量数据（GET /api/graph，graph.read）。
// 薄封装：数据组装下沉至 task.Service.Graph（TF-017 与 MCP graph_get 复用）。
// 返回全部未归档任务（排除回收站）的扁平列表 + 父子/依赖边；服务端不聚簇。
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	data, err := s.tasks.Graph(r.Context(), workdir)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": data})
}
