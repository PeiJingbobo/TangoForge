package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"tangoforge/internal/audit"
	"tangoforge/internal/auth"
	"tangoforge/internal/db"
	"tangoforge/internal/project"
	"tangoforge/internal/skill"
	"tangoforge/internal/task"
)

// newTestDeps 构造 MCP 测试依赖：临时项目（.taskboard + 默认权限）+ 业务服务全接线。
func newTestDeps(t *testing.T) (Deps, string) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	registry, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	if err := db.Migrate(context.Background(), registry, db.GlobalMigrations); err != nil {
		t.Fatalf("migrate registry: %v", err)
	}

	auditStore := audit.NewStore(logger)
	t.Cleanup(func() { _ = auditStore.Close() })
	permStore := auth.NewPermissionStore(logger)
	t.Cleanup(func() { _ = permStore.Close() })
	permStore.OnDenied = func(ctx context.Context, workdir, action string) {
		actor := auth.ActorFrom(ctx)
		auditStore.Write(ctx, workdir, audit.Entry{
			Actor: actor.Name, ActorClass: actor.Class,
			Action: action, Target: workdir, Result: audit.ResultDenied, Detail: "permission denied",
		})
	}
	taskSvc := task.NewService(task.Options{
		Logger: logger,
		OnWrite: func(ctx context.Context, workdir, action, target string) {
			actor := auth.ActorFrom(ctx)
			auditStore.Write(ctx, workdir, audit.Entry{
				Actor: actor.Name, ActorClass: actor.Class,
				Action: action, Target: target, Result: audit.ResultOK,
			})
		},
	})
	t.Cleanup(func() { _ = taskSvc.Close() })
	skillSvc := skill.NewService(logger)
	t.Cleanup(func() { _ = skillSvc.Close() })

	projSvc := project.NewService(registry, logger)
	dir := t.TempDir()
	if _, err := projSvc.Import(context.Background(), dir); err != nil {
		t.Fatalf("import project: %v", err)
	}

	return Deps{
		Logger:   logger,
		Tasks:    taskSvc,
		Projects: projSvc,
		Perms:    permStore,
		Skills:   skillSvc,
	}, dir
}

// stdioClient 手写 JSON-RPC 客户端（io.Pipe 驱动真实 stdio 传输）。
type stdioClient struct {
	in  io.WriteCloser
	out *bufio.Reader
}

func newStdioClient(t *testing.T, srv *Server) *stdioClient {
	t.Helper()
	serverInR, serverInW := io.Pipe()
	serverOutR, serverOutW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.StdioServer().Listen(ctx, serverInR, serverOutW) }()
	return &stdioClient{in: serverInW, out: bufio.NewReader(serverOutR)}
}

// send 发送 JSON-RPC 请求并等待同 id 响应。
func (c *stdioClient) send(t *testing.T, id int, method string, params any) map[string]any {
	t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	data, _ := json.Marshal(msg)
	if _, err := c.in.Write(append(data, '\n')); err != nil {
		t.Fatalf("write: %v", err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("等待响应超时")
		default:
		}
		line, err := c.out.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		if idf, ok := resp["id"].(float64); ok && int(idf) == id {
			return resp
		}
	}
}

// callTool 工具调用简写，返回 result 的 map 视图。
func callTool(t *testing.T, c *stdioClient, name string, args map[string]any) map[string]any {
	t.Helper()
	resp := c.send(t, 100, "tools/call", map[string]any{"name": name, "arguments": args})
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("响应缺少 result: %v", resp)
	}
	return result
}

// resultText 提取 result 首个 text content。
func resultText(t *testing.T, result map[string]any) string {
	t.Helper()
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("result 无 content: %v", result)
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("content 格式异常: %v", content[0])
	}
	return first["text"].(string)
}

// initialize 完成握手。
func initialize(t *testing.T, c *stdioClient) {
	t.Helper()
	resp := c.send(t, 1, "initialize", map[string]any{
		"protocolVersion": mcp.LATEST_PROTOCOL_VERSION,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test-client", "version": "1.0"},
	})
	if _, ok := resp["error"]; ok {
		t.Fatalf("initialize 失败: %v", resp)
	}
}

func TestStdio_ListTools(t *testing.T) {
	deps, _ := newTestDeps(t)
	srv := NewServer(deps)
	c := newStdioClient(t, srv)
	initialize(t, c)

	resp := c.send(t, 2, "tools/list", nil)
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list 失败: %v", resp)
	}
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) < 2 {
		t.Fatalf("工具数应 ≥ 2: %v", result)
	}
	names := make(map[string]bool)
	for _, tl := range tools {
		names[tl.(map[string]any)["name"].(string)] = true
	}
	if !names["task_read"] || !names["task_create"] {
		t.Fatalf("缺少 task_read/task_create: %v", names)
	}
}

func TestStdio_TaskCreateAndRead(t *testing.T) {
	deps, dir := newTestDeps(t)
	srv := NewServer(deps)
	c := newStdioClient(t, srv)
	initialize(t, c)

	// task_create：默认权限 task.create=false → denied（安全默认）。
	res := callTool(t, c, "task_create", map[string]any{"project": dir, "title": "MCP 任务"})
	if isErr, _ := res["isError"].(bool); !isErr {
		t.Fatalf("默认权限应拒绝 task_create: %v", res)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "PERMISSION_DENIED") {
		t.Fatalf("应返回 PERMISSION_DENIED: %s", text)
	}
}

func TestStdio_MissingProject(t *testing.T) {
	deps, _ := newTestDeps(t)
	srv := NewServer(deps)
	c := newStdioClient(t, srv)
	initialize(t, c)

	// 缺 project → 明确报错（TF-016 验收）。
	res := callTool(t, c, "task_read", map[string]any{})
	if isErr, _ := res["isError"].(bool); !isErr {
		t.Fatalf("缺 project 应报错: %v", res)
	}
	if !strings.Contains(resultText(t, res), "project") {
		t.Fatalf("错误信息应提及 project: %s", resultText(t, res))
	}
}

func TestStdio_UnknownProject(t *testing.T) {
	deps, _ := newTestDeps(t)
	srv := NewServer(deps)
	c := newStdioClient(t, srv)
	initialize(t, c)

	res := callTool(t, c, "task_read", map[string]any{"project": "/no/such/project"})
	if isErr, _ := res["isError"].(bool); !isErr {
		t.Fatalf("未导入项目应报错: %v", res)
	}
	if !strings.Contains(resultText(t, res), "PROJECT_NOT_FOUND") {
		t.Fatalf("错误码: %s", resultText(t, res))
	}
}

// TestStdio_ReadAfterGrant 授权后 task_read 可读（task.read 默认授予）。
func TestStdio_ReadAfterGrant(t *testing.T) {
	deps, dir := newTestDeps(t)
	srv := NewServer(deps)
	c := newStdioClient(t, srv)
	initialize(t, c)

	res := callTool(t, c, "task_read", map[string]any{"project": dir})
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("task.read 默认授予应成功: %s", resultText(t, res))
	}
	text := resultText(t, res)
	if !strings.Contains(text, "\"code\":0") {
		t.Fatalf("响应格式异常: %s", text)
	}
	// data 应为空数组（空项目树）。
	if !strings.Contains(text, `"data":[]`) {
		t.Fatalf("空项目树 data 应为 []: %s", text)
	}
}
