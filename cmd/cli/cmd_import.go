package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// collectRepeatFlag 收集重复出现的同名单值 flag（如多个 --file a.md --file b.md）。
func collectRepeatFlag(args []string, name string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) && !hasFlagPrefix(args[i+1]) {
			out = append(out, args[i+1])
			i++
		} else if strings.HasPrefix(args[i], name+"=") {
			out = append(out, strings.TrimPrefix(args[i], name+"="))
		}
	}
	return out
}

// hasFlagPrefix 判断参数是否为 flag。
func hasFlagPrefix(s string) bool {
	return strings.HasPrefix(s, "--")
}

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
		var err error
		if project, err = requireProject(project); err != nil {
			return err
		}
		// 支持：--file F（可多个，多文件合并一次解析）| --dir D（递归扫描目录）| --content C --source-file S。
		files := collectRepeatFlag(args[1:], "--file")
		body := map[string]any{}
		switch {
		case opts["dir"] != "":
			body["directory"] = opts["dir"]
		case len(files) > 1:
			body["file_paths"] = files
		case len(files) == 1:
			body["file_path"] = files[0]
		case opts["content"] != "" || opts["source-file"] != "":
			if opts["source-file"] == "" {
				return fmt.Errorf("用法: tangoforge import preview [--project P] (--file F... | --dir D | --content C --source-file S)")
			}
			body["content"] = opts["content"]
			body["source_file"] = opts["source-file"]
		default:
			return fmt.Errorf("用法: tangoforge import preview [--project P] (--file F... | --dir D | --content C --source-file S)")
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
		var err error
		if project, err = requireProject(project); err != nil {
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
		var err error
		if project, err = requireProject(project); err != nil {
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
		var err error
		if project, err = requireProject(project); err != nil {
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
		var err error
		if project, err = requireProject(project); err != nil {
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
	var err error
	if project, err = requireProject(project); err != nil {
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
