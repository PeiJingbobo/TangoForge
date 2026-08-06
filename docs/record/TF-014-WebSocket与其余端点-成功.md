# TF-014 WebSocket 与其余端点 — 任务总结
> 结果：成功　|　日期：2026-08-06　|　执行人：ai

## 1. 任务范围
实现 `/ws/events` 事件推送（task.* 等写操作事件）、`GET /api/graph` 全景图、`GET /api/audit` 查询与导出；import/export/skill 端点注册**占位**（QA P3-3：依赖 P4 TF-018/019/020，返回 NOT_IMPLEMENTED）。**M3 里程碑达成：API 可用，curl 全链路 + WS 事件可达。**

## 2. 交付内容
- **新增文件**：
  - `internal/api/ws.go` — hub 事件广播（按 project 分组，慢消费者保护）+ `/ws/events` 独立鉴权链（来源过滤→项目校验→识别→task.read 权限→升级订阅）+ 读写泵（ping/pong 保活）
  - `internal/api/handlers_graph.go` — GET /api/graph（graph.read）：全量任务扁平列表 + parent/dependency 边，服务端不聚簇（REQUIREMENTS §6）
  - `internal/api/handlers_audit.go` — GET /api/audit（filter[actor]/[action] + page/size，audit.read）、GET /api/audit/export（text/plain，含表头，不写盘 QA P3-8）
  - `internal/api/handlers_placeholder.go` — import/export/skill 8 个占位端点（NOT_IMPLEMENTED）
  - `internal/api/handlers_ws_test.go` — 8 用例
- **修改文件**：
  - `internal/api/server.go` — Server 增加 hub；写钩子双通道（audit + hub.Publish）；挂载 `/ws/events` + graph/audit/占位路由
  - `go.mod` / `go.sum` — 新增 `github.com/gorilla/websocket v1.5.3`（纯 Go，零信任约束满足，QA P3-4）
- **关键实现点**：
  1. **事件结构** `{type, project, data, ts}`：type 复用写钩子 action（task.created 等），data={id}，project=workdir（REQUIREMENTS §5.3）
  2. **WS 独立鉴权**（不在 /api 树下）：remote_access → 项目注册 → 识别（远程 401）→ task.read（未授权 403）→ 升级
  3. **慢消费者保护**：发送缓冲 64，写满即断开该连接（防阻塞广播）
  4. **graph 不聚簇**：nodes（排除 archived）+ edges（parent/dependency 方向语义 TASK-SEMANTICS §9）

## 3. 验证结果
- `go test ./internal/api/...` → ok（含 WS 事件接收、agent 订阅、404/403 鉴权、graph 全量、audit 查询/导出、占位 501）
- `CGO_ENABLED=0 go test ./...` → 全仓全绿；`go vet ./...` 干净；`internal/task` 覆盖率 **92.3%**
- **M3 真实 daemon 冒烟（全部通过）**：
  1. `/ping` → 200
  2. 导入项目 → 200（含 .taskboard 初始化）
  3. 建任务 → 201（priority high→4）
  4. 流转 todo→doing → 200
  5. 非法流转 doing→archived → **422**
  6. 归档→还原 → 200
  7. 审计查询 → task.created/status_changed/archived/restored 全部落库（actor=ui）
  8. agent 建任务 → **403**（默认权限 denied）
  9. 审计导出 → 表头 + 全记录（含 project.imported）
  10. **WS 事件**：连接成功 → 触发建任务 → 实时收到 `task.created` 事件 ✓

## 4. 遗留问题与后续
- import/export/skill 占位端点待 P4：TF-018（parser 草稿流）/ TF-019（exporter）/ TF-020（skill）落地时替换 handler。
- WS 事件仅含 `{id}`（写钩子只有 target），前端需要时可再拉详情；跨项目事件隔离已实现（按 project 分组）。
- WS 远程 401 未单独集成测试（httptest 仅回环；逻辑与 /api 共用 auth.Identify，已由 TestAPI_RemoteNoToken401 覆盖）。
