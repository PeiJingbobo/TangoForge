package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"tangoforge/internal/task"
)

// 工具定义（v1 固定工具集，REQUIREMENTS.md §8.3）。
// 每个工具首个参数为 project（工作目录绝对路径，强制，QA P4-1）。

// toolTaskRead 读取任务：带 id 返回详情；不带返回任务树（支持 status/q 过滤）。
var toolTaskRead = mcp.NewTool("task_read",
	mcp.WithDescription("读取任务列表（树形）或单个任务详情。需显式 project 参数；"+
		"status/q 为可选过滤；id 缺省返回全量任务树（默认排除 archived）。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("id", mcp.Description("任务 ID；缺省返回任务树")),
	mcp.WithString("status", mcp.Description("状态过滤（如 todo/doing/done/archived）")),
	mcp.WithString("q", mcp.Description("标题/描述关键词搜索")),
)

// toolTaskCreate 创建任务。
var toolTaskCreate = mcp.NewTool("task_create",
	mcp.WithDescription("创建任务。需显式 project 参数；title 必填；"+
		"status 缺省 todo（须为项目状态机 key）；priority 支持 0-5 整数或别名（lowest/low/normal/high/highest 等）。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("title", mcp.Required(), mcp.Description("任务标题（必填）")),
	mcp.WithString("description", mcp.Description("任务描述")),
	mcp.WithString("status", mcp.Description("状态机 key（缺省 todo）")),
	mcp.WithAny("priority", mcp.Description("优先级：0-5 整数或字符串别名")),
	mcp.WithArray("tags", mcp.Description("标签数组")),
	mcp.WithString("assignee", mcp.Description("负责人（自由文本）")),
	mcp.WithArray("depends_on", mcp.Description("被依赖任务 ID 数组（A 依赖 B → depends_on=[B]）")),
	mcp.WithString("parent_id", mcp.Description("父任务 ID（缺省顶层）")),
)

// handleTaskRead task_read 工具处理器。
func (s *Server) handleTaskRead(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "task.read", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		if id := strArg(args, "id", ""); id != "" {
			t, err := s.deps.Tasks.Get(ctx, workdir, id)
			if err != nil {
				return nil, err
			}
			return t, nil
		}
		f := task.ListFilter{Status: strArg(args, "status", ""), Q: strArg(args, "q", "")}
		res, err := s.deps.Tasks.List(ctx, workdir, f)
		if err != nil {
			return nil, err
		}
		if res.Tree != nil {
			return res.Tree, nil
		}
		return res, nil
	})
}

// handleTaskCreate task_create 工具处理器。
func (s *Server) handleTaskCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "task.create", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		in := task.CreateInput{
			Title:       strArg(args, "title", ""),
			Description: strArg(args, "description", ""),
			Assignee:    strArg(args, "assignee", ""),
			Tags:        strArrayArg(args, "tags"),
			DependsOn:   strArrayArg(args, "depends_on"),
		}
		if status := strArg(args, "status", ""); status != "" {
			in.Status = &status
		}
		if priority, ok := args["priority"]; ok {
			in.Priority = priority
		}
		if parent := strArg(args, "parent_id", ""); parent != "" {
			in.ParentID = &parent
		}
		t, err := s.deps.Tasks.Create(ctx, workdir, in)
		if err != nil {
			return nil, err
		}
		return t, nil
	})
}
