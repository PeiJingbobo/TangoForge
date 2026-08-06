package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"tangoforge/internal/exporter"
)

// toolExportMarkdown 导出 Markdown（默认/LLM 模板 + overwrite/copy）。
var toolExportMarkdown = mcp.NewTool("export_markdown",
	mcp.WithDescription("从任务库渲染 Markdown 并写盘。需 project；template_mode default|llm（缺省 default）；"+
		"target overwrite|copy（缺省 copy；overwrite 必须提供 path；copy 缺省写 {project}/.taskboard/export.md）。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("template_mode", mcp.Description("default|llm（缺省 default）")),
	mcp.WithString("target", mcp.Description("overwrite|copy（缺省 copy）")),
	mcp.WithString("path", mcp.Description("输出路径（overwrite 必填；copy 可选）")),
)

// handleExportMarkdown export_markdown 处理器（export.run）。
func (s *Server) handleExportMarkdown(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "export.run", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		opts := exporter.RenderOptions{
			TemplateMode: strArg(args, "template_mode", "default"),
			Target:       strArg(args, "target", "copy"),
			Path:         strArg(args, "path", ""),
		}
		res, err := s.deps.Exporter.Render(ctx, workdir, opts)
		if err != nil {
			return nil, err
		}
		return res, nil
	})
}
