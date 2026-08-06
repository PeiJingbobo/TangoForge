package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jsonUnmarshal 解析 JSON 文本（resultText 输出为 JSON 文本）。
func jsonUnmarshal(data string, v any) error {
	return json.Unmarshal([]byte(data), v)
}

// 工具全集（v1 固定工具集 + QA P4-1 Q6 扩展），tools/list 应返回全部。
func TestTools_ListComplete(t *testing.T) {
	deps, _ := newTestDeps(t)
	srv := NewServer(deps)
	c := newStdioClient(t, srv)
	initialize(t, c)

	resp := c.send(t, 2, "tools/list", nil)
	result := resp["result"].(map[string]any)
	tools := result["tools"].([]any)
	names := make(map[string]bool, len(tools))
	for _, tl := range tools {
		names[tl.(map[string]any)["name"].(string)] = true
	}
	want := []string{
		"project_list", "project_import", "project_init", "project_create",
		"task_read", "task_list", "task_create", "task_update", "task_archive", "task_restore",
		"import_preview", "import_confirm", "import_discard",
		"export_markdown",
		"graph_get",
		"state_machine_get", "state_machine_update",
		"skill_info",
		"permission_list",
	}
	for _, name := range want {
		if !names[name] {
			t.Fatalf("缺少工具 %s，当前: %v", name, names)
		}
	}
	if len(tools) != len(want) {
		t.Fatalf("工具数 %d != %d", len(tools), len(want))
	}
}

// TestTools_ProjectCreateAndTaskLifecycle：project_create → 授权 → 任务全生命周期 → graph/状态机/导出。
func TestTools_ProjectCreateAndTaskLifecycle(t *testing.T) {
	deps, _ := newTestDeps(t)
	srv := NewServer(deps)
	c := newStdioClient(t, srv)
	initialize(t, c)

	// project_create：新目录先 init 后 import。
	dir := t.TempDir()
	res := callTool(t, c, "project_create", map[string]any{"project": dir})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("project_create 失败: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), dir) {
		t.Fatalf("project_create 应返回注册记录: %s", resultText(t, res))
	}

	// project_init 幂等（已初始化）。
	res = callTool(t, c, "project_init", map[string]any{"project": dir})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("project_init 幂等失败: %s", resultText(t, res))
	}

	// project_import 幂等（已注册）。
	res = callTool(t, c, "project_import", map[string]any{"project": dir})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("project_import 幂等失败: %s", resultText(t, res))
	}

	// project_list 全局。
	res = callTool(t, c, "project_list", map[string]any{})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("project_list 失败: %s", resultText(t, res))
	}

	// 授权（Set 为全量覆盖：先取当前全量再合并）。
	ctx := t.Context()
	base, err := deps.Perms.Get(ctx, dir)
	if err != nil {
		t.Fatalf("读取权限: %v", err)
	}
	for _, a := range []string{"task.create", "task.update", "task.delete", "task.restore", "import.run", "import.confirm", "export.run", "state_machine.read", "state_machine.write"} {
		base[a] = true
	}
	if _, err := deps.Perms.Set(ctx, dir, base); err != nil {
		t.Fatalf("授权: %v", err)
	}

	// task_create。
	res = callTool(t, c, "task_create", map[string]any{"project": dir, "title": "MCP 任务", "priority": "high", "tags": []any{"mcp"}})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("task_create: %s", resultText(t, res))
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := jsonUnmarshal(resultText(t, res), &created); err != nil {
		t.Fatalf("解析创建结果: %v", err)
	}
	taskID := created.Data.ID
	if taskID == "" {
		t.Fatal("task ID 为空")
	}

	// task_read（详情）。
	res = callTool(t, c, "task_read", map[string]any{"project": dir, "id": taskID})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("task_read: %s", resultText(t, res))
	}

	// task_list（树）。
	res = callTool(t, c, "task_list", map[string]any{"project": dir})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("task_list: %s", resultText(t, res))
	}

	// task_update（title）。
	res = callTool(t, c, "task_update", map[string]any{"project": dir, "id": taskID, "title": "MCP 任务改"})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("task_update: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "MCP 任务改") {
		t.Fatalf("task_update 未生效: %s", resultText(t, res))
	}

	// task_archive。
	res = callTool(t, c, "task_archive", map[string]any{"project": dir, "id": taskID})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("task_archive: %s", resultText(t, res))
	}

	// task_restore。
	res = callTool(t, c, "task_restore", map[string]any{"project": dir, "id": taskID})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("task_restore: %s", resultText(t, res))
	}

	// graph_get。
	res = callTool(t, c, "graph_get", map[string]any{"project": dir})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("graph_get: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), taskID) {
		t.Fatalf("graph_get 应含任务: %s", resultText(t, res))
	}

	// state_machine_get。
	res = callTool(t, c, "state_machine_get", map[string]any{"project": dir})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("state_machine_get: %s", resultText(t, res))
	}

	// state_machine_update。
	sm := map[string]any{
		"states":      []map[string]any{{"key": "todo", "label": "待办"}, {"key": "done", "label": "完成"}},
		"transitions": []map[string]any{{"from": "todo", "to": []string{"done"}}},
	}
	res = callTool(t, c, "state_machine_update", map[string]any{"project": dir, "state_machine": sm})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("state_machine_update: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "done") {
		t.Fatalf("状态机更新未生效: %s", resultText(t, res))
	}

	// permission_list。
	res = callTool(t, c, "permission_list", map[string]any{"project": dir})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("permission_list: %s", resultText(t, res))
	}

	// import_preview（mock LLM）→ import_confirm。
	res = callTool(t, c, "import_preview", map[string]any{"project": dir, "content": "# 文档\n## 任务A\n", "source_file": "docs/a.md"})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("import_preview: %s", resultText(t, res))
	}
	var draftResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := jsonUnmarshal(resultText(t, res), &draftResp); err != nil || draftResp.Data.ID == "" {
		t.Fatalf("import_preview 结果: %v %s", err, resultText(t, res))
	}
	res = callTool(t, c, "import_confirm", map[string]any{"project": dir, "draft_id": draftResp.Data.ID})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("import_confirm: %s", resultText(t, res))
	}

	// export_markdown。
	res = callTool(t, c, "export_markdown", map[string]any{"project": dir, "target": "copy"})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("export_markdown: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "MCP 任务改") {
		t.Fatalf("导出内容应含任务: %s", resultText(t, res))
	}
}

