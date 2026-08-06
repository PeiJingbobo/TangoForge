# TF-016 MCP 服务框架 — 任务日志

> 日期：2026-08-06　|　执行人：ai　|　分支：`feat/TF-016-mcp-framework`

## 进展记录

### 2026-08-06（完成）
1. 引入 `github.com/mark3labs/mcp-go v0.57.0`（goproxy.cn 验证可达，纯 Go 无 CGO；go 指令 1.25.0→1.25.5）。
2. `internal/mcp/mcp.go`：Deps（业务依赖注入）+ Server（NewServer 注册工具）+ exec 统一执行骨架 + 双传输（StdioServer / HTTPHandler）+ 参数解析辅助 + toolResult/toolError。
3. `internal/mcp/tools_task.go`：task_read / task_create 工具定义（project 必填）+ handler。
4. `cmd/cli/cmd_mcp.go` + main.go mcp 分支：stdio 子命令 `tangoforge mcp --config`（独立进程自组装依赖，审计接线，无 hub）。
5. `internal/api`：/mcp 路由挂载（remoteAccessMiddleware → mcpAuthMiddleware → MCPHandler 惰性初始化复用 daemon 依赖）；`internal/auth`：PermissionStore.Require（非 HTTP 通道授权判定）+ ErrPermissionDenied + SecureEqual 导出。
6. 测试：`mcp_stdio_test.go` 5 用例（io.Pipe 驱动真实 stdio Listen：list_tools、denied、缺 project、未知项目、授权读取）+ `handlers_mcp_test.go` 3 用例（HTTP 回环成功、远程 401、denied 审计 actor=clientInfo.name）。

## 决策记录
- **库选型**（QA P4-1）：mark3labs/mcp-go v0.57.0；v0.57 API 变化：`NewMCPServer(name, version, opts...)`（非 NewServer），`StdioServer.Listen(ctx, in, out)`（非 Start），HTTP 用 `NewStreamableHTTPServer`（实现 http.Handler）。
- **双传输架构**（QA P4-1）：stdio = 独立进程直连业务层；HTTP = daemon 挂载 /mcp 复用既有依赖（taskSvc 已接 audit+hub 双通道 → MCP 写操作 WS 事件正常广播）。
- **MCP 恒为 agent**：/mcp 不经过来源识别中间件；actor = session clientInfo.name（stdioSession 与 HTTP session 均实现 SessionWithClientInfo）；X-UI-Token 在 /mcp 不构成 ui。
- **业务错误放 result**（MCP 规范）：isError=true + {code,message}，LLM 可见可自纠；HTTP 状态恒 200。
- **stdio 写操作无 WS 事件**：独立进程与 daemon 的 hub 隔离（跨进程限制），登记 TASK-SEMANTICS §16.1；远程 MCP（daemon 内）事件正常。
- **工具执行权限**：PermissionStore.Require（新增）与 HTTP RequirePermission 查同一张表，denied 触发 OnDenied → 审计，错误码一致。

## 踩坑记录
1. mcp-go v0.57 `NewServer` 不存在 → `NewMCPServer(name, version, ...)`（前两个位置参数）。
2. `toolError/toolResult` 返回两值，exec 内单值使用报编译错 → 直接 `return toolError(err)`。
3. go.sum 缺 mcp-go 传递依赖（jsonschema-go/spf13/cast/uritemplate/jsonschema）→ `go mod tidy` 补全（均为纯 Go）。
4. `secureEqual` 私有 → 导出 `SecureEqual`（api MCP 鉴权复用），identify.go/identify_test.go 同步改名。
5. 测试断言：auth.ErrProjectNotFound 在权限查询前出现 → toolError 需映射 PROJECT_NOT_FOUND（最初 INTERNAL）；空项目树 data 为 `[]`（omitempty 省略 tree 字段）。
6. 冒烟第一版 `/tmp/tf-daemon` 编译路径写错（`$GOBIN/build`）导致 curl 7；修正后 daemon 正常启动。

## 建议提交命令（经 SSH）
```bash
cd ~/HD-DATA/Coding/TangoForge
git add -A
git commit -m "feat(mcp): TF-016 MCP 双传输框架（stdio + HTTP 远程 /mcp，mark3labs/mcp-go）"
```
