// Package guide 生成 TangoForge 系统使用说明书（AI 版），供三端免鉴权输出（QA-S3）：
//
//   - HTTP：GET /api/guide（任何来源可读，不经过鉴权中间件）；
//   - MCP：guide 工具（无参数）；
//   - CLI：tangoforge guide。
//
// 内容单一来源（本包）：系统简介 / 核心概念 / HTTP 端点表 / MCP 工具表 / CLI 子命令表 /
// 业务语义速查，动态拼装 Markdown。**端点/工具清单与实现同步**（server.go 路由、
// mcp registerTools、cli main.go 改动后必须同步本文件，guard 测试断言关键项）。
//
// 分层铁律（AGENTS.md §3.2）：本包为业务层，禁止引用 api / mcp / cmd。
package guide

import (
	"fmt"
	"strings"
)

// Endpoint 一个 HTTP 端点描述。
type Endpoint struct {
	Method, Path, Perm, Desc string
}

// Tool 一个 MCP 工具描述。
type Tool struct {
	Name, Desc string
}

// Command 一个 CLI 子命令描述。
type Command struct {
	Name, Desc string
}

// HTTPEndpoints 端点表（与 internal/api/server.go 路由同步，改动必须更新）。
var HTTPEndpoints = []Endpoint{
	{"GET", "/ping", "-", "健康检查"},
	{"GET", "/api/guide", "-", "本说明书（免鉴权）"},
	{"GET", "/ws/events", "-", "WebSocket 事件订阅（?project= 指定项目）"},
	{"POST", "/mcp", "Bearer", "远程 MCP Streamable HTTP（回环免鉴权）"},
	{"GET", "/api/projects", "project.read", "项目列表（仅已完成引导的可见项目）"},
	{"POST", "/api/projects/import", "project.read", "导入（或初始化）目录为项目"},
	{"POST", "/api/projects/complete", "ui-only", "标记项目引导完成（TF-043 暂时隐藏 → 列表可见）"},
	{"POST", "/api/projects/check", "-", "目录导入前置检查（TF-041 引导：已注册/元数据合法性）"},
	{"POST", "/api/projects/import/reset", "ui-only", "清空历史元数据（TF-041 引导，仅未注册目录）"},
	{"DELETE", "/api/projects/{id}", "ui-only", "移除项目注册（不动磁盘数据）"},
	{"GET", "/api/config", "ui-only", "全局配置（LLM/端口/开关）"},
	{"PUT", "/api/config", "ui-only", "更新全局配置（热重载）"},
	{"POST", "/api/config/test", "ui-only", "LLM 连接测试（TF-041 引导，暂存配置）"},
	{"GET", "/api/tasks", "task.read", "任务树（全部任务）"},
	{"POST", "/api/tasks", "task.create", "创建任务"},
	{"GET", "/api/tasks/{id}", "task.read", "任务详情"},
	{"PATCH", "/api/tasks/{id}", "task.update", "更新任务字段"},
	{"POST", "/api/tasks/{id}/archive", "task.delete", "归档（删除=归档）"},
	{"POST", "/api/tasks/{id}/restore", "task.restore", "还原归档任务"},
	{"DELETE", "/api/tasks/{id}", "task.delete", "物理删除（仅回收站内任务）"},
	{"GET", "/api/state-machine", "state_machine.read", "项目状态机"},
	{"PUT", "/api/state-machine", "state_machine.write", "更新状态机"},
	{"GET", "/api/project-config", "state_machine.read", "项目配置（状态机+导出模板）"},
	{"PUT", "/api/project-config", "ui-only", "更新项目配置（仅 UI）"},
	{"GET", "/api/permissions", "permission.read", "Agent 权限范围"},
	{"PUT", "/api/permissions", "ui-only", "覆盖权限（仅 UI，全量）"},
	{"GET", "/api/graph", "graph.read", "全景图（节点+依赖边）"},
	{"GET", "/api/audit", "audit.read", "审计日志查询（分页）"},
	{"GET", "/api/audit/export", "audit.read", "导出 audit.log 文本"},
	{"POST", "/api/import", "import.run", "Markdown 导入 → 草稿（LLM 解析）"},
	{"GET", "/api/import/drafts", "import.run", "草稿列表"},
	{"GET", "/api/import/drafts/{id}", "import.run", "草稿详情（任务树）"},
	{"PUT", "/api/import/drafts/{id}/tasks", "import.run", "草稿任务树更新"},
	{"POST", "/api/import/drafts/{id}/confirm", "import.confirm", "确认导入（文件级覆盖）"},
	{"DELETE", "/api/import/drafts/{id}", "import.run", "丢弃草稿"},
	{"POST", "/api/export", "export.run", "导出 Markdown（模板渲染）"},
	{"POST", "/api/export/template/generate", "export.run", "LLM 生成导出模板"},
	{"GET", "/api/skills/packages", "skill.read", "技能包列表（内置+全局库）"},
	{"GET", "/api/skills/packages/{name}", "skill.read", "技能包详情（SKILL.md）"},
	{"GET", "/api/skills/status", "skill.read", "宿主安装状态矩阵"},
	{"POST", "/api/skills/install", "skill.install", "安装技能包到宿主位置"},
	{"POST", "/api/skills/uninstall", "skill.install", "卸载技能包"},
	{"PUT", "/api/skills/packages/{name}", "ui-only", "写入自定义技能包（全局库）"},
	{"GET", "/api/skill-template", "skill.read", "全局默认 Skill 模板"},
	{"PUT", "/api/skill-template", "ui-only", "写入自定义 Skill 模板（仅 UI）"},
}

