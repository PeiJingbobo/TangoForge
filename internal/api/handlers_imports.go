package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"tangoforge/internal/parser"
)

// handleImport POST /api/import（import.run，TF-018）：提交 Markdown 解析 → 生成草稿。
// body 支持 {file_path}（相对 workdir 或绝对）或 {content, source_file} 双形态（QA P4-1）。
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	var in parser.ParseInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体必须为 JSON", err.Error())
		return
	}
	draft, err := s.parserSvc.Parse(r.Context(), workdir, in)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": draft})
}

// handleImportDrafts GET /api/import/drafts（import.run）：pending 草稿列表。
func (s *Server) handleImportDrafts(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	drafts, err := s.parserSvc.List(r.Context(), workdir)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": drafts})
}

// handleImportDraftConfirm POST /api/import/drafts/:id/confirm（import.confirm）：
// 确认草稿入库（文件级全量覆盖，事务原子）。
func (s *Server) handleImportDraftConfirm(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	res, err := s.parserSvc.Confirm(r.Context(), workdir, chi.URLParam(r, "id"))
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": res})
}

// handleImportDraftDiscard DELETE /api/import/drafts/:id（import.run）：丢弃草稿。
func (s *Server) handleImportDraftDiscard(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	if err := s.parserSvc.Discard(r.Context(), workdir, chi.URLParam(r, "id")); err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]bool{"ok": true}})
}
