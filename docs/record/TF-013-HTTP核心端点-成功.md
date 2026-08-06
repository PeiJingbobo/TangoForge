# TF-013 HTTP API 核心端点 — 任务总结
> 结果：成功　|　日期：2026-08-06　|　执行人：ai

## 1. 任务范围
在 daemon 骨架（TF-003）之上挂载 HTTP 核心端点：projects（豁免 X-Project）/ tasks / state-machine / permissions，统一 `{code,data}` 响应与错误码映射；全链路集成冒烟（项目→任务→流转→归档→还原→审计）。

## 2. 交付内容
- **新增文件**：
  - `internal/api/routes.go`（合并入 server.go）— 路由挂载与中间件链：`remoteAccess → [/api/projects 组：identify] / [主组：project → identify → action 权限]`
  - `internal/api/handlers_projects.go` — GET /api/projects、POST /api/projects/import（写审计 project.imported）、DELETE /api/projects/:id（仅 UI）
  - `internal/api/handlers_tasks.go` — 任务 CRUD + PATCH（status 独立走 ChangeStatus + task.update_status 二次校验）+ archive/restore/delete + `ensureAction`
  - `internal/api/handlers_state_machine.go` — GET/PUT /api/state-machine
  - `internal/api/handlers_permissions.go` — GET /api/permissions（全量 16 项）、PUT /api/permissions（仅 UI，写审计 permission.changed）
  - `internal/api/errors.go` — `mapError` 错误码→HTTP 映射（QA P3-7）
  - `internal/api/handlers_test.go` — 11 个集成冒烟用例
- **修改文件**：
  - `internal/api/server.go` — Server 自组装依赖（task/project/perms/audit + 审计接线）+ `perm` 辅助 + `Close`；IdentifyMiddleware 改为配置提供者模式（热重载即时生效）
  - `internal/api/middleware.go` — projectMiddleware 将规范化 workdir 写入 ctx（auth.WithWorkdir）
  - `internal/auth/middleware.go` — IdentifyMiddleware 签名改为 `func() *config.GlobalConfig`（热重载）
  - `internal/task/archive.go` — **Archive 事务策略修复**（见 §3 遗留）：事务外读快照、事务内首条语句即写，消除 WAL 写升级 SQLITE_BUSY
- **关键实现点**：
  1. **审计接线**（QA P3-1）：task 写钩子 → audit（result=ok）；权限中间件 OnDenied → audit（result=denied）；actor 从 ctx 读取
  2. **PUT /api/permissions 仅 UI**：actor==ui 校验（回环+X-UI-Token 由识别层保证），Agent 403
  3. **PATCH 动态权限**：body 含 status → 二次校验 task.update_status（ensureAction），未授权 403+denied 审计
  4. **错误码映射**：404（PROJECT/TASK/PARENT_NOT_FOUND）、400（TASK_INVALID/DELETE_NOT_ALLOWED/PROJECT_INVALID）、422（INVALID_TRANSITION/STATUS_IN_USE/STATUS_NOT_FOUND/PARENT_CYCLE/DEPENDENCY_NOT_FOUND/CIRCULAR_DEPENDENCY）
  5. **projects 组豁免 X-Project**（QA P3-2），DELETE 仅 UI

## 3. 验证结果
- `CGO_ENABLED=0 go test ./...` → 全仓全绿（7 包 ok）；`go vet ./...` 干净
- `internal/task` 覆盖率 **92.3%**（> 90% 门槛）
- 全链路冒烟（TestAPIFullChain_UI）：建任务→列表→流转→非法流转 422→归档→还原→物理删除→审计落库，连续 3 次稳定通过
- 安全用例：agent 默认 task.create 403 + denied 审计、PUT 权限仅 UI、state_machine.read 默认 403、远程无 Token 401、远程有效 Token 放行、依赖环 422

## 4. 遗留问题与后续
- **WAL 写锁问题修复（重要）**：archive 原为先读后写的 DEFERRED 事务，WAL 下遇 audit 并发写时"读快照已建、写升级即失败"→ 立即 SQLITE_BUSY（busy handler 不适用）。修复：事务外读快照（幂等短路保留）、事务内首条语句即写（获取写锁阶段应用 busy_timeout）。依赖 modernc.org/sqlite 行为已验证。
- `GET /api/audit` / `GET /api/audit/export` 端点挂载在 TF-014。
- `PATCH /api/tasks/:id` 同时含 status 与其他字段时先字段后状态（两次写、两次审计），语义待前端接入时验证。
