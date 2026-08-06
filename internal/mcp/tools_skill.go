package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"tangoforge/internal/task"
)

// toolSkillInfo 查询 Skill 详情（skill_info）。
var toolSkillInfo = mcp.NewTool("skill_info",
	mcp.WithDescription("查询 AI Skill 详情（来自 {project}/.taskboard/skills/）。需 project + name。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("name", mcp.Required(), mcp.Description("Skill 名称")),
)

// handleSkillInfo skill_info 处理器（skill.read）。
func (s *Server) handleSkillInfo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "skill.read", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		name := strArg(args, "name", "")
		if name == "" {
			return nil, task.NewInvalid("缺少必填参数 name")
		}
		info, err := s.deps.Skills.Info(ctx, workdir, name)
		if err != nil {
			return nil, err
		}
		return info, nil
	})
}
