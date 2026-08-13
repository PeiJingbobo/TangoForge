// Command cli 是 TangoForge 命令行客户端入口（TF-021：全部子命令转 HTTP 调用，多端等价）。
//
// 项目标识约定（docs/TECHNICAL.md §3.4）：任务/导入/导出等操作子命令必须显式携带
// --project <工作目录>（未携带报错）；来源识别：CLI 默认 X-Actor: human（agent 身份查权限表，
// 可 --actor 覆盖）；--server 指定守护进程地址（默认 127.0.0.1:19810），未运行时自动拉起
// （QA P4-1 Q15-A：spawn 同目录 daemon 二进制并轮询 /ping ≤5s，找不到则提示手动启动）。
//
// 子命令：mcp（stdio 服务，QA P4-1）/ projects / tasks / import / export / graph /
// state-machine / skills / permission / audit。
package main

import (
	"fmt"
	"os"
	"strings"
	"tangoforge/internal/version"
)

// version 通过 -ldflags "-X tangoforge/internal/version.Version=..." 注入（Makefile LDFLAGS 统一注入）。
var buildVersion = version.String()

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	args := os.Args[1:]
	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "version", "-v", "--version":
		fmt.Printf("tangoforge %s\n", buildVersion)
	case "help", "-h", "--help":
		usage()
	case "mcp":
		// stdio MCP 服务（QA P4-1：同二进制子命令，直连业务层；不经 HTTP）。
		runMCPCommand(rest)
	case "guide":
		// AI 说明书（TF-033：免鉴权端点，不经过 ensureDaemon 的 actor 判断；仍探活拉起）。
		runGuideCommand(rest)
	case "projects":
		// 项目组子命令无 --project 参数（与 HTTP /api/projects 组一致）。
		runCLI("projects", rest)
	case "tasks", "import", "export", "graph", "state-machine", "skills", "permission", "audit", "knowledge":
		runCLI(cmd, rest)
	default:
		usage()
		os.Exit(2)
	}
}

// runCLI 解析全局参数（--server / --actor / --json，可出现在任意位置）并分发子命令。
func runCLI(cmd string, args []string) {
	g, rest := extractGlobal(args)
	c := newCLIClient(g)
	if err := c.ensureDaemon(g.NoLift); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
	var err error
	switch cmd {
	case "projects":
		err = runProjects(rest, g)
	case "tasks":
		err = runTasks(rest, g)
	case "import":
		err = runImport(rest, g)
	case "export":
		err = runExport(rest, g)
	case "graph":
		err = runGraph(rest, g)
	case "state-machine":
		err = runStateMachine(rest, g)
	case "skills":
		err = runSkills(rest, g)
	case "permission":
		err = runPermission(rest, g)
	case "audit":
		err = runAudit(rest, g)
	case "knowledge":
		err = runKnowledge(rest, g)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

// extractGlobal 提取全局参数（--server / --actor / --json），返回剩余参数。
func extractGlobal(args []string) (cliGlobal, []string) {
	g := cliGlobal{}
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--server" && i+1 < len(args):
			g.Server = args[i+1]
			i++
		case a == "--actor" && i+1 < len(args):
			g.Actor = args[i+1]
			i++
		case a == "--json":
			g.JSON = true
		case a == "--no-lift":
			g.NoLift = true
		case strings.HasPrefix(a, "--server="):
			g.Server = strings.TrimPrefix(a, "--server=")
		case strings.HasPrefix(a, "--actor="):
			g.Actor = strings.TrimPrefix(a, "--actor=")
		default:
			rest = append(rest, a)
		}
	}
	return g, rest
}

func usage() {
	fmt.Fprintln(os.Stderr, `TangoForge CLI — 人机协作任务看板命令行入口（HTTP 等价操作）

用法:
  tangoforge version               输出版本
  tangoforge mcp                   启动 stdio MCP 服务（AI Agent 使用）
  tangoforge projects list|import <dir>|remove <id>
  tangoforge tasks list|get|create|update|status|archive|restore|delete ...
  tangoforge import preview|drafts|confirm|discard ...
  tangoforge export [run|template] ...
  tangoforge graph
  tangoforge state-machine get|update <file.json>
  tangoforge skills [list|info <name>|install|status|uninstall]
  tangoforge permission
  tangoforge audit [export]
  tangoforge knowledge bases|documents|search|read|link|unlink|relink|scan|edit
  tangoforge guide

全局参数: --server <addr>（默认 127.0.0.1:19810） --actor <name>（默认 human） --json --no-lift
任务类子命令必须携带 --project <工作目录>；守护进程未运行且未找到二进制时命令无法完成；详细用法见 docs/TECHNICAL.md §3.4 与 docs/TASK-SEMANTICS.md。`)
}
