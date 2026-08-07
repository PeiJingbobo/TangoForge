package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tangoforge/internal/skill"
)

// writeUserSkill 向全局技能库（测试 Server 的临时 homeDir）写入自定义技能包。
// 通过 PUT /api/skills/packages/{name} 写入（需项目上下文，主业务组 X-Project），返回响应体。
func writeUserSkill(t *testing.T, srv *Server, dir, name, content string) string {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"content": content})
	rec := uiReq(t, srv, http.MethodPut, "/api/skills/packages/"+name, dir, string(payload))
	return mustCode(t, rec, http.StatusOK, "write skill")
}

func TestSkills_ListAndInfo(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// 内置包 taskboard-basic 在列表中（Skill 服务持有独立临时 homeDir）。
	rec := uiReq(t, srv, http.MethodGet, "/api/skills", dir, "")
	body := mustCode(t, rec, http.StatusOK, "skills list")
	var resp struct {
		Data []skill.Package `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if len(resp.Data) != 1 || resp.Data[0].Name != "taskboard-basic" {
		t.Fatalf("内置包列表不符: %+v", resp.Data)
	}
	if resp.Data[0].Source != "builtin" {
		t.Fatalf("source 应为 builtin: %+v", resp.Data[0])
	}

	// GET /api/skills/packages/taskboard-basic → 详情。
	rec = uiReq(t, srv, http.MethodGet, "/api/skills/packages/taskboard-basic", dir, "")
	body = mustCode(t, rec, http.StatusOK, "skill info")
	var info struct {
		Data skill.Package `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.Data.Name != "taskboard-basic" || info.Data.Version == "" || info.Data.Content == "" {
		t.Fatalf("info 不符: %+v", info.Data)
	}

	// 不存在 → 404 SKILL_NOT_FOUND。
	rec = uiReq(t, srv, http.MethodGet, "/api/skills/packages/no-such", dir, "")
	body = mustCode(t, rec, http.StatusNotFound, "skill not found")
	if apiCode(t, body) != "SKILL_NOT_FOUND" {
		t.Fatalf("code %s", apiCode(t, body))
	}
}

func TestSkills_AgentReadDefaultAllowed(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// agent（X-Actor）默认 skill.read=true → 200。
	rec := agentReq(t, srv, http.MethodGet, "/api/skills/packages", dir, "")
	mustCode(t, rec, http.StatusOK, "agent skill list")
}

func TestSkills_WriteCustomPackage(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// UI 写自定义包 → 200 + 出现在列表。
	content := "---\nname: my-skill\ndescription: 自定义技能\nversion: \"0.1.0\"\nhosts: [AGENTS.md]\nwhen_to_use: 自定义场景\n---\n# My Skill\n\n自定义正文\n"
	writeUserSkill(t, srv, dir, "my-skill", content)

	rec := uiReq(t, srv, http.MethodGet, "/api/skills/packages", dir, "")
	body := mustCode(t, rec, http.StatusOK, "skills list")
	var resp struct {
		Data []skill.Package `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("应有内置+自定义 2 个包, got %d", len(resp.Data))
	}
	found := false
	for _, p := range resp.Data {
		if p.Name == "my-skill" {
			found = true
			if p.Source != "user" || p.Version != "0.1.0" {
				t.Fatalf("自定义包字段: %+v", p)
			}
		}
	}
	if !found {
		t.Fatal("my-skill 未出现在列表")
	}

	// 自定义包覆盖同名内置（编辑语义）。
	writeUserSkill(t, srv, dir, "taskboard-basic", strings.Replace(content, "my-skill", "taskboard-basic", 1))
	rec = uiReq(t, srv, http.MethodGet, "/api/skills/packages/taskboard-basic", dir, "")
	body = mustCode(t, rec, http.StatusOK, "override info")
	var info struct {
		Data skill.Package `json:"data"`
	}
	_ = json.Unmarshal([]byte(body), &info)
	if info.Data.Source != "user" {
		t.Fatalf("自定义应覆盖内置: source=%s", info.Data.Source)
	}

	// 非法包（frontmatter name 与路径不一致）→ 422。
	bad := "---\nname: other-name\ndescription: x\nversion: \"1\"\n---\n正文\n"
	badPayload, _ := json.Marshal(map[string]string{"content": bad})
	rec = uiReq(t, srv, http.MethodPut, "/api/skills/packages/my-skill", dir, string(badPayload))
	body = mustCode(t, rec, http.StatusUnprocessableEntity, "invalid skill")
	if apiCode(t, body) != "SKILL_INVALID" {
		t.Fatalf("code %s", apiCode(t, body))
	}

	// Agent 写自定义包 → 403（仅 UI）。
	payload, _ := json.Marshal(map[string]string{"content": content})
	rec = agentReq(t, srv, http.MethodPut, "/api/skills/packages/my-skill", dir, string(payload))
	mustCode(t, rec, http.StatusForbidden, "agent write forbidden")
}

func TestSkills_InstallStatusUninstall(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// 初始状态：全部 missing。
	rec := uiReq(t, srv, http.MethodGet, "/api/skills/status", dir, "")
	body := mustCode(t, rec, http.StatusOK, "status")
	var statusResp struct {
		Data []skill.HostStatus `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &statusResp); err != nil {
		t.Fatalf("unmarshal status: %v body=%s", err, body)
	}
	status := statusResp.Data
	if len(status) != len(skill.Hosts) {
		t.Fatalf("宿主数 %d != %d", len(status), len(skill.Hosts))
	}
	allMissing := true
	for _, hs := range status {
		if len(hs.Installed) == 0 || hs.Installed[0].State != "missing" {
			allMissing = false
		}
	}
	if !allMissing {
		t.Fatalf("初始应全 missing: %+v", status)
	}

	// 安装 taskboard-basic 到 AGENTS.md（skill.install 权限：UI 放行）。
	rec = uiReq(t, srv, http.MethodPost, "/api/skills/install", dir,
		`{"host":"AGENTS.md","packages":["taskboard-basic"]}`)
	body = mustCode(t, rec, http.StatusOK, "install")
	var installResp struct {
		Data []skill.InstallResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &installResp); err != nil {
		t.Fatalf("unmarshal install: %v body=%s", err, body)
	}
	results := installResp.Data
	if len(results) != 1 || !results[0].Ok || results[0].Action != "install" {
		t.Fatalf("install 结果: %+v", results)
	}
	// 宿主文件已写入标记段。
	agentsPath := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(data), "tangoforge:skill:taskboard-basic:begin") {
		t.Fatalf("标记段缺失: %s", string(data))
	}

	// 再次安装 → update 语义。
	rec = uiReq(t, srv, http.MethodPost, "/api/skills/install", dir,
		`{"host":"AGENTS.md","packages":["taskboard-basic"]}`)
	body = mustCode(t, rec, http.StatusOK, "reinstall")
	var updateResp struct {
		Data []skill.InstallResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &updateResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	results = updateResp.Data
	if results[0].Action != "update" {
		t.Fatalf("重复安装应为 update: %+v", results[0])
	}

	// 状态 → current。
	rec = uiReq(t, srv, http.MethodGet, "/api/skills/status", dir, "")
	body = mustCode(t, rec, http.StatusOK, "status after install")
	_ = json.Unmarshal([]byte(body), &statusResp)
	status = statusResp.Data
	for _, hs := range status {
		if hs.Key == "AGENTS.md" {
			if hs.Installed[0].State != "current" {
				t.Fatalf("AGENTS.md 应为 current: %+v", hs.Installed)
			}
		}
	}

	// 卸载 → 文件移除标记段。
	rec = uiReq(t, srv, http.MethodPost, "/api/skills/uninstall", dir,
		`{"host":"AGENTS.md","packages":["taskboard-basic"]}`)
	body = mustCode(t, rec, http.StatusOK, "uninstall")
	var uninstallResp struct {
		Data []skill.InstallResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &uninstallResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !uninstallResp.Data[0].Ok {
		t.Fatalf("uninstall 结果: %+v", uninstallResp.Data[0])
	}
	if _, err := os.Stat(agentsPath); !os.IsNotExist(err) {
		t.Fatalf("卸载后宿主文件应被删除（仅含技能段）: %v", err)
	}

	// 未知宿主 → 422。
	rec = uiReq(t, srv, http.MethodPost, "/api/skills/install", dir,
		`{"host":"no-such","packages":["taskboard-basic"]}`)
	body = mustCode(t, rec, http.StatusUnprocessableEntity, "unknown host")
	if apiCode(t, body) != "SKILL_INVALID" {
		t.Fatalf("code %s", apiCode(t, body))
	}

	// Agent 安装（无 skill.install 权限，默认 false）→ 403。
	rec = agentReq(t, srv, http.MethodPost, "/api/skills/install", dir,
		`{"host":"AGENTS.md","packages":["taskboard-basic"]}`)
	mustCode(t, rec, http.StatusForbidden, "agent install forbidden")
}

func TestSkills_InstallToUserHost(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	// 安装到用户级宿主 user-claude（homeDir 为测试临时目录）。
	rec := uiReq(t, srv, http.MethodPost, "/api/skills/install", dir,
		`{"host":"user-claude","packages":["taskboard-basic"]}`)
	body := mustCode(t, rec, http.StatusOK, "install user host")
	var installResp struct {
		Data []skill.InstallResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &installResp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if !installResp.Data[0].Ok {
		t.Fatalf("install 结果: %+v", installResp.Data[0])
	}
	// 用户级位置：{homeDir}/.claude/skills/taskboard-basic/SKILL.md。
	home := srv.skills.HomeDir()
	installed := filepath.Join(home, ".claude", "skills", "taskboard-basic", "SKILL.md")
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("用户级安装文件缺失: %v", err)
	}
}
