package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"tangoforge/internal/auth"
	"tangoforge/internal/task"

	"github.com/go-chi/chi/v5"
)

// handleTaskList 任务树 / 扁平分页（GET /api/tasks?filter[status]=&q=&page=&size=）。
// workdir 由 projectMiddleware 写入 ctx（auth.WorkdirFrom）。
func (s *Server) handleTaskList(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	q := r.URL.Query()
	f := task.ListFilter{
		Status: q.Get("filter[status]"),
		Q:      q.Get("q"),
	}
	if p := q.Get("page"); p != "" {
		f.Page, _ = strconv.Atoi(p)
	}
	if sz := q.Get("size"); sz != "" {
		f.Size, _ = strconv.Atoi(sz)
	}
	res, err := s.tasks.List(r.Context(), workdir, f)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": res})
}

// handleTaskGet 任务详情（GET /api/tasks/:id）。
// TF-050：响应内嵌 knowledge_documents 摘要数组（任务关联文档，knowledge.read 读取；
// 权限中间件已挂 task.read，knowledge.read 默认只读也放行；查询失败不阻断详情返回）。
func (s *Server) handleTaskGet(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	id := chi.URLParam(r, "id")
	t, err := s.tasks.Get(r.Context(), workdir, id)
	if err != nil {
		writeBizError(w, err)
		return
	}
	data := map[string]any{
		"id":                  t.ID,
		"project_id":          t.ProjectID,
		"parent_id":           t.ParentID,
		"title":               t.Title,
		"description":         t.Description,
		"status":              t.Status,
		"priority":            t.Priority,
		"tags":                t.Tags,
		"assignee":            t.Assignee,
		"depends_on":          t.DependsOn,
		"archived_from":       t.ArchivedFrom,
		"source_file":         t.SourceFile,
		"source_section":      t.SourceSection,
		"number":              t.Number,
		"created_at":          t.CreatedAt,
		"updated_at":          t.UpdatedAt,
		"knowledge_documents": []any{},
	}
	// 任务关联文档（摘要数组）。
	if s.knowledgeSvc != nil {
		if docs, err := s.knowledgeSvc.TaskDocuments(r.Context(), workdir, id); err == nil {
			summaries := make([]map[string]any, 0, len(docs))
			for _, d := range docs {
				summaries = append(summaries, map[string]any{
					"id":           d.ID,
					"display_name": d.DisplayName,
					"path":         d.Path,
					"abs_path":     d.AbsPath,
					"rel_path":     d.RelPath,
					"type":         d.Type,
					"status":       d.Status,
					"summary":      d.Summary,
				})
			}
			data["knowledge_documents"] = summaries
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": data})
}

// handleTaskCreate 创建任务（POST /api/tasks）。
func (s *Server) handleTaskCreate(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	var in task.CreateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}
	t, err := s.tasks.Create(r.Context(), workdir, in)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"code": 0, "data": t})
}

// taskUpdateReq PATCH 请求体：业务层 UpdateInput + 可选 status（独立走 ChangeStatus）。
type taskUpdateReq struct {
	task.UpdateInput
	Status *string `json:"status"`
}

// handleTaskUpdate 更新任务字段（PATCH /api/tasks/:id）。
//
// 职责边界（docs/TASK-SEMANTICS.md §4.1）：status 与其它字段分属不同接口。
//   - body 含 status → 调 ChangeStatus（需要 task.update_status，此处二次校验；
//     权限中间件已挂 task.update，二者任一被拒均不得执行对应动作）；
//   - 其余字段 → 调 Update（task.update）。
//
// 顺序：先字段更新，后状态流转；各自成功均经写钩子产生审计。
func (s *Server) handleTaskUpdate(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	id := chi.URLParam(r, "id")

	var req taskUpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}

	// 字段更新（UpdateInput 不含 status；JSON 中 status 已被独立捕获）。
	var hasField bool
	if req.Title != nil || req.Description != nil || req.Priority != nil ||
		req.Tags != nil || req.Assignee != nil || req.DependsOn != nil || req.ParentID != nil {
		hasField = true
		if _, err := s.tasks.Update(r.Context(), workdir, id, req.UpdateInput); err != nil {
			writeBizError(w, err)
			return
		}
	}

	// 状态流转（独立 ChangeStatus；需 task.update_status 权限）。
	if req.Status != nil {
		if !s.ensureAction(w, r, workdir, "task.update_status") {
			return // 403 已写
		}
		if _, err := s.tasks.ChangeStatus(r.Context(), workdir, id, *req.Status); err != nil {
			writeBizError(w, err)
			return
		}
	}

	if !hasField && req.Status == nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体不含任何可更新字段", "")
		return
	}

	t, err := s.tasks.Get(r.Context(), workdir, id)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": t})
}

// ensureAction 对非 UI 来源二次校验权限（PATCH 动态 action 场景，QA 默认项 2）。
// 未授权 → 403 + denied 审计；返回 false。
func (s *Server) ensureAction(w http.ResponseWriter, r *http.Request, workdir, action string) bool {
	actor := auth.ActorFrom(r.Context())
	if actor.Class == auth.ClassUI {
		return true
	}
	allowed, err := s.perms.Allowed(r.Context(), workdir, action)
	if err != nil {
		writeBizError(w, err)
		return false
	}
	if !allowed {
		if s.perms.OnDenied != nil {
			s.perms.OnDenied(r.Context(), workdir, action)
		}
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED",
			"无权执行该操作（action="+action+"）", actor.Class)
		return false
	}
	return true
}

// handleTaskArchive 归档任务（POST /api/tasks/:id/archive）。
func (s *Server) handleTaskArchive(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	id := chi.URLParam(r, "id")
	res, err := s.tasks.Archive(r.Context(), workdir, id)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": res})
}

// handleTaskRestore 还原任务（POST /api/tasks/:id/restore）。
// body 可选 {"fallback_todo": true}（docs/TASK-SEMANTICS.md §8.2）。
func (s *Server) handleTaskRestore(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	id := chi.URLParam(r, "id")
	var req struct {
		FallbackTodo bool `json:"fallback_todo"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // body 可缺省
	t, err := s.tasks.Restore(r.Context(), workdir, id, task.RestoreOptions{FallbackTodo: req.FallbackTodo})
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": t})
}

// handleTaskDelete 物理删除回收站任务（DELETE /api/tasks/:id）。
func (s *Server) handleTaskDelete(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	id := chi.URLParam(r, "id")
	t, err := s.tasks.Delete(r.Context(), workdir, id)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": t})
}
