package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ---- tasks 子命令 ----

// runTasks tasks 子命令分发：list / get / create / update / status / archive / restore / delete。
func runTasks(args []string, g cliGlobal) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: tangoforge tasks <list|get|create|update|status|archive|restore|delete> ...")
	}
	c := newCLIClient(g)
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return cmdTaskList(rest, g, c)
	case "get":
		return cmdTaskGet(rest, g, c)
	case "create":
		return cmdTaskCreate(rest, g, c)
	case "update":
		return cmdTaskUpdate(rest, g, c)
	case "status":
		return cmdTaskStatus(rest, g, c)
	case "archive":
		return cmdTaskArchive(rest, g, c)
	case "restore":
		return cmdTaskRestore(rest, g, c)
	case "delete":
		return cmdTaskDelete(rest, g, c)
	}
	return fmt.Errorf("未知 tasks 子命令: %s", sub)
}

func cmdTaskList(args []string, g cliGlobal, c *cliClient) error {
	opts := parseFlags(args)
	project := opts["project"]
	if err := requireProjectFlag(project); err != nil {
		return err
	}
	q := ""
	if opts["q"] != "" {
		q = "&q=" + opts["q"]
	}
	status := ""
	if opts["status"] != "" {
		status = "&filter[status]=" + opts["status"]
	}
	resp, err := c.call("GET", "/api/tasks?page=0"+status+q, project, nil)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return prettyJSON(data) })
	return nil
}

func cmdTaskGet(args []string, g cliGlobal, c *cliClient) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: tangoforge tasks get <id> [--project P]")
	}
	opts := parseFlags(args[1:])
	project := opts["project"]
	if err := requireProjectFlag(project); err != nil {
		return err
	}
	resp, err := c.call("GET", "/api/tasks/"+args[0], project, nil)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return prettyJSON(data) })
	return nil
}

func cmdTaskCreate(args []string, g cliGlobal, c *cliClient) error {
	opts := parseFlags(args)
	project := opts["project"]
	if err := requireProjectFlag(project); err != nil {
		return err
	}
	// title 为位置参数（第一个非 -- 值），也可用 --title 显式指定。
	title := opts["title"]
	for _, a := range args {
		if !strings.HasPrefix(a, "--") {
			title = a
			break
		}
	}
	if title == "" {
		return fmt.Errorf("用法: tangoforge tasks create <title> [--project P] [--status S] [--priority P] [--tags a,b] [--assignee X] [--parent ID] [--depends a,b]")
	}
	body := map[string]any{"title": title}
	if v := opts["status"]; v != "" {
		body["status"] = v
	}
	if v := opts["priority"]; v != "" {
		body["priority"] = parseNumOrStr(v)
	}
	if v := opts["tags"]; v != "" {
		body["tags"] = strings.Split(v, ",")
	}
	if v := opts["assignee"]; v != "" {
		body["assignee"] = v
	}
	if v := opts["parent"]; v != "" {
		body["parent_id"] = v
	}
	if v := opts["depends"]; v != "" {
		body["depends_on"] = strings.Split(v, ",")
	}
	resp, err := c.call("POST", "/api/tasks", project, body)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string {
		return "已创建任务: " + prettyJSON(data)
	})
	return nil
}

func cmdTaskUpdate(args []string, g cliGlobal, c *cliClient) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: tangoforge tasks update <id> [--project P] [--title T] [--priority P] [--tags a,b] [--assignee X] [--parent ID|\"\"] [--depends a,b]")
	}
	opts := parseFlags(args[1:])
	project := opts["project"]
	if err := requireProjectFlag(project); err != nil {
		return err
	}
	body := map[string]any{}
	if v, ok := opts["title"]; ok {
		body["title"] = v
	}
	if v, ok := opts["priority"]; ok {
		body["priority"] = parseNumOrStr(v)
	}
	if v, ok := opts["tags"]; ok {
		body["tags"] = strings.Split(v, ",")
	}
	if v, ok := opts["assignee"]; ok {
		body["assignee"] = v
	}
	if v, ok := opts["parent"]; ok {
		if v == "" {
			body["parent_id"] = nil // 置顶
		} else {
			body["parent_id"] = v
		}
	}
	if v, ok := opts["depends"]; ok {
		body["depends_on"] = strings.Split(v, ",")
	}
	if len(body) == 0 {
		return fmt.Errorf("至少提供一个更新字段")
	}
	resp, err := c.call("PATCH", "/api/tasks/"+args[0], project, body)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return "已更新: " + prettyJSON(data) })
	return nil
}

