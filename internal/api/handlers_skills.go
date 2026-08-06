package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"tangoforge/internal/skill"
)

// handleSkills GET /api/skills（skill.read）：返回全部 Skill 索引（扫描 + 缓存同步）。
func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	items, err := s.skills.List(r.Context(), workdir)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": items})
}

// handleSkillInfo GET /api/skills/:name（skill.read）：返回单个 Skill 详情（skill_info）。
func (s *Server) handleSkillInfo(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	name := chi.URLParam(r, "name")
	sk, err := s.skills.Info(r.Context(), workdir, name)
	if err != nil {
		if isErr(err, skill.ErrSkillNotFound) {
			writeError(w, http.StatusNotFound, "SKILL_NOT_FOUND", "Skill 不存在", "")
			return
		}
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": sk})
}

// isErr 判断错误是否与目标哨兵相等（errors.Is 简写）。
func isErr(err error, target error) bool { return err != nil && errors.Is(err, target) }
