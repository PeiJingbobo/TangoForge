// Package mcp 是 TangoForge 的 MCP 传输层（QA P4-1：stdio 与 HTTP 双传输）。
//
// 架构（分层铁律 AGENTS.md §3.2：本包为传输层薄封装，业务实现全部来自
// task / project / skill 等业务层）：
//   - 工具注册表：固定 v1 工具集（REQUIREMENTS.md §8.3），每个工具首参 project 强制；
//   - 统一执行骨架 exec()：从 MCP 会话取 actor（clientInfo.name，agent）→
//     参数解析 → 权限校验（PermissionStore.Require，与 HTTP 等价查同一张表）→
//     业务调用 → JSON 文本返回（结构对齐 HTTP data 字段）；
//   - 双传输：stdio（cmd/cli 子命令 `tangoforge mcp`，独立进程直连业务层）与
//     Streamable HTTP（daemon 挂载 /mcp，复用 daemon 依赖，写操作事件经既有钩子广播）。
//
// 本包禁止引用 internal/api（避免 api↔mcp 循环；daemon 侧由 api 层组装 Deps 后
// 调用 HTTPHandler() 挂载，stdio 侧由 cmd 层组装 Deps 后调用 StdioServer().Listen）。
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"tangoforge/internal/auth"
	"tangoforge/internal/exporter"
	"tangoforge/internal/parser"
	"tangoforge/internal/project"
	"tangoforge/internal/skill"
	"tangoforge/internal/task"
)

// Deps MCP 工具执行所需的业务依赖（由调用方组装注入：
// daemon 侧复用 api.Server 既有实例，stdio 侧 cmd 层自建）。
type Deps struct {
	Logger   *slog.Logger
	Tasks    task.Service
	Projects *project.Service
	Perms    *auth.PermissionStore
	Skills   *skill.Service
	Parser   *parser.Service
	Exporter *exporter.Service
}

// Server MCP 服务：持有业务依赖与已注册工具。
type Server struct {
	deps   Deps
	mcpSrv *server.MCPServer
}

// NewServer 构造 MCP 服务并注册 v1 工具集。
func NewServer(deps Deps) *Server {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	s := &Server{deps: deps}
	s.mcpSrv = server.NewMCPServer("TangoForge", "1.0.0",
		server.WithInstructions("人机协作任务看板 — Agent 任务池操作（v1 固定工具集）。所有工具第一个参数均为 project（工作目录绝对路径，强制）。"),
	)
	s.registerTools()
	return s
}

// registerTools 注册 v1 固定工具集（REQUIREMENTS.md §8.3 + QA P4-1 Q6 扩展）。
func (s *Server) registerTools() {
	// task 域。
	s.mcpSrv.AddTool(toolTaskRead, s.handleTaskRead)
	s.mcpSrv.AddTool(toolTaskList, s.handleTaskList)
	s.mcpSrv.AddTool(toolTaskCreate, s.handleTaskCreate)
	s.mcpSrv.AddTool(toolTaskUpdate, s.handleTaskUpdate)
	s.mcpSrv.AddTool(toolTaskArchive, s.handleTaskArchive)
	s.mcpSrv.AddTool(toolTaskRestore, s.handleTaskRestore)
	// project 域（QA P4-1 Q6：import 仅导入 / init 仅初始化 / create 先 init 后 import）。
	s.mcpSrv.AddTool(toolProjectList, s.handleProjectList)
	s.mcpSrv.AddTool(toolProjectImport, s.handleProjectImport)
	s.mcpSrv.AddTool(toolProjectInit, s.handleProjectInit)
	s.mcpSrv.AddTool(toolProjectCreate, s.handleProjectCreate)
	// import / export 域。
	s.mcpSrv.AddTool(toolImportPreview, s.handleImportPreview)
	s.mcpSrv.AddTool(toolImportConfirm, s.handleImportConfirm)
	s.mcpSrv.AddTool(toolImportDiscard, s.handleImportDiscard)
	s.mcpSrv.AddTool(toolExportMarkdown, s.handleExportMarkdown)
	// 只读查询域。
	s.mcpSrv.AddTool(toolGraphGet, s.handleGraphGet)
	s.mcpSrv.AddTool(toolStateMachineGet, s.handleStateMachineGet)
	s.mcpSrv.AddTool(toolStateMachineUpdate, s.handleStateMachineUpdate)
	s.mcpSrv.AddTool(toolSkillInfo, s.handleSkillInfo)
	s.mcpSrv.AddTool(toolSkillInstall, s.handleSkillInstall)
	s.mcpSrv.AddTool(toolSkillStatus, s.handleSkillStatus)
	s.mcpSrv.AddTool(toolSkillUninstall, s.handleSkillUninstall)
	s.mcpSrv.AddTool(toolPermissionList, s.handlePermissionList)
	// TF-033 guide 说明书工具（免鉴权，无 project 参数）。
	s.mcpSrv.AddTool(toolGuide, s.handleGuide)
}

