package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"tangoforge/internal/guide"
	"tangoforge/internal/skill"
	"tangoforge/internal/task"
)

// toolSkillInfo 查询 Skill 技能包详情（skill_info，TF-033 语义迁移：全局技能库而非项目目录）。
var toolSkillInfo = mcp.NewTool("skill_info",
	mcp.WithDescription("查询 AI Skill 技能包详情（SKILL.md，内置 + 全局技能库 ~/.taskboard-app/skills）。需 project + name。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("name", mcp.Required(), mcp.Description("技能包名称")),
)

// handleSkillInfo skill_info 处理器（skill.read）。
func (s *Server) handleSkillInfo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "skill.read", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		name := strArg(args, "name", "")
		if name == "" {
			return nil, task.NewInvalid("缺少必填参数 name")
		}
		pkg, err := s.deps.Skills.Info(ctx, name)
		if err != nil {
			return nil, err
		}
		return pkg, nil
	})
}

// toolSkillInstall 安装技能包到宿主位置（skill_install，TF-033）。
var toolSkillInstall = mcp.NewTool("skill_install",
	mcp.WithDescription("将技能包复制到指定 Agent 宿主的约定位置（AGENTS.md/CLAUDE.md/.cursor/rules/copilot/~/.claude/skills/~/.workbuddy/skills），建立可发现性。需 project + host + packages。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("host", mcp.Required(), mcp.Description("宿主 key：AGENTS.md / CLAUDE.md / .cursor/rules / copilot / user-claude / user-codebuddy")),
	mcp.WithArray("packages", mcp.Required(), mcp.Description("技能包名称列表（批量安装）"), mcp.Items(map[string]any{"type": "string"})),
)

// handleSkillInstall skill_install 处理器（skill.install）。
func (s *Server) handleSkillInstall(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "skill.install", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		host := strArg(args, "host", "")
		if host == "" {
			return nil, task.NewInvalid("缺少必填参数 host")
		}
		packages := strArrayArg(args, "packages")
		if len(packages) == 0 {
			return nil, task.NewInvalid("缺少必填参数 packages（技能包名列表）")
		}
		if _, ok := skill.HostByKey(host); !ok {
			return nil, task.NewInvalid("未知宿主: %s", host)
		}
		results, err := s.deps.Skills.Install(ctx, workdir, host, packages)
		if err != nil {
			return nil, err
		}
		return results, nil
	})
}

// toolSkillStatus 检查宿主安装状态（skill_status，TF-033）。
var toolSkillStatus = mcp.NewTool("skill_status",
	mcp.WithDescription("检查各 Agent 宿主位置的技能包安装状态（missing/current/stale，按版本比对）。需 project。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
)

// handleSkillStatus skill_status 处理器（skill.read）。
func (s *Server) handleSkillStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "skill.read", req.GetArguments(), func(ctx context.Context, workdir string, _ map[string]any) (any, error) {
		return s.deps.Skills.Status(ctx, workdir)
	})
}

// toolSkillUninstall 卸载技能包（skill_uninstall，TF-033）。
var toolSkillUninstall = mcp.NewTool("skill_uninstall",
	mcp.WithDescription("从指定 Agent 宿主位置卸载技能包（移除标记段或删除安装文件）。需 project + host + packages。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("host", mcp.Required(), mcp.Description("宿主 key（同 skill_install）")),
	mcp.WithArray("packages", mcp.Required(), mcp.Description("技能包名称列表"), mcp.Items(map[string]any{"type": "string"})),
)

// handleSkillUninstall skill_uninstall 处理器（skill.install）。
func (s *Server) handleSkillUninstall(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "skill.install", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		host := strArg(args, "host", "")
		if host == "" {
			return nil, task.NewInvalid("缺少必填参数 host")
		}
		packages := strArrayArg(args, "packages")
		if len(packages) == 0 {
			return nil, task.NewInvalid("缺少必填参数 packages（技能包名列表）")
		}
		if _, ok := skill.HostByKey(host); !ok {
			return nil, task.NewInvalid("未知宿主: %s", host)
		}
		results, err := s.deps.Skills.Uninstall(ctx, workdir, host, packages)
		if err != nil {
			return nil, err
		}
		return results, nil
	})
}

// toolGuide 读取系统使用说明书（guide，QA-S3 免鉴权：无 project 参数、不走 exec 权限链）。
var toolGuide = mcp.NewTool("guide",
	mcp.WithDescription("读取 TangoForge 系统使用说明书（免鉴权）：系统概念 / HTTP 端点表 / MCP 工具表 / CLI 用法 / 业务语义。AI Agent 首次接入时先调用本工具掌握系统用法。"),
)

// handleGuide guide 工具处理器：直接渲染说明书（不查权限、不需 project）。
func (s *Server) handleGuide(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return toolResult(guide.Render(0))
}
