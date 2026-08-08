package api

import (
	"encoding/json"
	"net/http"
	"tangoforge/internal/auth"
)

// skillTemplateReq 模板写入请求。
type skillTemplateReq struct {
	Content string `json:"content"`
}

// handleSkillTemplateGet GET /api/skill-template（skill.read）：返回全局默认 Skill 模板
// （{home}/.taskboard-app/skills/_template/SKILL.md；不存在回退内置模板）。
func (s *Server) handleSkillTemplateGet(w http.ResponseWriter, r *http.Request) {
	content, err := s.skills.DefaultTemplate(r.Context())
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]string{"content": content}})
}

// handleSkillTemplatePut PUT /api/skill-template（仅 UI）：写入自定义默认模板（QA-S4）。
func (s *Server) handleSkillTemplatePut(w http.ResponseWriter, r *http.Request) {
	if auth.ActorFrom(r.Context()).Class != auth.ClassUI {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "仅 App UI 可写 Skill 模板", "")
		return
	}
	var req skillTemplateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体必须为 JSON", "")
		return
	}
	if err := s.skills.WriteTemplate(r.Context(), req.Content); err != nil {
		writeBizError(w, err)
		return
	}
	s.auditWrite(r, "skill.template_written", "_template", "")
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]bool{"ok": true}})
}
