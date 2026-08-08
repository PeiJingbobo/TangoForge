package api

import (
	"encoding/json"
	"net/http"
	"tangoforge/internal/auth"
)

// handlePermissionGet 查询项目权限范围（GET /api/permissions，permission.read）。
// 返回全量 16 项（含 allowed=false，QA P3-6），UI 编辑与 Agent 自查两用。
func (s *Server) handlePermissionGet(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	perms, err := s.perms.Get(r.Context(), workdir)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]any{"actions": perms}})
}

// permissionPutReq 全量覆盖请求体（QA P3-5）：{"actions": {"task.read": true, ...}}。
// 未提交项重置 false；未知 action 拒绝（TASK_INVALID）。
type permissionPutReq struct {
	Actions map[string]bool `json:"actions"`
}

// handlePermissionPut 修改权限（PUT /api/permissions）。
//
// 仅 UI（回环 + X-UI-Token）：识别层已保证 actor==ui 即回环且 Token 有效；
// Agent / 远程 / unknown 一律 403（REQUIREMENTS.md §7.4：CLI/MCP/HTTP 不提供权限修改）。
// 变更成功写审计 permission.changed（写钩子不覆盖权限域，此处显式写入）。
func (s *Server) handlePermissionPut(w http.ResponseWriter, r *http.Request) {
	actor := auth.ActorFrom(r.Context())
	if actor.Class != auth.ClassUI {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED",
			"权限修改仅允许 UI 操作（回环 + X-UI-Token）", actor.Class)
		return
	}
	workdir := auth.WorkdirFrom(r.Context())

	var req permissionPutReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}
	if req.Actions == nil {
		req.Actions = map[string]bool{}
	}
	got, err := s.perms.Set(r.Context(), workdir, req.Actions)
	if err != nil {
		writeBizError(w, err)
		return
	}
	if s.audit != nil {
		s.audit.Write(r.Context(), workdir, auditEntryOf(r, "permission.changed", workdir, "ok", ""))
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]any{"actions": got}})
}