// MCPTools 工具表（与 internal/mcp registerTools 同步，改动必须更新）。
var MCPTools = []Tool{
	{"guide", "读取本系统使用说明书（免鉴权，无参数）"},
	{"project_list", "列出已注册项目"},
	{"project_import", "导入目录为项目（幂等）"},
	{"project_init", "仅初始化目录元数据，不注册"},
	{"project_create", "初始化并注册项目"},
	{"task_list", "任务树（全部任务）"},
	{"task_read", "任务详情"},
	{"task_create", "创建任务"},
	{"task_update", "更新任务字段/状态"},
	{"task_archive", "归档任务（删除=归档）"},
	{"task_restore", "还原归档任务"},
	{"import_preview", "Markdown 导入 → 草稿预览"},
	{"import_confirm", "确认导入草稿"},
	{"import_discard", "丢弃草稿"},
	{"export_markdown", "导出 Markdown"},
	{"graph_get", "全景图数据"},
	{"state_machine_get", "项目状态机"},
	{"state_machine_update", "更新状态机"},
	{"skill_info", "技能包详情"},
	{"skill_install", "安装技能包到宿主位置"},
	{"skill_status", "宿主安装状态"},
	{"skill_uninstall", "卸载技能包"},
	{"permission_list", "Agent 权限范围"},
}

// CLICmds 子命令表（与 cmd/cli main.go 同步，改动必须更新）。
var CLICmds = []Command{
	{"guide", "打印本系统使用说明书"},
	{"mcp", "启动 stdio MCP 服务（AI Agent 使用）"},
	{"projects list|import <dir>|remove <id>", "项目注册表操作"},
	{"tasks list|get|create|update|status|archive|restore|delete", "任务操作"},
	{"import preview|drafts|confirm|discard", "Markdown 导入草稿流"},
	{"export [run|template]", "导出 Markdown / LLM 模板生成"},
	{"graph", "全景图数据"},
	{"state-machine get|update <file.json>", "状态机读写"},
	{"skills [list|info|install|status|uninstall]", "技能包管理"},
	{"permission", "Agent 权限范围"},
	{"audit [export]", "审计查询/导出"},
}

