package api

import (
	"encoding/json"
	"net/http"
	"tangoforge/internal/parser"

	"github.com/go-chi/chi/v5"
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

// handleImportDraftGet GET /api/import/drafts/:id（import.run）：草稿明细（完整任务树，审阅用）。
func (s *Server) handleImportDraftGet(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	detail, err := s.parserSvc.Get(r.Context(), workdir, chi.URLParam(r, "id"))
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": detail})
}

// handleImportDraftUpdateTasks PUT /api/import/drafts/:id/tasks（import.run）：
// 审阅编辑保存——整体更新草稿任务树（校验后重写 parsed_json）。
func (s *Server) handleImportDraftUpdateTasks(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	var req struct {
		Tasks []parser.ParsedTask `json:"tasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}
	if err := s.parserSvc.UpdateTasks(r.Context(), workdir, chi.URLParam(r, "id"), req.Tasks); err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]bool{"ok": true}})
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
