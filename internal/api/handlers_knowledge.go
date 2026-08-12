package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"tangoforge/internal/auth"
	"tangoforge/internal/knowledge"

	"github.com/go-chi/chi/v5"
)

// ---- 库 CRUD（docs/KNOWLEDGE-BASE.md §6）----

// handleKnowledgeBasesGet 库列表（GET /api/knowledge/bases，含文档数）。
func (s *Server) handleKnowledgeBasesGet(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	res, err := s.knowledgeSvc.ListBases(r.Context(), workdir)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": res})
}

// knowledgeBaseReq 创建/更新库请求体。
type knowledgeBaseReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// handleKnowledgeBasesCreate 创建库（POST /api/knowledge/bases）。
func (s *Server) handleKnowledgeBasesCreate(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	var req knowledgeBaseReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "KNOWLEDGE_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}
	kb, err := s.knowledgeSvc.CreateBase(r.Context(), workdir, req.Name, req.Description)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"code": 0, "data": kb})
}

// handleKnowledgeBasePatch 重命名/改描述（PATCH /api/knowledge/bases/:id）。
func (s *Server) handleKnowledgeBasePatch(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "KNOWLEDGE_INVALID", "库 id 非法", err.Error())
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "KNOWLEDGE_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}
	kb, err := s.knowledgeSvc.UpdateBase(r.Context(), workdir, id, req.Name, req.Description)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": kb})
}

// handleKnowledgeBaseDelete 删除库（DELETE /api/knowledge/bases/:id，仅移除边）。
func (s *Server) handleKnowledgeBaseDelete(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "KNOWLEDGE_INVALID", "库 id 非法", err.Error())
		return
	}
	if err := s.knowledgeSvc.DeleteBase(r.Context(), workdir, id); err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]any{"id": id}})
}

// ---- 文档 CRUD ----

// handleKnowledgeDocumentsGet 文档列表（GET /api/knowledge/documents）。
func (s *Server) handleKnowledgeDocumentsGet(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	q := r.URL.Query()
	f := knowledge.DocumentFilter{
		Status: q.Get("filter[status]"),
		Q:      q.Get("q"),
	}
	if kb := q.Get("filter[kb_id]"); kb != "" {
		f.KBID, _ = strconv.ParseInt(kb, 10, 64)
	}
	if p := q.Get("page"); p != "" {
		f.Page, _ = strconv.Atoi(p)
	}
	if sz := q.Get("size"); sz != "" {
		f.Size, _ = strconv.Atoi(sz)
	}
	res, err := s.knowledgeSvc.ListDocuments(r.Context(), workdir, f)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": res})
}

// knowledgeDocumentReq 注册/关联文档请求体。
type knowledgeDocumentReq struct {
	Path    string  `json:"path"`
	Copy    string  `json:"copy"` // none / copy / auto
	KBIDs   []int64 `json:"kb_ids"`
	TaskID  string  `json:"task_id"`
	DocID   string  `json:"document_id"`
	NewPath string  `json:"new_path"`
	Content string  `json:"content"`
}

// handleKnowledgeDocumentRegister 注册文档（POST /api/knowledge/documents，存在则复用）。
func (s *Server) handleKnowledgeDocumentRegister(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	var req knowledgeDocumentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "DOCUMENT_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}
	doc, err := s.knowledgeSvc.RegisterDocument(r.Context(), workdir, req.Path, req.Copy, req.KBIDs)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"code": 0, "data": doc})
}

// handleKnowledgeDocumentGet 文档详情（GET /api/knowledge/documents/:id）。
func (s *Server) handleKnowledgeDocumentGet(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	doc, err := s.knowledgeSvc.GetDocument(r.Context(), workdir, chi.URLParam(r, "id"))
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": doc})
}

