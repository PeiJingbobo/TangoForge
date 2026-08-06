package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// project 域工具（QA P4-1 Q6）：project_list 无 project 参数（全局操作，豁免）；
// project_import / project_init / project_create 的 project 参数为目标工作目录。
// 权限：与 HTTP /api/projects 组一致——项目组豁免权限中间件（QA P3-2），
// 故本域工具不做 Perms.Require 检查（project.read 默认授予，注册/初始化属项目引导）。

// toolProjectList 项目列表（全局，无 project 参数）。
var toolProjectList = mcp.NewTool("project_list",
	mcp.WithDescription("列出全部已注册项目（无需 project 参数）。"),
)

// toolProjectImport 仅导入（注册）：要求目录已初始化（存在 .taskboard/meta.db）。
var toolProjectImport = mcp.NewTool("project_import",
	mcp.WithDescription("导入已初始化的目录为项目（仅注册，不初始化）。目录必须已有 .taskboard/meta.db（请先 project_init 或 project_create）。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("目标工作目录绝对路径")),
)

// toolProjectInit 仅初始化：创建 .taskboard/ 元数据（meta.db + 默认权限 + config.yaml + skills/），不注册。
var toolProjectInit = mcp.NewTool("project_init",
	mcp.WithDescription("初始化目录为项目元数据（创建 .taskboard/：meta.db + 默认权限 + config.yaml + skills/），不注册项目。幂等。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("目标工作目录绝对路径")),
)

// toolProjectCreate 创建全新项目：先初始化，成功后再导入注册。
var toolProjectCreate = mcp.NewTool("project_create",
	mcp.WithDescription("创建全新项目：先初始化 .taskboard/ 元数据，成功后再导入注册。"),
	mcp.WithString("project", mcp.Required(), mcp.Description("目标工作目录绝对路径")),
)

// handleProjectList project_list 处理器（全局，无权限/无 project 参数）。
func (s *Server) handleProjectList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	list, err := s.deps.Projects.List(ctx)
	if err != nil {
		return toolError(err)
	}
	return toolResult(list)
}

// handleProjectImport project_import 处理器（仅导入注册，要求已初始化）。
func (s *Server) handleProjectImport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workdir, err := requireProject(req.GetArguments())
	if err != nil {
		return toolError(err)
	}
	p, err := s.deps.Projects.ImportExisting(ctx, workdir)
	if err != nil {
		return toolError(err)
	}
	return toolResult(p)
}

// handleProjectInit project_init 处理器（仅初始化，不注册）。
func (s *Server) handleProjectInit(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workdir, err := requireProject(req.GetArguments())
	if err != nil {
		return toolError(err)
	}
	if err := s.deps.Projects.Init(ctx, workdir); err != nil {
		return toolError(err)
	}
	return toolResult(map[string]bool{"ok": true})
}

// handleProjectCreate project_create 处理器（先 init 后 import）。
func (s *Server) handleProjectCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workdir, err := requireProject(req.GetArguments())
	if err != nil {
		return toolError(err)
	}
	p, err := s.deps.Projects.Create(ctx, workdir)
	if err != nil {
		return toolError(err)
	}
	return toolResult(p)
}
