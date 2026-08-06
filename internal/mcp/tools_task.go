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

// toolTaskList 任务列表（树形，支持 status/q 过滤）。
var toolTaskList = mcp.NewTool("task_list",
	mcp.WithDescription("任务列表（树形）。需显式 project 参数；status/q 可选过滤；默认排除 archived。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("status", mcp.Description("状态过滤（如 todo/doing/done/archived）")),
	mcp.WithString("q", mcp.Description("标题/描述关键词搜索")),
)

// toolTaskUpdate 更新任务字段（禁止修改 status；状态流转走 task_update_status？——v1 用 task_update 不可改 status，
// 状态流转请使用 task_update 之外的状态机接口；本工具支持 title/description/priority/tags/assignee/depends_on/parent_id）。
var toolTaskUpdate = mcp.NewTool("task_update",
	mcp.WithDescription("更新任务字段（不含 status）。需 project + id；title/description/priority/tags/assignee/depends_on/parent_id 均可部分更新；"+
		"parent_id 传空串表示置为顶层。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("id", mcp.Required(), mcp.Description("任务 ID")),
	mcp.WithString("title", mcp.Description("新标题")),
	mcp.WithString("description", mcp.Description("新描述（空串清空）")),
	mcp.WithAny("priority", mcp.Description("优先级：0-5 整数或字符串别名")),
	mcp.WithArray("tags", mcp.Description("标签数组（空数组清空）")),
	mcp.WithString("assignee", mcp.Description("负责人（空串清空）")),
	mcp.WithArray("depends_on", mcp.Description("被依赖任务 ID 数组（空数组清空）")),
	mcp.WithString("parent_id", mcp.Description("父任务 ID；空串=置为顶层")),
)

// toolTaskArchive 归档任务（删除语义）。
var toolTaskArchive = mcp.NewTool("task_archive",
	mcp.WithDescription("归档任务（软删除，子任务级联置空 parent_id）。需 project + id。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("id", mcp.Required(), mcp.Description("任务 ID")),
)

// toolTaskRestore 还原归档任务。
var toolTaskRestore = mcp.NewTool("task_restore",
	mcp.WithDescription("从回收站还原归档任务。需 project + id；FallbackTodo 为 true 时，归档前状态已从状态机删除则回退 todo。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithString("id", mcp.Required(), mcp.Description("任务 ID")),
	mcp.WithAny("fallback_todo", mcp.Description("目标状态已删除时回退 todo（默认 false）")),
)

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

// handleTaskList task_list 工具处理器（树形列表）。
func (s *Server) handleTaskList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "task.read", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
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

// handleTaskUpdate task_update 工具处理器（指针语义，QA P4-1 §4.2）。
func (s *Server) handleTaskUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "task.update", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		id := strArg(args, "id", "")
		if id == "" {
			return nil, task.NewInvalid("缺少必填参数 id")
		}
		in := task.UpdateInput{}
		if v, ok := args["title"]; ok {
			if sv, ok := v.(string); ok {
				in.Title = &sv
			}
		}
		if v, ok := args["description"]; ok {
			if sv, ok := v.(string); ok {
				in.Description = &sv
			}
		}
		if v, ok := args["assignee"]; ok {
			if sv, ok := v.(string); ok {
				in.Assignee = &sv
			}
		}
		if v, ok := args["priority"]; ok {
			p := v
			in.Priority = &p
		}
		if v, ok := args["tags"]; ok {
			if arr, ok := v.([]any); ok {
				tags := make([]string, 0, len(arr))
				for _, item := range arr {
					if s, ok := item.(string); ok {
						tags = append(tags, s)
					}
				}
				in.Tags = &tags
			}
		}
		if v, ok := args["depends_on"]; ok {
			if arr, ok := v.([]any); ok {
				deps := make([]string, 0, len(arr))
				for _, item := range arr {
					if s, ok := item.(string); ok {
						deps = append(deps, s)
					}
				}
				in.DependsOn = &deps
			}
		}
		if v, ok := args["parent_id"]; ok {
			if sv, ok := v.(string); ok {
				// 三重态：缺省不改 / 空串置顶 / 非空改父（UpdateInput.ParentID **string）。
				var pstr *string
				if sv != "" {
					pstr = &sv
				}
				in.ParentID = &pstr
			}
		}
		t, err := s.deps.Tasks.Update(ctx, workdir, id, in)
		if err != nil {
			return nil, err
		}
		return t, nil
	})
}

// handleTaskArchive task_archive 工具处理器。
func (s *Server) handleTaskArchive(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "task.delete", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		id := strArg(args, "id", "")
		if id == "" {
			return nil, task.NewInvalid("缺少必填参数 id")
		}
		res, err := s.deps.Tasks.Archive(ctx, workdir, id)
		if err != nil {
			return nil, err
		}
		return res, nil
	})
}

// handleTaskRestore task_restore 工具处理器。
func (s *Server) handleTaskRestore(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "task.restore", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		id := strArg(args, "id", "")
		if id == "" {
			return nil, task.NewInvalid("缺少必填参数 id")
		}
		opts := task.RestoreOptions{}
		if v, ok := args["fallback_todo"].(bool); ok && v {
			opts.FallbackTodo = true
		}
		t, err := s.deps.Tasks.Restore(ctx, workdir, id, opts)
		if err != nil {
			return nil, err
		}
		return t, nil
	})
}
