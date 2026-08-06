# TF-016 MCP 服务框架 — 任务总结
> 结果：成功　|　日期：2026-08-06　|　执行人：ai

## 1. 任务范围
实现 MCP 服务框架（QA P4-1 扩展：stdio + HTTP 双传输），mark3labs/mcp-go v0.57.0（纯 Go）；工具注册表与统一执行骨架；先落地 `task_read` / `task_create`（project 参数强制）；MCP 恒为 agent 身份、权限与 HTTP 等价。

## 2. 交付内容
- **新增文件**：
  - `internal/mcp/mcp.go` — Deps（业务依赖注入）+ Server（NewServer 注册工具、StdioServer/HTTPHandler 双传输）+ `exec` 统一执行骨架（actor=clientInfo.name → project 必填 → Perms.Require → 业务调用）+ `toolResult`/`toolError`（业务错误放 result isError）+ 参数解析辅助
  - `internal/mcp/tools_task.go` — `task_read`（id 详情 / 树 + status/q 过滤）、`task_create`（title 必填、priority 别名、tags/depends_on/parent_id）
  - `internal/mcp/mcp_stdio_test.go` — 5 用例（io.Pipe 驱动真实 stdio Listen：list_tools、task_create 默认 denied、缺 project 报错、未导入项目 PROJECT_NOT_FOUND、授权 task_read 空树）
  - `cmd/cli/cmd_mcp.go` — stdio 子命令（`tangoforge mcp --config`，独立进程自组装依赖：audit/perms/task/project/skill，写钩子审计接线）
  - `internal/api/handlers_mcp_test.go` — 3 用例（HTTP 回环 initialize+call 成功、远程无 Bearer 401、denied 审计 actor=http-client）
- **修改文件**：
  - `internal/api/server.go` — `/mcp` 路由（remoteAccess → mcpAuthMiddleware → MCPHandler 惰性初始化复用 daemon 依赖）、`mcpAuthMiddleware`（远程必须 Bearer）
  - `internal/auth` — `PermissionStore.Require`（非 HTTP 通道授权判定 + OnDenied 触发）、`ErrPermissionDenied`、`SecureEqual` 导出（token.go/identify.go）
  - `cmd/cli/main.go` — mcp 子命令分支与 usage
  - `go.mod`/`go.sum` — mcp-go v0.57.0 + 传递依赖（jsonschema-go/spf13/cast/uritemplate/jsonschema，均纯 Go）
  - `docs/TASK-SEMANTICS.md` — 新增 §16（MCP 传输层语义）
  - `docs/task/TASKS.md` / `OVERVIEW.md` — 状态 ✅、统计同步
- **关键实现点**：
  1. 双传输共享同一工具注册与 exec 骨架；HTTP 模式复用 daemon 依赖（写操作 WS 事件正常广播）
  2. MCP 通道不识别 UI 凭据；actor 取会话 clientInfo.name（agent）
  3. 权限与 HTTP 查同一 permissions 表（Require），denied → 审计 + PERMISSION_DENIED
  4. 业务错误放 result（isError=true），HTTP 状态恒 200（MCP 规范）

## 3. 验证结果
- `CGO_ENABLED=0 go build ./...` → 通过；`go vet ./...` → 干净
- `CGO_ENABLED=0 go test ./internal/mcp/... ./internal/api/...` → ok（stdio 5 用例 + HTTP 3 用例）
- `CGO_ENABLED=0 go test ./...` → **全仓全绿**
- **真实 daemon 冒烟**：`POST /mcp` initialize → 200 + `Mcp-Session-Id`；带 session `tools/list` → 返回 task_read/task_create 工具声明（inputSchema 完整）

## 4. 遗留问题与后续
- stdio 独立进程写操作不推送 WS 事件（跨进程限制，登记 §16.1）；远程 MCP（daemon 内）事件正常。
- TF-017 补齐剩余工具（project/task 全集/import/export/graph/state_machine/skill/permission）。
- TF-021 CLI 子命令与 mcp 子命令同二进制共存，需在 TF-021 验收时确认无冲突。
- `skill.changed`/`import.*`/`export.*` 事件（TF-018/019/020 相关）在 MCP 写路径的覆盖留待对应任务。
