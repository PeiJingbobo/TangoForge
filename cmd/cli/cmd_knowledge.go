package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// runKnowledge knowledge 子命令（TF-051，HTTP 等价操作）：
//
//	tangoforge knowledge bases|documents|search|read|link|unlink|relink|scan|edit --project <dir>
func runKnowledge(args []string, g cliGlobal) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: tangoforge knowledge <bases|documents|search|read|link|unlink|relink|scan|edit> [--project P]")
	}
	sub := args[0]
	rest := args[1:]
	c := newCLIClient(g)
	opts := parseFlags(rest)
	project, err := requireProject(opts["project"])
	if err != nil {
		return err
	}

	switch sub {
	case "bases":
		return runKnowledgeBases(rest, g, c, project)
	case "documents":
		return runKnowledgeDocuments(rest, g, c, project)
	case "search":
		return runKnowledgeSearch(rest, g, c, project)
	case "read":
		return runKnowledgeRead(rest, g, c, project)
	case "link":
		return runKnowledgeLink(rest, g, c, project)
	case "unlink":
		return runKnowledgeUnlink(rest, g, c, project)
	case "relink":
		return runKnowledgeRelink(rest, g, c, project)
	case "scan":
		resp, err := c.call("POST", "/api/knowledge/scan", project, nil)
		if err != nil {
			return err
		}
		printOutput(g, resp, func(data json.RawMessage) string { return "扫描完成: " + prettyJSON(data) })
		return nil
	case "edit":
		return runKnowledgeEdit(rest, g, c, project)
	}
	return fmt.Errorf("未知 knowledge 子命令: %s", sub)
}

// runKnowledgeBases 库列表（bases）。
func runKnowledgeBases(_ []string, g cliGlobal, c *cliClient, project string) error {
	resp, err := c.call("GET", "/api/knowledge/bases", project, nil)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return prettyJSON(data) })
	return nil
}

// runKnowledgeDocuments 文档列表（documents）。
func runKnowledgeDocuments(args []string, g cliGlobal, c *cliClient, project string) error {
	opts := parseFlags(args)
	q := ""
	if opts["kb_id"] != "" {
		q += "&filter[kb_id]=" + opts["kb_id"]
	}
	if opts["status"] != "" {
		q += "&filter[status]=" + opts["status"]
	}
	if opts["q"] != "" {
		q += "&q=" + opts["q"]
	}
	resp, err := c.call("GET", "/api/knowledge/documents?size=50"+q, project, nil)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return prettyJSON(data) })
	return nil
}

// runKnowledgeSearch 向量检索（search q=...）。
func runKnowledgeSearch(args []string, g cliGlobal, c *cliClient, project string) error {
	opts := parseFlags(args)
	if opts["q"] == "" {
		return fmt.Errorf("用法: tangoforge knowledge search --q <关键词> [--kb_id N] [--top_k N] --project P")
	}
	q := "?q=" + opts["q"]
	if opts["kb_id"] != "" {
		q += "&kb_id=" + opts["kb_id"]
	}
	if opts["top_k"] != "" {
		q += "&top_k=" + opts["top_k"]
	}
	resp, err := c.call("GET", "/api/knowledge/search"+q, project, nil)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return prettyJSON(data) })
	return nil
}

// runKnowledgeRead 文档详情/内容（read id=... [content=true]）。
func runKnowledgeRead(args []string, g cliGlobal, c *cliClient, project string) error {
	opts := parseFlags(args)
	if opts["id"] == "" {
		return fmt.Errorf("用法: tangoforge knowledge read --id <文档ID> [--content true] --project P")
	}
	resp, err := c.call("GET", "/api/knowledge/documents/"+opts["id"], project, nil)
	if err != nil {
		return err
	}
	if opts["content"] == "true" {
		resp, err = c.call("GET", "/api/knowledge/documents/"+opts["id"]+"/content", project, nil)
		if err != nil {
			return err
		}
	}
	printOutput(g, resp, func(data json.RawMessage) string { return prettyJSON(data) })
	return nil
}

// runKnowledgeLink 任务关联（link task_id=... document_id=... 或 path=...）。
func runKnowledgeLink(args []string, g cliGlobal, c *cliClient, project string) error {
	opts := parseFlags(args)
	if opts["task_id"] == "" {
		return fmt.Errorf("用法: tangoforge knowledge link --task_id <任务ID> --document_id <ID> 或 --path <路径> [--copy none|copy|auto] --project P")
	}
	if opts["document_id"] == "" && opts["path"] == "" {
		return fmt.Errorf("document_id 或 path 必填其一")
	}
	body := map[string]any{"task_id": opts["task_id"]}
	if opts["document_id"] != "" {
		body["document_id"] = opts["document_id"]
	}
	if opts["path"] != "" {
		body["path"] = opts["path"]
	}
	if opts["copy"] != "" {
		body["copy"] = opts["copy"]
	}
	resp, err := c.call("POST", "/api/knowledge/link", project, body)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return "已关联: " + prettyJSON(data) })
	return nil
}

// runKnowledgeUnlink 解除关联（unlink task_id=... document_id=...）。
func runKnowledgeUnlink(args []string, g cliGlobal, c *cliClient, project string) error {
	opts := parseFlags(args)
	if opts["task_id"] == "" || opts["document_id"] == "" {
		return fmt.Errorf("用法: tangoforge knowledge unlink --task_id <ID> --document_id <ID> --project P")
	}
	resp, err := c.call("POST", "/api/knowledge/unlink", project,
		map[string]any{"task_id": opts["task_id"], "document_id": opts["document_id"]})
	if err != nil {
		return err
	}
	printOutput(g, resp, func(_ json.RawMessage) string { return "已解除关联" })
	return nil
}

// runKnowledgeRelink 重新链接（relink id=... new_path=...）。
func runKnowledgeRelink(args []string, g cliGlobal, c *cliClient, project string) error {
	opts := parseFlags(args)
	if opts["id"] == "" || opts["new_path"] == "" {
		return fmt.Errorf("用法: tangoforge knowledge relink --id <ID> --new_path <路径> [--copy none|copy|auto] --project P")
	}
	body := map[string]any{"new_path": opts["new_path"]}
	if opts["copy"] != "" {
		body["copy"] = opts["copy"]
	}
	resp, err := c.call("POST", "/api/knowledge/documents/"+opts["id"]+"/relink", project, body)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return "已重新链接: " + prettyJSON(data) })
	return nil
}

// runKnowledgeEdit 编辑原文（edit id=... content=... 或 content=<文件>）。
func runKnowledgeEdit(args []string, g cliGlobal, c *cliClient, project string) error {
	opts := parseFlags(args)
	if opts["id"] == "" {
		return fmt.Errorf("用法: tangoforge knowledge edit --id <ID> --content <文本> 或 --content-file <路径> --project P")
	}
	content := opts["content"]
	if content == "" && opts["content-file"] != "" {
		data, err := os.ReadFile(opts["content-file"])
		if err != nil {
			return fmt.Errorf("读取内容文件: %w", err)
		}
		content = string(data)
	}
	if content == "" {
		return fmt.Errorf("content 或 content-file 必填")
	}
	resp, err := c.call("PUT", "/api/knowledge/documents/"+opts["id"]+"/content", project,
		map[string]any{"content": content})
	if err != nil {
		return err
	}
	printOutput(g, resp, func(_ json.RawMessage) string { return "已保存（触发重索引）" })
	return nil
}
