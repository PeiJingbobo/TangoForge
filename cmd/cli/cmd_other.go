package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
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

// runSkills skills 子命令（TF-033 重设计）：list（默认）/ info <name> / install / status / uninstall。
//
//	install  host=<host> packages=<p1,p2,...> --project P  批量安装技能包到宿主位置
//	status   --project P                                   检查宿主安装状态矩阵
//	uninstall host=<host> packages=<p1,p2,...> --project P  卸载技能包
func runSkills(args []string, g cliGlobal) error {
	c := newCLIClient(g)
	sub := ""
	if len(args) > 0 && !isFlag(args[0]) {
		sub = args[0]
		args = args[1:]
	}
	opts := parseFlags(args)
	project, err := requireProject(opts["project"])
	if err != nil {
		return err
	}
	var resp *apiResp
	switch sub {
	case "", "list":
		resp, err = c.call("GET", "/api/skills/packages", project, nil)
	case "info":
		name := opts["name"]
		if name == "" {
			return fmt.Errorf("用法: tangoforge skills info <name> --project P")
		}
		resp, err = c.call("GET", "/api/skills/packages/"+name, project, nil)
	case "status":
		resp, err = c.call("GET", "/api/skills/status", project, nil)
	case "install", "uninstall":
		host := opts["host"]
		packages := splitCSV(opts["packages"])
		if host == "" || len(packages) == 0 {
			return fmt.Errorf("用法: tangoforge skills %s host=<host> packages=<p1,p2,...> --project P", sub)
		}
		resp, err = c.call("POST", "/api/skills/"+sub, project,
			map[string]any{"host": host, "packages": packages})
	default:
		return fmt.Errorf("未知 skills 子命令: %s", sub)
	}
	if err != nil {
		return err
	}
	printOutput(g, resp, func(data json.RawMessage) string { return prettyJSON(data) })
	return nil
}

// runGuide guide 子命令（TF-033）：输出 AI 使用说明书（GET /api/guide，免鉴权）。
// 不经 runCLI（避免 --project 强制）；仍走 ensureDaemon 探活拉起。
func runGuideCommand(args []string) {
	g, _ := extractGlobal(args)
	c := newCLIClient(g)
	if err := c.ensureDaemon(); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
	resp, err := c.call("GET", "/api/guide", "", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
	fmt.Println(string(resp.Data))
}

// splitCSV 拆分逗号分隔列表（packages=p1,p2）。
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// isFlag 判断参数是否为 -- 开头的标志。
func isFlag(s string) bool { return strings.HasPrefix(s, "--") }

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
			"graph.read", "skill.read", "skill.install", "state_machine.read", "state_machine.write",
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
