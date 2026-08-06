package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// runImport import 子命令：preview / drafts / confirm / discard。
func runImport(args []string, g cliGlobal) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: tangoforge import <preview|drafts|confirm|discard> ...")
	}
	c := newCLIClient(g)
	switch args[0] {
	case "preview":
		opts := parseFlags(args[1:])
		project := opts["project"]
		if err := requireProjectFlag(project); err != nil {
			return err
		}
		body := map[string]string{}
		if f := opts["file"]; f != "" {
			body["file_path"] = f
		} else {
			content := opts["content"]
			source := opts["source-file"]
			if source == "" {
				return fmt.Errorf("用法: tangoforge import preview [--project P] (--file F | --content C --source-file S)")
			}
			body["content"] = content
			body["source_file"] = source
		}
		resp, err := c.call("POST", "/api/import", project, body)
		if err != nil {
			return err
		}
		printOutput(g, resp, func(data json.RawMessage) string { return "草稿已生成: " + prettyJSON(data) })
		return nil
	case "drafts":
		opts := parseFlags(args[1:])
		project := opts["project"]
		if err := requireProjectFlag(project); err != nil {
			return err
		}
		resp, err := c.call("GET", "/api/import/drafts", project, nil)
		if err != nil {
			return err
		}
		printOutput(g, resp, func(data json.RawMessage) string { return prettyJSON(data) })
		return nil
	case "confirm":
		if len(args) < 2 {
			return fmt.Errorf("用法: tangoforge import confirm <draft-id> [--project P]")
		}
		opts := parseFlags(args[2:])
		project := opts["project"]
		if err := requireProjectFlag(project); err != nil {
			return err
		}
		resp, err := c.call("POST", "/api/import/drafts/"+args[1]+"/confirm", project, nil)
		if err != nil {
			return err
		}
		printOutput(g, resp, func(data json.RawMessage) string { return "已确认入库: " + prettyJSON(data) })
		return nil
	case "discard":
		if len(args) < 2 {
			return fmt.Errorf("用法: tangoforge import discard <draft-id> [--project P]")
		}
		opts := parseFlags(args[2:])
		project := opts["project"]
		if err := requireProjectFlag(project); err != nil {
			return err
		}
		resp, err := c.call("DELETE", "/api/import/drafts/"+args[1], project, nil)
		if err != nil {
			return err
		}
		printOutput(g, resp, func(data json.RawMessage) string { return "草稿已丢弃" })
		return nil
	}
	return fmt.Errorf("未知 import 子命令: %s", args[0])
}

// runExport export 子命令：run（默认）/ template。
func runExport(args []string, g cliGlobal) error {
	c := newCLIClient(g)
	// 子命令：export [run] <flags> 或 export template <示例文件> [--project P]。
	if len(args) > 0 && args[0] == "template" {
		if len(args) < 2 {
			return fmt.Errorf("用法: tangoforge export template <示例文件> [--project P]")
		}
		opts := parseFlags(args[2:])
		project := opts["project"]
		if err := requireProjectFlag(project); err != nil {
			return err
		}
		data, err := os.ReadFile(args[1])
		if err != nil {
			return fmt.Errorf("读取示例文件: %w", err)
		}
		resp, err := c.call("POST", "/api/export/template/generate", project, map[string]string{"example": string(data)})
		if err != nil {
			return err
		}
		printOutput(g, resp, func(data json.RawMessage) string {
			return "LLM 模板已生成并写入项目配置: " + prettyJSON(data)
		})
		return nil
	}
	// export [run]。
	rest := args
	if len(rest) > 0 && rest[0] == "run" {
		rest = rest[1:]
	}
	opts := parseFlags(rest)
	project := opts["project"]
	if err := requireProjectFlag(project); err != nil {
		return err
	}
	body := map[string]string{}
	if v := opts["template-mode"]; v != "" {
		body["template_mode"] = v
	}
	if v := opts["target"]; v != "" {
		body["target"] = v
	}
	if v := opts["path"]; v != "" {
		body["path"] = v
	}
	resp, err := c.call("POST", "/api/export", project, body)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return "已导出: " + prettyJSON(data) })
	return nil
}