// StdioServer 返回 stdio 传输服务（cmd 层调用 Listen(ctx, stdin, stdout)）。
func (s *Server) StdioServer() *server.StdioServer {
	return server.NewStdioServer(s.mcpSrv)
}

// HTTPHandler 返回 Streamable HTTP 传输 handler（daemon 挂载 /mcp，经远程访问/鉴权中间件）。
func (s *Server) HTTPHandler() http.Handler {
	return server.NewStreamableHTTPServer(s.mcpSrv)
}

// toolResult 成功返回：data 序列化为 JSON 文本（对齐 HTTP data 结构）。
func toolResult(data any) (*mcp.CallToolResult, error) {
	body, err := json.Marshal(map[string]any{"code": 0, "data": data})
	if err != nil {
		return toolError(fmt.Errorf("mcp: 序列化结果: %w", err))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(string(body))},
	}, nil
}

// toolError 错误返回：{code, message} JSON 文本 + isError=true（MCP 规范：业务错误
// 放在 result 内由 LLM 可见，而非协议级错误）。
func toolError(err error) (*mcp.CallToolResult, error) {
	code := "INTERNAL"
	msg := err.Error()
	var te *task.Error
	if errors.As(err, &te) {
		code = te.Code
	} else if errors.Is(err, auth.ErrPermissionDenied) {
		code = "PERMISSION_DENIED"
	} else if errors.Is(err, auth.ErrProjectNotFound) {
		code = "PROJECT_NOT_FOUND"
	}
	body, _ := json.Marshal(map[string]string{"code": code, "message": msg})
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(string(body))},
		IsError: true,
	}, nil
}

// exec 工具执行公共骨架：
//  1. actor = MCP 会话 clientInfo.name（agent），写入 ctx（审计/写钩子复用）；
//  2. project 参数必填解析；
//  3. 权限校验（PermissionStore.Require，denied → 审计 + 错误）；
//  4. 业务函数执行（fn 返回 data 或 error）。
func (s *Server) exec(ctx context.Context, action string, args map[string]any,
	fn func(ctx context.Context, workdir string, args map[string]any) (any, error)) (*mcp.CallToolResult, error) {

	actor := actorFromSession(ctx)
	ctx = auth.WithActor(ctx, auth.FromMCP(actor))

	workdir, err := requireProject(args)
	if err != nil {
		return toolError(err)
	}

	if err := s.deps.Perms.Require(ctx, workdir, action); err != nil {
		return toolError(err)
	}

	data, err := fn(ctx, workdir, args)
	if err != nil {
		return toolError(err)
	}
	return toolResult(data)
}

// actorFromSession 从 MCP 会话上下文取客户端名（initialize clientInfo.name；缺省 unknown）。
func actorFromSession(ctx context.Context) string {
	if sess, ok := server.ClientSessionFromContext(ctx).(server.SessionWithClientInfo); ok {
		if info := sess.GetClientInfo(); info.Name != "" {
			return info.Name
		}
	}
	return "unknown"
}

// --- 参数解析辅助 ---

// requireProject 提取并校验 project 参数（强制必填，QA Q4/TF-016 验收）。
func requireProject(args map[string]any) (string, error) {
	v, ok := args["project"]
	if !ok {
		return "", errors.New("TASK_INVALID: 缺少必填参数 project（工作目录绝对路径）")
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", errors.New("TASK_INVALID: 参数 project 必须为非空字符串")
	}
	return s, nil
}

// strArg 提取字符串参数；缺省返回默认值。
func strArg(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return def
}

// strArrayArg 提取字符串数组参数；缺省返回 nil。
func strArrayArg(args map[string]any, key string) []string {
	raw, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
