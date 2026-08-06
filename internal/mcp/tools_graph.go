package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// toolGraphGet 全景图全量数据（未归档任务 + parent/dependency 边，服务端不聚簇）。
var toolGraphGet = mcp.NewTool("graph_get",
	mcp.WithDescription("获取全景图全量数据（nodes=未归档任务，edges=parent/dependency 边；服务端不聚簇）。需 project。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
)

// handleGraphGet graph_get 处理器（graph.read）。
func (s *Server) handleGraphGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "graph.read", req.GetArguments(), func(ctx context.Context, workdir string, _ map[string]any) (any, error) {
		return s.deps.Tasks.Graph(ctx, workdir)
	})
}
