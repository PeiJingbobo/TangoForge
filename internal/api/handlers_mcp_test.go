package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"tangoforge/internal/audit"
	"tangoforge/internal/config"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// mcpPost 向 /mcp 发送 JSON-RPC 请求（回环来源）。
func mcpPost(t *testing.T, srv *Server, body string, sessionID string, remote string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	if remote == "" {
		remote = "127.0.0.1:5555"
	}
	req.RemoteAddr = remote
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec, resp
}

// mcpInitialize 完成握手，返回 session id。
func mcpInitialize(t *testing.T, srv *Server, remote string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": mcp.LATEST_PROTOCOL_VERSION,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "http-client", "version": "1.0"},
		},
	})
	rec, resp := mcpPost(t, srv, string(body), "", remote)
	if rec.Code != http.StatusOK {
		t.Fatalf("initialize: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := resp["error"]; ok {
		t.Fatalf("initialize 失败: %v", resp)
	}
	sid := rec.Header().Get("Mcp-Session-Id")
	if sid == "" {
		t.Fatalf("initialize 响应缺少 Mcp-Session-Id: headers=%v", rec.Header())
	}
	// initialized 通知（无响应体）。
	notif, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	mcpPost(t, srv, string(notif), sid, remote)
	return sid
}

// mcpCall 调用工具，返回 result 文本。
func mcpCall(t *testing.T, srv *Server, sid string, args map[string]any, remote string) (string, bool) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": "task_read", "arguments": args},
	})
	rec, resp := mcpPost(t, srv, string(body), sid, remote)
	if rec.Code != http.StatusOK {
		t.Fatalf("call: code=%d body=%s", rec.Code, rec.Body.String())
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("响应缺少 result: %v", resp)
	}
	content := result["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	isErr, _ := result["isError"].(bool)
	return text, isErr
}

func TestMCPHTTP_LoopbackRead(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	sid := mcpInitialize(t, srv, "127.0.0.1:5555")
	text, isErr := mcpCall(t, srv, sid, map[string]any{"project": dir}, "")
	if isErr {
		t.Fatalf("task_read 应成功: %s", text)
	}
	if !strings.Contains(text, `"code":0`) {
		t.Fatalf("响应异常: %s", text)
	}
}

func TestMCPHTTP_RemoteNoToken401(t *testing.T) {
	srv := newAPIServer(t, func(cfg *config.GlobalConfig) {
		cfg.RemoteAccess = true // 远程放行，测试 MCP 通道自身的 Bearer 校验。
	})
	defer func() { _ = srv.Close() }()
	_ = importProjectViaAPI(t, srv)

	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": mcp.LATEST_PROTOCOL_VERSION,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "remote-client", "version": "1.0"},
		},
	})
	rec, _ := mcpPost(t, srv, string(body), "", "192.168.1.10:4444")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("远程无 Bearer 应 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMCPHTTP_DeniedAudit(t *testing.T) {
	srv := newAPIServer(t, nil)
	defer func() { _ = srv.Close() }()
	dir := importProjectViaAPI(t, srv)

	sid := mcpInitialize(t, srv, "127.0.0.1:5555")

	// MCP 建任务（默认权限 task.create=false）→ 工具级错误 + denied 审计。
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{"name": "task_create", "arguments": map[string]any{"project": dir, "title": "x"}},
	})
	rec, resp := mcpPost(t, srv, string(body), sid, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("工具调用 HTTP 应为 200（业务错误在 result 内）: %d", rec.Code)
	}
	result := resp["result"].(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Fatalf("应返回业务错误: %v", result)
	}
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "PERMISSION_DENIED") {
		t.Fatalf("应含 PERMISSION_DENIED: %s", text)
	}

	// 等待异步审计落库后验证 denied 记录。
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		res, err := srv.audit.Query(context.Background(), dir, audit.Filter{Action: "task.create"})
		if err == nil && len(res.Items) > 0 {
			if res.Items[0].Result != audit.ResultDenied {
				t.Fatalf("审计 result 应为 denied: %+v", res.Items[0])
			}
			if res.Items[0].Actor != "http-client" {
				t.Fatalf("审计 actor 应为 clientInfo.name: %+v", res.Items[0])
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("等待 denied 审计超时")
}