// Render 渲染完整说明书（Markdown）。port 为守护进程监听端口（0 → 默认 19810）。
func Render(port int) string {
	if port <= 0 {
		port = 19810
	}
	var b strings.Builder
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	b.WriteString("# TangoForge 使用指南（AI 版）\n\n")
	b.WriteString("TangoForge 是本地优先的人机协作任务中间件：任务数据自包含于项目工作目录的 `.taskboard/`，\n")
	b.WriteString("系统没有云端依赖；GUI、CLI、MCP、HTTP 拥有完全等价的操作权（多端等价）。\n\n")

	b.WriteString("## 1. 核心概念\n\n")
	b.WriteString("- **项目** = 一个工作目录（含 `.taskboard/` 元数据）。**所有操作必须显式携带项目目录**\n")
	b.WriteString("  （HTTP `X-Project` 头 / MCP `project` 参数 / CLI `--project`），禁止隐式依赖\"当前项目\"。\n")
	b.WriteString("- **任务**：`id`(UUID)、`parent_id`(子任务)、`title`、`description`、`status`、`priority`(0-5)、\n")
	b.WriteString("  `tags`、`assignee`、`depends_on`(依赖 ID 数组，必须无环)、`source_file/section`。\n")
	b.WriteString("- **状态机**：默认 `todo → doing → done → archived`；每项目独立可配置\n")
	b.WriteString("  （`state_machine_get` 查看；`archived` 为系统保留态）。\n")
	b.WriteString("- **删除 = 归档**：删除操作统一归档（记录归档前状态），回收站可还原；物理删除仅限回收站内任务。\n")
	b.WriteString("- **级联**：父任务归档/删除时，全部子任务自动置空 `parent_id`（成为顶层任务）。\n")
	b.WriteString("- **依赖无环**：写操作若引入循环依赖直接拒绝（`CIRCULAR_DEPENDENCY`）。\n")
	b.WriteString("- **导入走草稿流**：LLM 解析结果先入草稿，确认后按源文件**文件级全量覆盖**入库。\n\n")

	b.WriteString("## 2. HTTP API（推荐 AI 使用）\n\n")
	fmt.Fprintf(&b, "- Base：`%s`\n", base)
	b.WriteString("- 业务端点需携带 `X-Project: <项目工作目录>`；UI 专属端点（写全局配置/权限/项目配置）需\n")
	b.WriteString("  `X-UI-Token`（仅回环）；远程访问需 `Authorization: Bearer <api_token>`。\n")
	b.WriteString("- 响应：成功 `{\"code\":0,\"data\":...}`；失败 `{\"code\":\"<错误码>\",\"message\":\"可读错误\",\"detail\":\"可选\"}`。\n")
	b.WriteString("- 端点表（权限列为 Agent 需被授予的动作；ui-only 表示仅 App UI 可调）：\n\n")

	b.WriteString("| 方法 | 路径 | 权限 | 说明 |\n|---|---|---|---|\n")
	for _, ep := range HTTPEndpoints {
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s |\n", ep.Method, ep.Path, ep.Perm, ep.Desc)
	}

	b.WriteString("\n## 3. MCP（AI Agent 首选接入）\n\n")
	b.WriteString("- 启动：`tangoforge mcp`（stdio 传输；守护进程自动拉起）。所有工具第一个参数均为\n")
	b.WriteString("  `project`（工作目录绝对路径，强制）；`guide` 工具除外（免鉴权说明书）。\n")
	b.WriteString("- 工具表：\n\n")

	b.WriteString("| 工具 | 说明 |\n|---|---|\n")
	for _, t := range MCPTools {
		fmt.Fprintf(&b, "| `%s` | %s |\n", t.Name, t.Desc)
	}

	b.WriteString("\n## 4. CLI\n\n")
	b.WriteString("- 二进制 `tangoforge`（与 daemon 同目录自动拉起）。操作类子命令必须带 `--project <工作目录>`；\n")
	b.WriteString("  全局参数 `--server <addr>` / `--actor <name>` / `--json`。\n")
	b.WriteString("- 子命令表：\n\n")

	b.WriteString("| 子命令 | 说明 |\n|---|---|\n")
	for _, c := range CLICmds {
		fmt.Fprintf(&b, "| `tangoforge %s` | %s |\n", c.Name, c.Desc)
	}

	b.WriteString("\n## 5. 业务语义速查\n\n")
	b.WriteString("- **错误码 → HTTP**：`PROJECT_NOT_FOUND`→404 / `TASK_INVALID`→422 / `INVALID_TRANSITION`→400 /\n")
	b.WriteString("  `CIRCULAR_DEPENDENCY`→409 / `STATUS_IN_USE`→422 / `PERMISSION_DENIED`→403 / `UNAUTHORIZED`→401 /\n")
	b.WriteString("  `SKILL_NOT_FOUND`→404 / `NOT_IMPLEMENTED`→501 / 未知→500。\n")
	b.WriteString("- **权限模型**：来源识别 5 级（ui / agent-Bearer / agent-X-Actor / agent-MCP / unknown）；\n")
	b.WriteString("  ui 全权限放行，其余查项目 `permissions` 表；**权限行缺失 = denied**（安全默认）；\n")
	b.WriteString("  新项目默认授予只读 5 项（task.read / graph.read / skill.read / project.read / permission.read）。\n")
	b.WriteString("- **状态机**：状态流转必须满足项目 `transitions`；非法流转 `INVALID_TRANSITION`；\n")
	b.WriteString("  **有任务占用的状态不可移除**（`STATUS_IN_USE`）。\n")
	b.WriteString("- **审计**：所有写操作异步写入项目库 `audit_log`（actor/action/target/result），只追加不可篡改；\n")
	b.WriteString("  `GET /api/audit/export` 导出文本文件。\n")
	b.WriteString("- **Skill 技能包**：`GET /api/skills/packages` 查看内置+全局技能库；`POST /api/skills/install`\n")
	b.WriteString("  把技能包复制到宿主目录型位置（.claude/skills / .cursor/skills / .github/skills /\n")
	b.WriteString("  ~/.claude/skills / ~/.workbuddy/skills，各目录下 <包名>/SKILL.md）；`GET /api/skills/status` 检查安装状态。\n")
	return b.String()
}
