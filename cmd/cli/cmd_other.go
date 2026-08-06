package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// runGraph graph 子命令（全景图数据）。
func runGraph(args []string, g cliGlobal) error {
	opts := parseFlags(args)
	project := opts["project"]
	var err error
	if project, err = requireProject(project); err != nil {
		return err
	}
	c := newCLIClient(g)
	resp, err := c.call("GET", "/api/graph", project, nil)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string {
		var graph struct {
			Nodes []map[string]any `json:"nodes"`
			Edges []map[string]any `json:"edges"`
		}
		_ = json.Unmarshal(data, &graph)
		return fmt.Sprintf("节点 %d 个 / 边 %d 条（--json 查看全量）", len(graph.Nodes), len(graph.Edges))
	})
	return nil
}

// runStateMachine state-machine 子命令：get / update。
func runStateMachine(args []string, g cliGlobal) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: tangoforge state-machine <get|update> ...")
	}
	c := newCLIClient(g)
	switch args[0] {
	case "get":
		opts := parseFlags(args[1:])
		project := opts["project"]
		var err error
		if project, err = requireProject(project); err != nil {
			return err
		}
		resp, err := c.call("GET", "/api/state-machine", project, nil)
		if err != nil {
			return err
		}
		printOutput(g, resp, func(data json.RawMessage) string { return prettyJSON(data) })
		return nil
	case "update":
		if len(args) < 2 {
			return fmt.Errorf("用法: tangoforge state-machine update <状态机文件.json> [--project P]")
		}
		opts := parseFlags(args[2:])
		project := opts["project"]
		var err error
		if project, err = requireProject(project); err != nil {
			return err
		}
		data, err := os.ReadFile(args[1])
		if err != nil {
			return fmt.Errorf("读取状态机文件: %w", err)
		}
		var sm map[string]any
		if err := json.Unmarshal(data, &sm); err != nil {
			return fmt.Errorf("状态机文件必须为 JSON（states + transitions）: %w", err)
		}
		resp, err := c.call("PUT", "/api/state-machine", project, sm)
		if err != nil {
			return err
		}
		printOutput(g, resp, func(data json.RawMessage) string { return "状态机已更新: " + prettyJSON(data) })
		return nil
	}
	return fmt.Errorf("未知 state-machine 子命令: %s", args[0])
}

// runSkills skills 子命令：list（默认）/ info。
func runSkills(args []string, g cliGlobal) error {
	c := newCLIClient(g)
	project := ""
	name := ""
	if len(args) > 0 && args[0] == "info" {
		if len(args) < 2 {
			return fmt.Errorf("用法: tangoforge skills info <name> [--project P]")
		}
		name = args[1]
		project = parseFlags(args[2:])["project"]
	} else {
		project = parseFlags(args)["project"]
	}
	var err error
	if project, err = requireProject(project); err != nil {
		return err
	}
	var resp *apiResp
	if name != "" {
		resp, err = c.call("GET", "/api/skills/"+name, project, nil)
	} else {
		resp, err = c.call("GET", "/api/skills", project, nil)
	}
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return prettyJSON(data) })
	return nil
}

// runPermission permission 子命令（查询自身权限范围）。
func runPermission(args []string, g cliGlobal) error {
	opts := parseFlags(args)
	project := opts["project"]
	var err error
	if project, err = requireProject(project); err != nil {
		return err
	}
	c := newCLIClient(g)
	resp, err := c.call("GET", "/api/permissions", project, nil)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string {
		var perms map[string]bool
		_ = json.Unmarshal(data, &perms)
		out := "Agent 权限范围：\n"
		for _, a := range []string{
			"project.read", "task.read", "task.create", "task.update", "task.update_status",
			"task.delete", "task.restore", "import.run", "import.confirm", "export.run",
			"graph.read", "skill.read", "state_machine.read", "state_machine.write",
			"audit.read", "permission.read",
		} {
			mark := "✗"
			if perms[a] {
				mark = "✓"
			}
			out += fmt.Sprintf("  %s %s\n", mark, a)
		}
		return out
	})
	return nil
}

// runAudit audit 子命令：query（默认）/ export。
func runAudit(args []string, g cliGlobal) error {
	c := newCLIClient(g)
	if len(args) > 0 && args[0] == "export" {
		opts := parseFlags(args[1:])
		project := opts["project"]
		var err error
		if project, err = requireProject(project); err != nil {
			return err
		}
		resp, err := c.call("GET", "/api/audit/export", project, nil)
		if err != nil {
			return err
		}
		fmt.Println(string(resp.Data))
		return nil
	}
	opts := parseFlags(args)
	project := opts["project"]
	var err error
	if project, err = requireProject(project); err != nil {
		return err
	}
	q := ""
	if v := opts["actor"]; v != "" {
		q += "&filter[actor]=" + v
	}
	if v := opts["action"]; v != "" {
		q += "&filter[action]=" + v
	}
	if v := opts["page"]; v != "" {
		q += "&page=" + v
	}
	if v := opts["size"]; v != "" {
		q += "&size=" + v
	}
	resp, err := c.call("GET", "/api/audit?page=1"+q, project, nil)
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return prettyJSON(data) })
	return nil
}
