package api

import (
	"encoding/json"
	"net/http"
	"tangoforge/internal/audit"
	"tangoforge/internal/auth"
	"tangoforge/internal/skill"

	"github.com/go-chi/chi/v5"
)

// handleSkills GET /api/skills（skill.read）：技能包列表（内置 + 全局库，按名称升序）。
// 兼容 TF-028 旧端点：原「项目 .taskboard/skills/ 扫描」已废弃，语义迁移到全局技能库。
func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	items, err := s.skills.ListPackages(r.Context())
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": items})
}

// handleSkillInfo GET /api/skills/:name（skill.read）：技能包详情（SKILL.md）。
// 兼容 TF-028 旧端点（原 skill_info）；新端点见 handleSkillPackageInfo。
func (s *Server) handleSkillInfo(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	pkg, err := s.skills.Info(r.Context(), name)
	if err != nil {
		if isErr(err, skill.ErrSkillNotFound) {
			writeError(w, http.StatusNotFound, "SKILL_NOT_FOUND", "技能包不存在", "")
			return
		}
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": pkg})
}

// handleSkillPackages GET /api/skills/packages（skill.read）：技能包列表（TF-033 新端点）。
func (s *Server) handleSkillPackages(w http.ResponseWriter, r *http.Request) {
	items, err := s.skills.ListPackages(r.Context())
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": items})
}

// handleSkillPackageInfo GET /api/skills/packages/{name}（skill.read）：技能包详情。
func (s *Server) handleSkillPackageInfo(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	pkg, err := s.skills.Info(r.Context(), name)
	if err != nil {
		if isErr(err, skill.ErrSkillNotFound) {
			writeError(w, http.StatusNotFound, "SKILL_NOT_FOUND", "技能包不存在", "")
			return
		}
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": pkg})
}

// skillPackageWriteReq 自定义技能包写入请求（PUT /api/skills/packages/{name}）。
type skillPackageWriteReq struct {
	Content string `json:"content"` // SKILL.md 全文（含 frontmatter）
}

// handleSkillPackageWrite PUT /api/skills/packages/{name}（仅 UI）：
// 将自定义 SKILL.md 写入全局技能库（QA G5 用户自定义编辑当前项目的 Skill）。
func (s *Server) handleSkillPackageWrite(w http.ResponseWriter, r *http.Request) {
	// 仅 UI（回环 + X-UI-Token 由识别层保证，此处二次校验，同 /api/permissions PUT）。
	if auth.ActorFrom(r.Context()).Class != auth.ClassUI {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "仅 App UI 可写自定义技能包", "")
		return
	}
	name := chi.URLParam(r, "name")
	var req skillPackageWriteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体必须为 JSON", "")
		return
	}
	pkg, err := s.skills.WriteUserPackage(r.Context(), name, req.Content)
	if err != nil {
		if isErr(err, skill.ErrInvalidPackage) {
			writeError(w, http.StatusUnprocessableEntity, "SKILL_INVALID", err.Error(), "")
			return
		}
		writeBizError(w, err)
		return
	}
	s.auditWrite(r, "skill.package_written", name, pkg.Version)
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": pkg})
}

// skillInstallReq 安装/卸载请求体。
type skillInstallReq struct {
	Host     string   `json:"host"`     // 宿主 key（QA-S1 矩阵）
	Packages []string `json:"packages"` // 技能包名列表（批量）
}

// handleSkillStatus GET /api/skills/status（skill.read）：宿主安装状态矩阵。
func (s *Server) handleSkillStatus(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	status, err := s.skills.Status(r.Context(), workdir)
	if err != nil {
		writeBizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": status})
}

// handleSkillInstall POST /api/skills/install（skill.install）：
// 将技能包复制到宿主约定位置（QA-S6），批量安装。
func (s *Server) handleSkillInstall(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	var req skillInstallReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体必须为 JSON", "")
		return
	}
	if req.Host == "" || len(req.Packages) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "TASK_INVALID", "host 与 packages 必填", "")
		return
	}
	if _, ok := skill.HostByKey(req.Host); !ok {
		writeError(w, http.StatusUnprocessableEntity, "SKILL_INVALID", "未知宿主: "+req.Host, "")
		return
	}
	results, err := s.skills.Install(r.Context(), workdir, req.Host, req.Packages)
	if err != nil {
		writeBizError(w, err)
		return
	}
	for _, res := range results {
		if res.Ok {
			s.auditWrite(r, "skill."+res.Action, res.Name, res.Host)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": results})
}

// handleSkillUninstall POST /api/skills/uninstall（skill.install）：卸载技能包。
func (s *Server) handleSkillUninstall(w http.ResponseWriter, r *http.Request) {
	workdir := projectFromRequest(r)
	var req skillInstallReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "TASK_INVALID", "请求体必须为 JSON", "")
		return
	}
	if req.Host == "" || len(req.Packages) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "TASK_INVALID", "host 与 packages 必填", "")
		return
	}
	if _, ok := skill.HostByKey(req.Host); !ok {
		writeError(w, http.StatusUnprocessableEntity, "SKILL_INVALID", "未知宿主: "+req.Host, "")
		return
	}
	results, err := s.skills.Uninstall(r.Context(), workdir, req.Host, req.Packages)
	if err != nil {
		writeBizError(w, err)
		return
	}
	for _, res := range results {
		if res.Ok {
			s.auditWrite(r, "skill."+res.Action, res.Name, res.Host)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": results})
}

// auditWrite 写审计（skill 域写操作统一入口）。
func (s *Server) auditWrite(r *http.Request, action, target, detail string) {
	actor := auth.ActorFrom(r.Context())
	s.audit.Write(r.Context(), projectFromRequest(r), audit.Entry{
		Actor: actor.Name, ActorClass: actor.Class,
		Action: action, Target: target, Result: audit.ResultOK, Detail: detail,
	})
}