// TestTools_ProjectImportUninitialized：project_import 对未初始化目录报错。
func TestTools_ProjectImportUninitialized(t *testing.T) {
	deps, _ := newTestDeps(t)
	srv := NewServer(deps)
	c := newStdioClient(t, srv)
	initialize(t, c)

	res := callTool(t, c, "project_import", map[string]any{"project": t.TempDir()})
	if isErr, _ := res["isError"].(bool); !isErr {
		t.Fatalf("未初始化目录 import 应报错: %s", resultText(t, res))
	}
}

// TestTools_SkillInfo：skill_info 工具。
func TestTools_SkillInfo(t *testing.T) {
	deps, dir := newTestDeps(t)
	// 写一个 skill 文件。
	skillsDir := filepath.Join(dir, ".taskboard", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "basic.yaml"), []byte("name: taskboard-basic\ninstructions: 使用 task_read\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(deps)
	c := newStdioClient(t, srv)
	initialize(t, c)

	res := callTool(t, c, "skill_info", map[string]any{"project": dir, "name": "taskboard-basic"})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("skill_info: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "taskboard-basic") {
		t.Fatalf("skill_info 内容: %s", resultText(t, res))
	}
}

// TestTools_DeniedPaths：未授权工具返回 PERMISSION_DENIED。
func TestTools_DeniedPaths(t *testing.T) {
	deps, dir := newTestDeps(t)
	srv := NewServer(deps)
	c := newStdioClient(t, srv)
	initialize(t, c)

	// 默认权限 task.update=false → denied。
	res := callTool(t, c, "task_update", map[string]any{"project": dir, "id": "x", "title": "y"})
	if isErr, _ := res["isError"].(bool); !isErr {
		t.Fatalf("应拒绝 task_update: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "PERMISSION_DENIED") {
		t.Fatalf("错误码: %s", resultText(t, res))
	}

	// state_machine.write 默认 false → denied。
	res = callTool(t, c, "state_machine_update", map[string]any{
		"project": dir,
		"state_machine": map[string]any{
			"states": []map[string]any{{"key": "todo"}},
		},
	})
	if isErr, _ := res["isError"].(bool); !isErr {
		t.Fatalf("应拒绝 state_machine_update: %s", resultText(t, res))
	}
}
