package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKnowledgeTools_FullLifecycle(t *testing.T) {
	deps, dir := newTestDeps(t)
	srv := NewServer(deps)
	c := newStdioClient(t, srv)
	initialize(t, c)

	// 0. 授权 task.create（MCP agent 默认 denied）并建任务。
	if err := grantAction(t, deps, dir, "task.create", true); err != nil {
		t.Fatal(err)
	}
	taskResp := callTool(t, c, "task_create", map[string]any{"project": dir, "title": "知识库任务"})
	if err := toolErr(taskResp); err != "" {
		t.Fatalf("task_create 失败: %s", err)
	}
	var taskData struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(resultText(t, taskResp)), &taskData)

	// 1. 写候选文件。
	docPath := filepath.Join(dir, "kb.md")
	if err := os.WriteFile(docPath, []byte("# 知识库文档内容\n接口说明"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. knowledge_list（读权限默认 true）。
	resp := callTool(t, c, "knowledge_list", map[string]any{"project": dir})
	if err := toolErr(resp); err != "" {
		t.Fatalf("knowledge_list: %s", err)
	}
	if !strings.Contains(resultText(t, resp), "默认库") {
		t.Fatalf("应含默认库: %s", resultText(t, resp))
	}

	// 3. knowledge_link（write 默认 false → denied）。
	resp = callTool(t, c, "knowledge_link", map[string]any{
		"project": dir, "task_id": taskData.Data.ID, "path": docPath,
	})
	if err := toolErr(resp); err == "" {
		t.Fatalf("write 默认应 denied")
	}
	if !strings.Contains(toolErr(resp), "PERMISSION_DENIED") {
		t.Fatalf("应 PERMISSION_DENIED: %s", toolErr(resp))
	}

	// 4. 授权 knowledge.write → 重试成功。
	if err := grantAction(t, deps, dir, "knowledge.write", true); err != nil {
		t.Fatal(err)
	}
	resp = callTool(t, c, "knowledge_link", map[string]any{
		"project": dir, "task_id": taskData.Data.ID, "path": docPath,
	})
	if err := toolErr(resp); err != "" {
		t.Fatalf("knowledge_link: %s", err)
	}

	// 5. knowledge_read 详情 + content。
	var docID string
	docs := callTool(t, c, "knowledge_list", map[string]any{"project": dir})
	if err := toolErr(docs); err != "" {
		t.Fatalf("list: %s", err)
	}
	var docList struct {
		Data struct {
			Documents []struct {
				ID string `json:"id"`
			} `json:"documents"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(resultText(t, docs)), &docList)
	if len(docList.Data.Documents) != 1 {
		t.Fatalf("应 1 文档: %s", resultText(t, docs))
	}
	docID = docList.Data.Documents[0].ID

	resp = callTool(t, c, "knowledge_read", map[string]any{"project": dir, "id": docID, "content": "true"})
	if err := toolErr(resp); err != "" {
		t.Fatalf("read: %s", err)
	}
	if !strings.Contains(resultText(t, resp), "知识库文档内容") {
		t.Fatalf("read 应含内容: %s", resultText(t, resp))
	}

	// 6. knowledge_edit（写盘）。
	resp = callTool(t, c, "knowledge_edit", map[string]any{
		"project": dir, "id": docID, "content": "# 更新后内容",
	})
	if err := toolErr(resp); err != "" {
		t.Fatalf("edit: %s", err)
	}
	data, _ := os.ReadFile(docPath)
	if !strings.Contains(string(data), "更新后内容") {
		t.Fatalf("edit 应写盘: %s", string(data))
	}

	// 7. knowledge_relink。
	newPath := filepath.Join(dir, "relinked.md")
	if err := os.WriteFile(newPath, []byte("# 新路径"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp = callTool(t, c, "knowledge_relink", map[string]any{
		"project": dir, "id": docID, "new_path": newPath,
	})
	if err := toolErr(resp); err != "" {
		t.Fatalf("relink: %s", err)
	}
	if !strings.Contains(resultText(t, resp), "relinked.md") {
		t.Fatalf("relink 应更新路径: %s", resultText(t, resp))
	}

	// 8. knowledge_unlink + 文档保留。
	resp = callTool(t, c, "knowledge_unlink", map[string]any{
		"project": dir, "task_id": taskData.Data.ID, "document_id": docID,
	})
	if err := toolErr(resp); err != "" {
		t.Fatalf("unlink: %s", err)
	}
	resp = callTool(t, c, "knowledge_read", map[string]any{"project": dir, "id": docID})
	if err := toolErr(resp); err != "" {
		t.Fatalf("unlink 后文档应保留: %s", err)
	}

	// 9. knowledge_search（未配置 embedding → 422 NOT_CONFIGURED）。
	resp = callTool(t, c, "knowledge_search", map[string]any{"project": dir, "q": "接口"})
	if err := toolErr(resp); err == "" {
		t.Fatalf("未配置 embedding 应报错")
	}
	if !strings.Contains(toolErr(resp), "EMBEDDING_NOT_CONFIGURED") {
		t.Fatalf("应 NOT_CONFIGURED: %s", toolErr(resp))
	}

	// 10. knowledge_scan（scanner nil → INDEX_FAILED）。
	resp = callTool(t, c, "knowledge_scan", map[string]any{"project": dir})
	if err := toolErr(resp); err == "" {
		t.Fatalf("scanner nil 应报错")
	}
}

func TestKnowledgeTools_ListContainsAll(t *testing.T) {
	deps, _ := newTestDeps(t)
	srv := NewServer(deps)
	c := newStdioClient(t, srv)
	initialize(t, c)
	resp := c.send(t, 200, "tools/list", nil)
	list, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list 失败: %v", resp)
	}
	tools, _ := list["tools"].([]any)
	var names []string
	for _, tl := range tools {
		m, _ := tl.(map[string]any)
		if n, ok := m["name"].(string); ok {
			names = append(names, n)
		}
	}
	want := []string{
		"knowledge_list", "knowledge_search", "knowledge_read",
		"knowledge_link", "knowledge_unlink", "knowledge_relink",
		"knowledge_scan", "knowledge_edit",
	}
	for _, w := range want {
		found := false
		for _, n := range names {
			if n == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("缺少工具 %s，全部: %v", w, names)
		}
	}
}

// grantAction 直接改权限表（测试辅助）。
func grantAction(t *testing.T, deps Deps, workdir, action string, allowed bool) error {
	t.Helper()
	perms, err := deps.Perms.Get(context.Background(), workdir)
	if err != nil {
		return err
	}
	perms[action] = allowed
	_, err = deps.Perms.Set(context.Background(), workdir, perms)
	return err
}

// toolErr 提取工具错误码（isError 时）；成功返回空串。
func toolErr(result map[string]any) string {
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		return ""
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := first["text"].(string)
	var parsed struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(text), &parsed)
	return parsed.Code
}
