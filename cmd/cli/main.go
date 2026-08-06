// Command cli 是 TangoForge 命令行客户端入口（与守护进程通过 HTTP 交互）。
//
// 项目标识约定（docs/TECHNICAL.md §3.4）：所有操作子命令必须显式携带
// --project <工作目录>，未携带或目录未注册返回 PROJECT_NOT_FOUND。
//
// 当前为最小骨架：仅提供 version / help；完整命令集（project / task /
// import / export / permission / skill）按 docs/AGENTS.md「当前开发阶段
// 重点任务」逐步实现，并共享同一套业务层实现（接口先行）。
package main

import (
	"fmt"
	"os"
)

// version 通过 -ldflags "-X main.version=..." 注入。
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version", "-v", "--version":
		fmt.Printf("tangoforge %s\n", version)
	case "help", "-h", "--help":
		usage()
	case "mcp":
		// stdio MCP 服务（QA P4-1：同二进制子命令，直连业务层；daemon 侧另有 /mcp HTTP 传输）。
		runMCPCommand(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `TangoForge CLI — 人机协作任务看板命令行入口

用法:
  tangoforge version        输出版本
  tangoforge help           显示帮助
  tangoforge mcp            启动 stdio MCP 服务（供 AI Agent 操作任务池）

规划中的子命令: project / task / import / export / permission / skill
所有操作子命令均需显式携带 --project <工作目录>。`)
}
