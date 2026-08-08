package mcp

import (
	"context"
	"tangoforge/internal/parser"
	"tangoforge/internal/task"

	"github.com/mark3labs/mcp-go/mcp"
)

// import 域工具（草稿流，TF-018 业务层）。

// toolImportPreview 提交 Markdown 解析 → 生成草稿（LLM）。支持单文件/多文件/目录一次解析。
var toolImportPreview = mcp.NewTool("import_preview",
	mcp.WithDescription("提交 Markdown 解析生成草稿（不直接入库）。需 project；输入四选一："+
		"file_path（单文件）| file_paths（多文件数组，合并为一次解析）| directory（目录，递归扫描 *.md）| content+source_file。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("file_path", mcp.Description("单文件路径（相对 workdir 或绝对）")),
	mcp.WithArray("file_paths", mcp.Description("多文件路径数组（合并为一次解析）")),
	mcp.WithString("directory", mcp.Description("目录路径（递归扫描 *.md/*.markdown）")),
	mcp.WithString("content", mcp.Description("Markdown 内容（与 source_file 搭配）")),
	mcp.WithString("source_file", mcp.Description("覆盖单元标识（content 方式必填）")),
)

// toolImportConfirm 确认草稿入库（source_file 文件级全量覆盖）。
var toolImportConfirm = mcp.NewTool("import_confirm",
	mcp.WithDescription("确认草稿入库（文件级全量覆盖：归档该 source_file 旧任务 + 批量重建）。需 project + draft_id。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("draft_id", mcp.Required(), mcp.Description("草稿 ID（import_preview 返回）")),
)

// toolImportDiscard 丢弃草稿。
var toolImportDiscard = mcp.NewTool("import_discard",
	mcp.WithDescription("丢弃草稿（不影响正式任务池）。需 project + draft_id。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("draft_id", mcp.Required(), mcp.Description("草稿 ID")),
)

// handleImportPreview import_preview 处理器（import.run）。
func (s *Server) handleImportPreview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "import.run", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		in := parser.ParseInput{
			FilePath:   strArg(args, "file_path", ""),
			FilePaths:  strArrayArg(args, "file_paths"),
			Directory:  strArg(args, "directory", ""),
			Content:    strArg(args, "content", ""),
			SourceFile: strArg(args, "source_file", ""),
		}
		draft, err := s.deps.Parser.Parse(ctx, workdir, in)
		if err != nil {
			return nil, err
		}
		return draft, nil
	})
}

// handleImportConfirm import_confirm 处理器（import.confirm）。
func (s *Server) handleImportConfirm(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "import.confirm", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		draftID := strArg(args, "draft_id", "")
		if draftID == "" {
			return nil, task.NewInvalid("缺少必填参数 draft_id")
		}
		res, err := s.deps.Parser.Confirm(ctx, workdir, draftID)
		if err != nil {
			return nil, err
		}
		return res, nil
	})
}

// handleImportDiscard import_discard 处理器（import.run）。
func (s *Server) handleImportDiscard(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "import.run", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		draftID := strArg(args, "draft_id", "")
		if draftID == "" {
			return nil, task.NewInvalid("缺少必填参数 draft_id")
		}
		if err := s.deps.Parser.Discard(ctx, workdir, draftID); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, nil
	})
}
