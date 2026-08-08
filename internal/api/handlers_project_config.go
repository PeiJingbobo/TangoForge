package api

import (
	"encoding/json"
	"net/http"
	"tangoforge/internal/auth"
	"tangoforge/internal/config"
)

// 项目配置端点（GET/PUT /api/project-config，TF-032）——项目设置页数据源。
//
// 语义：
//   - GET 读取项目 config.yaml（{workdir}/.taskboard/config.yaml），缺失回退默认配置；
//     权限 state_machine.read（UI 放行，Agent 默认无权限）；
//   - PUT 全量覆盖 state_machine + export 两节：状态机校验（编辑校验 + STATUS_IN_USE，
//     复用 task.ValidateStateMachineUpdate）→ 部分更新写盘（UpdateProjectFile，
//     保留 config.yaml 未知节）→ 审计 project_config.updated + WS project_config.changed；
//   - PUT 仅 UI（回环 + X-UI-Token）：配置为业务敏感数据，Agent / 远程一律 403
//     （与 PUT /api/permissions、PUT /api/config 同策略）。

// handleProjectConfigGet 读取项目配置（GET /api/project-config，state_machine.read）。
func (s *Server) handleProjectConfigGet(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	cfg, err := config.LoadProject(workdir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CONFIG_LOAD_FAILED",
			"项目配置读取失败", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": cfg})
}

// handleProjectConfigPut 更新项目配置（PUT /api/project-config，仅 UI）。
// 请求体为完整 ProjectConfig（state_machine + export 全量覆盖）。
func (s *Server) handleProjectConfigPut(w http.ResponseWriter, r *http.Request) {
	actor := auth.ActorFrom(r.Context())
	if actor.Class != auth.ClassUI {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED",
			"项目配置修改仅允许 UI 操作（回环 + X-UI-Token）", actor.Class)
		return
	}
	workdir := auth.WorkdirFrom(r.Context())

	var req config.ProjectConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}

	// 状态机校验（编辑校验 + 占用校验，与 PUT /api/state-machine 共用业务层）。
	norm, err := s.tasks.ValidateStateMachineUpdate(r.Context(), workdir, req.StateMachine)
	if err != nil {
		writeBizError(w, err)
		return
	}

	// 部分更新写盘：替换 state_machine + export 节，保留未知节（TF-032）。
	if err := config.UpdateProjectFile(workdir, func(cfg *config.ProjectConfig) {
		cfg.StateMachine = norm
		cfg.Export = req.Export
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "CONFIG_SAVE_FAILED",
			"项目配置写入失败", err.Error())
		return
	}

	// 审计 + WS 事件（与 task 写钩子同构，双通道齐全）。
	if s.audit != nil {
		s.audit.Write(r.Context(), workdir, auditEntryOf(r, "project_config.updated", workdir, "ok", ""))
	}
	s.hub.Publish(workdir, "project_config.changed", map[string]any{"path": workdir})

	// 响应最新配置（与 state-machine 端点一致，返回落盘结果）。
	cfg, err := config.LoadProject(workdir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CONFIG_LOAD_FAILED",
			"项目配置读取失败", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": cfg})
}
