package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"tangoforge/internal/skill"
)

// writeSkillFile 向项目 skills/ 目录写入 Skill 文件。
func writeSkillFile(t *testing.T, workdir, name, content string) {
	t.Helper()
	dir := filepath.Join(workdir, ".taskboard", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestSkills_ListAndInfo(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)
	writeSkillFile(t, dir, "basic.yaml", "name: taskboard-basic\nversion: 1\ndescription: 基础指引\ninstructions: 使用 task_read\n")
	writeSkillFile(t, dir, "guide.md", "# 导入指南\n\n草稿流说明\n")

	// GET /api/skills → 列表。
	rec := uiReq(t, srv, http.MethodGet, "/api/skills", dir, "")
	body := mustCode(t, rec, http.StatusOK, "skills list")
	var resp struct {
		Data []skill.Skill `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("skills 数量 %d != 2", len(resp.Data))
	}

	// GET /api/skills/:name → 详情。
	rec = uiReq(t, srv, http.MethodGet, "/api/skills/taskboard-basic", dir, "")
	body = mustCode(t, rec, http.StatusOK, "skill info")
	var info struct {
		Data skill.Skill `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.Data.Name != "taskboard-basic" || info.Data.Version != "1" || info.Data.Instructions != "使用 task_read" {
		t.Fatalf("info 不符: %+v", info.Data)
	}
	if info.Data.Content == "" {
		t.Fatal("content 应为原文")
	}

	// 不存在 → 404 SKILL_NOT_FOUND。
	rec = uiReq(t, srv, http.MethodGet, "/api/skills/no-such", dir, "")
	body = mustCode(t, rec, http.StatusNotFound, "skill not found")
	if apiCode(t, body) != "SKILL_NOT_FOUND" {
		t.Fatalf("code %s", apiCode(t, body))
	}
}

func TestSkills_AgentReadDefaultAllowed(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)
	writeSkillFile(t, dir, "a.yaml", "name: skill-a\n")

	// agent（X-Actor）默认 skill.read=true → 200。
	rec := agentReq(t, srv, http.MethodGet, "/api/skills", dir, "")
	mustCode(t, rec, http.StatusOK, "agent skill list")
}
