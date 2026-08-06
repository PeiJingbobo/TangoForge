package api

import (
	"encoding/json"
	"net/http"

	"tangoforge/internal/auth"
	"tangoforge/internal/config"
)

// handleStateMachineGet 读取项目状态机定义（GET /api/state-machine，state_machine.read）。
func (s *Server) handleStateMachineGet(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	sm, err := s.tasks.GetStateMachine(r.Context(), workdir)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": sm})
}

// handleStateMachinePut 更新状态机（PUT /api/state-machine，state_machine.write）。
// 编辑校验 + 占用校验（STATUS_IN_USE）由业务层完成；写入经写钩子产生审计 state_machine.changed。
func (s *Server) handleStateMachinePut(w http.ResponseWriter, r *http.Request) {
	workdir := auth.WorkdirFrom(r.Context())
	var sm config.StateMachine
	if err := json.NewDecoder(r.Body).Decode(&sm); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}
	norm, err := s.tasks.UpdateStateMachine(r.Context(), workdir, sm)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": norm})
}
