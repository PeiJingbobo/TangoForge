package api

import (
	"net/http"
	"strconv"

	"tangoforge/internal/audit"
	"tangoforge/internal/auth"
)

// handleAuditQuery 审计查询（GET /api/audit，audit.read）。
// 支持 filter[actor] / filter[action] + page/size（默认 100 上限 500），ts 倒序。
func (s *Server) handleAuditQuery(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	q := r.URL.Query()
	f := audit.Filter{
		Actor:  q.Get("filter[actor]"),
		Action: q.Get("filter[action]"),
	}
	if p := q.Get("page"); p != "" {
		f.Page, _ = strconv.Atoi(p)
	}
	if sz := q.Get("size"); sz != "" {
		f.Size, _ = strconv.Atoi(sz)
	}
	res, err := s.audit.Query(r.Context(), workdir, f)
	if err != nil {
		writeBizError(w, err)
		return
	}
	if res.Items == nil {
		res.Items = []audit.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": res})
}

// handleAuditExport 导出审计日志（GET /api/audit/export，audit.read）。
// 返回 text/plain 文本（行格式 ts|actor|actor_class|action|target|result|detail，含表头），
// 不写盘（响应体即导出物，QA P3-8）。
func (s *Server) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	text, err := s.audit.Export(r.Context(), workdir)
	if err != nil {
		writeBizError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(text))
}
