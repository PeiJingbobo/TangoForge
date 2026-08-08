package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"tangoforge/internal/audit"
	"tangoforge/internal/auth"
	"tangoforge/internal/project"
)

// projectImportReq 导入项目请求体。
type projectImportReq struct {
	Workdir string `json:"workdir"`
}

// handleProjectList 项目列表（GET /api/projects）。
//
// 豁免 X-Project（QA P3-2）；UI 与 Agent 均可访问（project.read 默认授予，
// 项目列表仅含名称+路径，不泄露业务数据；permissions 按项目存储，
// 列表场景无项目上下文，不逐项查表）。
func (s *Server) handleProjectList(w http.ResponseWriter, r *http.Request) {
	list, err := s.projects.List(r.Context())
	if err != nil {
		writeBizError(w, err)
		return
	}
	if list == nil {
		list = []project.Project{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": list})
}

// handleProjectImport 导入目录为项目（POST /api/projects/import）。
// 导入成功后写审计 project.imported。
func (s *Server) handleProjectImport(w http.ResponseWriter, r *http.Request) {
	var req projectImportReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体必须是 JSON {workdir: <绝对路径>}", "")
		return
	}
	p, err := s.projects.Import(r.Context(), req.Workdir)
	if err != nil {
		writeBizError(w, err)
		return
	}
	s.writeProjectAudit(r, "project.imported", req.Workdir, audit.ResultOK, "")
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": p})
}

// handleProjectCheck 检查目录导入前置状态（POST /api/projects/check，TF-041 引导 Step 0）：
// {registered, has_meta, meta_valid, meta_reason}。豁免 X-Project（目录可能未注册）。
func (s *Server) handleProjectCheck(w http.ResponseWriter, r *http.Request) {
	var req projectImportReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体必须是 JSON {workdir: <绝对路径>}", "")
		return
	}
	res, err := s.projects.Check(r.Context(), req.Workdir)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": res})
}

// handleProjectResetMetadata 清空历史元数据（POST /api/projects/import/reset，TF-041 引导）：
// 删除 {workdir}/.taskboard/（仅限未注册目录；元数据版本过旧/损坏时用户确认后重置）。
// 仅 UI（删除元数据是破坏性操作）。
func (s *Server) handleProjectResetMetadata(w http.ResponseWriter, r *http.Request) {
	actor := auth.ActorFrom(r.Context())
	if actor.Class != auth.ClassUI {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED",
			"重置元数据仅允许 UI 操作（回环 + X-UI-Token）", actor.Class)
		return
	}
	var req projectImportReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体必须是 JSON {workdir: <绝对路径>}", "")
		return
	}
	if err := s.projects.ResetMetadata(r.Context(), req.Workdir); err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]bool{"reset": true}})
}

// handleProjectRemove 移除项目注册记录（DELETE /api/projects/:id）。
//
// 仅 UI（回环 + X-UI-Token，识别层保证）；Agent / 远程一律 403（QA 默认项 3：
// 项目记录移除是注册表级操作，Agent 无权限）。绝不删除磁盘数据（project.Service 语义）。
func (s *Server) handleProjectRemove(w http.ResponseWriter, r *http.Request) {
	actor := auth.ActorFrom(r.Context())
	if actor.Class != auth.ClassUI {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED",
			"移除项目记录仅允许 UI 操作（回环 + X-UI-Token）", actor.Class)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "项目 id 非法", err.Error())
		return
	}
	if err := s.projects.Remove(r.Context(), id); err != nil {
		if errors.Is(err, project.ErrNotFound) {
			writeBizError(w, err)
			return
		}
		writeBizError(w, err)
		return
	}
	// 移除后无 workdir 可查审计（记录已删）；以注册表库为目标的审计写入全局 registry 不可行，
	// 此处仅记录到日志（移除本身是注册表级操作，项目库审计表随项目保留）。
	s.logger.Info("project record removed", "id", id)
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]bool{"removed": true}})
}

// projectRenameReq 重命名请求体。
type projectRenameReq struct {
	Name string `json:"name"`
}

// handleProjectRename 重命名项目显示名称（PATCH /api/projects/:id）。
//
// 仅 UI（同删除：注册表级操作，Agent 无权限）；仅改 projects.name 行，
// 不触碰磁盘与 workdir（project.Service.Rename 语义）；审计 project.renamed（按 workdir）。
func (s *Server) handleProjectRename(w http.ResponseWriter, r *http.Request) {
	actor := auth.ActorFrom(r.Context())
	if actor.Class != auth.ClassUI {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED",
			"重命名项目仅允许 UI 操作（回环 + X-UI-Token）", actor.Class)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "项目 id 非法", err.Error())
		return
	}
	var req projectRenameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体必须是 JSON {name: <新名称>}", "")
		return
	}
	p, err := s.projects.Rename(r.Context(), id, req.Name)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			writeBizError(w, err)
			return
		}
		if errors.Is(err, project.ErrInvalidWorkdir) {
			writeError(w, http.StatusUnprocessableEntity, "TASK_INVALID", err.Error(), "")
			return
		}
		writeBizError(w, err)
		return
	}
	s.writeProjectAudit(r, "project.renamed", p.Workdir, audit.ResultOK, "")
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": p})
}

// writeProjectAudit 写项目级审计（projects 组无项目库上下文时的兜底路径）。
func (s *Server) writeProjectAudit(r *http.Request, action, workdir, result, detail string) {
	if s.audit == nil || workdir == "" {
		return
	}
	s.audit.Write(r.Context(), workdir, auditEntryOf(r, action, workdir, result, detail))
}

// auditEntryOf 从请求上下文构造审计条目（actor 取自识别结果）。
func auditEntryOf(r *http.Request, action, target, result, detail string) audit.Entry {
	actor := auth.ActorFrom(r.Context())
	return audit.Entry{
		Actor: actor.Name, ActorClass: actor.Class,
		Action: action, Target: target, Result: result, Detail: detail,
	}
}