func cmdTaskStatus(args []string, g cliGlobal, c *cliClient) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: tangoforge tasks status <id> <目标状态> [--project P]")
	}
	opts := parseFlags(args[2:])
	project := opts["project"]
	if err := requireProjectFlag(project); err != nil {
		return err
	}
	resp, err := c.call("PATCH", "/api/tasks/"+args[0], project, map[string]any{"status": args[1]})
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return "状态已流转: " + prettyJSON(data) })
	return nil
}

func cmdTaskArchive(args []string, g cliGlobal, c *cliClient) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: tangoforge tasks archive <id> [--project P]")
	}
	opts := parseFlags(args[1:])
	project := opts["project"]
	if err := requireProjectFlag(project); err != nil {
		return err
	}
	resp, err := c.call("POST", "/api/tasks/"+args[0]+"/archive", project, nil)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return "已归档: " + prettyJSON(data) })
	return nil
}

func cmdTaskRestore(args []string, g cliGlobal, c *cliClient) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: tangoforge tasks restore <id> [--project P] [--fallback-todo]")
	}
	opts := parseFlags(args[1:])
	project := opts["project"]
	if err := requireProjectFlag(project); err != nil {
		return err
	}
	body := map[string]any{"fallback_todo": opts["fallback-todo"] == "true"}
	resp, err := c.call("POST", "/api/tasks/"+args[0]+"/restore", project, body)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return "已还原: " + prettyJSON(data) })
	return nil
}

func cmdTaskDelete(args []string, g cliGlobal, c *cliClient) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: tangoforge tasks delete <id> [--project P]")
	}
	opts := parseFlags(args[1:])
	project := opts["project"]
	if err := requireProjectFlag(project); err != nil {
		return err
	}
	resp, err := c.call("DELETE", "/api/tasks/"+args[0], project, nil)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return "已物理删除: " + prettyJSON(data) })
	return nil
}

// ---- projects 子命令 ----

// runProjects projects 子命令分发：list / import / remove。
func runProjects(args []string, g cliGlobal) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: tangoforge projects <list|import|remove> ...")
	}
	c := newCLIClient(g)
	switch args[0] {
	case "list":
		resp, err := c.call("GET", "/api/projects/", "", nil)
		if err != nil {
			return err
		}
		printOutput(g, resp, func(data json.RawMessage) string { return prettyJSON(data) })
		return nil
	case "import":
		if len(args) < 2 {
			return fmt.Errorf("用法: tangoforge projects import <工作目录>")
		}
		resp, err := c.call("POST", "/api/projects/import", "", map[string]string{"workdir": args[1]})
		if err != nil {
			return err
		}
		printOutput(g, resp, func(data json.RawMessage) string { return "已导入项目: " + prettyJSON(data) })
		return nil
	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("用法: tangoforge projects remove <id>（仅 UI 可执行，CLI 为 agent 身份将被拒绝）")
		}
		resp, err := c.call("DELETE", "/api/projects/"+args[1], "", nil)
		if err != nil {
			return err
		}
		printOutput(g, resp, func(data json.RawMessage) string { return "已移除注册记录（磁盘数据保留）" })
		return nil
	}
	return fmt.Errorf("未知 projects 子命令: %s", args[0])
}

// ---- 辅助 ----

// parseFlags 解析 --key value 风格参数（含布尔 --flag true）。
func parseFlags(args []string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			key := strings.TrimPrefix(a, "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				out[key] = args[i+1]
				i++
			} else {
				out[key] = "true"
			}
		}
	}
	return out
}

// parseNumOrStr priority 参数：数字字符串保持字符串（服务端归一化），原样返回。
func parseNumOrStr(v string) any {
	return v
}

// prettyJSON 格式化 JSON 文本。
func prettyJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}
