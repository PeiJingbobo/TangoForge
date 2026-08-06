package mcp

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"

	"tangoforge/internal/config"
	"tangoforge/internal/task"
)

// toolStateMachineGet 读取项目状态机定义。
var toolStateMachineGet = mcp.NewTool("state_machine_get",
	mcp.WithDescription("读取项目状态机定义（states + transitions）。需 project。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
)

// toolStateMachineUpdate 更新状态机定义。
var toolStateMachineUpdate = mcp.NewTool("state_machine_update",
	mcp.WithDescription("更新项目状态机定义（states + transitions，全量覆盖；有任务占用的状态不可删除/重命名）。"+
		"需 project + state_machine 对象：{\"states\":[{\"key\":\"todo\",\"label\":\"待办\",\"color\":\"#fff\"}],\"transitions\":[{\"from\":\"todo\",\"to\":[\"doing\"]}]}"),
	mcp.WithString("project", mcp.Required(), mcp.Description("项目工作目录绝对路径")),
	mcp.WithAny("state_machine", mcp.Required(), mcp.Description("状态机对象（states + transitions）")),
)

// handleStateMachineGet state_machine_get 处理器（state_machine.read）。
func (s *Server) handleStateMachineGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "state_machine.read", req.GetArguments(), func(ctx context.Context, workdir string, _ map[string]any) (any, error) {
		return s.deps.Tasks.GetStateMachine(ctx, workdir)
	})
}

// handleStateMachineUpdate state_machine_update 处理器（state_machine.write）。
func (s *Server) handleStateMachineUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.exec(ctx, "state_machine.write", req.GetArguments(), func(ctx context.Context, workdir string, args map[string]any) (any, error) {
		raw, ok := args["state_machine"]
		if !ok {
			return nil, task.NewInvalid("缺少必填参数 state_machine")
		}
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, task.NewInvalid("state_machine 参数非法: %v", err)
		}
		var sm config.StateMachine
		if err := json.Unmarshal(data, &sm); err != nil {
			return nil, task.NewInvalid("state_machine 参数非法: %v", err)
		}
		return s.deps.Tasks.UpdateStateMachine(ctx, workdir, sm)
	})
}