// handleKnowledgeDocumentContentGet 阅读原文（GET /api/knowledge/documents/:id/content）。
func (s *Server) handleKnowledgeDocumentContentGet(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	doc, err := s.knowledgeSvc.GetDocument(r.Context(), workdir, chi.URLParam(r, "id"))
	if err != nil {
		writeBizError(w, err)
		return
	}
	if doc.Type == knowledge.DocTypeBinary {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]any{
			"type": doc.Type, "path": doc.AbsPath,
		}})
		return
	}
	content, err := readDiskFile(doc.AbsPath)
	if err != nil {
		writeBizError(w, knowledge.NewDocumentMissing("文件不可读: %s", doc.AbsPath))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]any{
		"type": doc.Type, "content": content, "path": doc.AbsPath,
	}})
}

// handleKnowledgeDocumentContentPut 编辑原文（PUT /api/knowledge/documents/:id/content，写盘 → 重索引）。
func (s *Server) handleKnowledgeDocumentContentPut(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	id := chi.URLParam(r, "id")
	var req knowledgeDocumentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "DOCUMENT_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}
	if err := s.knowledgeSvc.UpdateContent(r.Context(), workdir, id, req.Content); err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]any{"id": id}})
}

// handleKnowledgeDocumentRelink 重新链接（POST /api/knowledge/documents/:id/relink）。
func (s *Server) handleKnowledgeDocumentRelink(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	id := chi.URLParam(r, "id")
	var req knowledgeDocumentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "DOCUMENT_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}
	doc, err := s.knowledgeSvc.RelinkDocument(r.Context(), workdir, id, req.NewPath, req.Copy)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": doc})
}

// handleKnowledgeDocumentDelete 解除引用（DELETE /api/knowledge/documents/:id）。
func (s *Server) handleKnowledgeDocumentDelete(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	id := chi.URLParam(r, "id")
	if err := s.knowledgeSvc.DeleteDocument(r.Context(), workdir, id); err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]any{"id": id}})
}

// ---- 任务关联 ----

// handleKnowledgeLink 任务关联（POST /api/knowledge/link）。
func (s *Server) handleKnowledgeLink(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	var req knowledgeDocumentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "DOCUMENT_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}
	if err := s.knowledgeSvc.LinkTask(r.Context(), workdir, req.TaskID, req.DocID, req.Path, req.Copy, req.KBIDs); err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]any{"task_id": req.TaskID}})
}

// handleKnowledgeUnlink 解除任务关联（POST /api/knowledge/unlink）。
func (s *Server) handleKnowledgeUnlink(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	var req struct {
		TaskID string `json:"task_id"`
		DocID  string `json:"document_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "DOCUMENT_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}
	if err := s.knowledgeSvc.UnlinkTask(r.Context(), workdir, req.TaskID, req.DocID); err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]any{"ok": true}})
}

// handleKnowledgeTaskDocuments 任务关联的文档列表（GET /api/knowledge/tasks/:taskId）。
func (s *Server) handleKnowledgeTaskDocuments(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	docs, err := s.knowledgeSvc.TaskDocuments(r.Context(), workdir, chi.URLParam(r, "taskId"))
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": docs})
}

// ---- 检索与扫描 ----

// handleKnowledgeSearch 向量检索（GET /api/knowledge/search）。
func (s *Server) handleKnowledgeSearch(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	q := r.URL.Query()
	query := knowledge.SearchQuery{Q: q.Get("q")}
	if kb := q.Get("kb_id"); kb != "" {
		query.KBID, _ = strconv.ParseInt(kb, 10, 64)
	}
	if k := q.Get("top_k"); k != "" {
		query.TopK, _ = strconv.Atoi(k)
	}
	if th := q.Get("threshold"); th != "" {
		query.Threshold, _ = strconv.ParseFloat(th, 64)
	}
	res, err := s.knowledgeSvc.Search(r.Context(), workdir, query)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": res})
}

// handleKnowledgeScan 手动触发扫描（POST /api/knowledge/scan）。
func (s *Server) handleKnowledgeScan(w http.ResponseWriter, r *http.Request) {
	stats, err := s.knowledgeScanner.Scan(r.Context())
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": stats})
}

// readDiskFile 读取磁盘文件文本（上限 2MB，传输层薄封装）。
func readDiskFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	const maxRead = 2 << 20
	if len(data) > maxRead {
		data = data[:maxRead]
	}
	return string(data), nil
}
