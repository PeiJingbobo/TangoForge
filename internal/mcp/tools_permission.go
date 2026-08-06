package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// toolPermissionList 查询项目 Agent 权限范围（全量 16 项）。
var toolPermissionList = mcp.NewTool("permission_list",
	mcp.WithDescription("查询项目 Agent 权限范围（全量 action，allowed 布尔）。需 project。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
)

// handlePermissionList permission_list 处理器（permission.read）。
func (s *Server) handlePermissionList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "permission.read", req.GetArguments(), func(ctx context.Context, workdir string, _ map[string]any) (any, error) {
		perms, err := s.deps.Perms.Get(ctx, workdir)
		if err != nil {
			return nil, err
		}
		return perms, nil
	})
}
