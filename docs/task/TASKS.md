# TangoForge 可执行任务清单（Task Backlog）

> **版本**：v1.0（2026-08-05）
> **定位**：本文件是**唯一可执行的任务来源**。开发时一次取一个任务，做完按 §4 更新状态并产出记录（总结/日志）。
> **配套**：执行策略与完成定义见 `docs/task/MASTER-PLAN.md` §5；进度看板见 `docs/task/OVERVIEW.md`。
> **状态枚举**：`待开始 / 进行中 / 已完成 / 阻塞 / 废弃`。任务编号一经分配不重用。

---

## 状态速览

| 阶段 | 任务数 | 已完成 | 进行中 | 待开始 |
|------|--------|--------|--------|--------|
| P1 基础设施与数据层 | 4 | 4 | 0 | 0 |
| P2 任务核心域 | 5 | 5 | 0 | 0 |
| P3 传输层与安全 | 5 | 5 | 0 | 0 |
| P4 Agent 能力 | 7 | 7 | 0 | 0 |
| P5 前端应用 | 7 | 7 | 0 | 0 |
| P5.5 项目设置页 | 1 | 1 | 0 | 0 |
| P6 测试与交付 | 3 | 0 | 0 | 3 |
| **合计** | **32** | **29** | **0** | **3** |

> 每完成一个任务，更新本表、OVERVIEW.md 统计，并新建 `docs/record/TF-XXX-<标题>-<结果>.md` 总结与 `docs/log/TF-XXX-<标题>.md` 日志。

---

## P1 基础设施与数据层（M1：数据层可用）

### TF-001 数据库迁移脚本（P0，无依赖）

- **涉及模块**：`internal/db`、`cmd/daemon`
- **描述**：实现 `internal/db` 迁移框架（`Migrate` up/down + schema 版本管理），按 `REQUIREMENTS.md §四.4` 的 DDL 创建全部 6 表（`projects / tasks / permissions / import_drafts / skills / audit_log`）及索引；守护进程启动时自动建库建表（`{workdir}/.taskboard/meta.db`）。全部代码 `CGO_ENABLED=0` 可编译。
- **验收标准**：
  - [x] `go test ./internal/db/...` 全绿（`sqlite:memory:` 隔离，不依赖文件系统）
  - [x] 迁移可重复执行（幂等）；up/down 双向可用
  - [x] 用临时目录初始化后表/索引可查（Go 断言替代 sqlite3 CLI；双库模型：全局库 projects 表 + 项目库 5 表）
  - [x] `CGO_ENABLED=0 go build ./...` 通过
- **状态**：已完成（2026-08-05）
- **总结文件**：`docs/record/TF-001-数据库迁移脚本-成功.md`

### TF-002 配置加载与热重载（P0，依赖 TF-001）

- **涉及模块**：`internal/config`
- **描述**：实现双层配置：全局配置 `~/.taskboard-app/config.yaml`（端口、LLM、remote_access、api_token、ui_token）+ 项目配置 `{workdir}/.taskboard/config.yaml`（state_machine、export）；提供默认值、合并逻辑、`fsnotify` 热重载（端口/remote_access/LLM 即时生效）。
- **验收标准**：
  - [x] 单测覆盖：默认值、文件加载、双层合并、缺失文件容错
  - [x] 热重载单测：修改全局配置后内存配置同步更新
  - [x] 配置加载不做业务判断（纯加载合并，遵守分层）
- **状态**：已完成（2026-08-05）
- **总结文件**：`docs/record/TF-002-配置加载与热重载-成功.md`
- **产出文件**：`internal/config/config.go`、`internal/config/global.go`、`internal/config/project.go`、`internal/config/config_test.go`

### TF-003 守护进程骨架（P0，依赖 TF-001、TF-002）

- **涉及模块**：`cmd/daemon`、`internal/api`
- **描述**：守护进程常驻骨架：单实例锁（监听端口 + PID 文件双重检测）、监听 `0.0.0.0:19810`（端口可热重载）、`GET /ping` 健康检查、来源过滤中间件（`remote_access=false` 时非回环 403）、`X-Project` 解析与未注册项目返回 `PROJECT_NOT_FOUND`。
- **验收标准**：
  - [x] 启动后 `curl http://127.0.0.1:19810/ping` 返回 200
  - [x] 模拟非回环来源请求被 403 拒绝
  - [x] 携带未注册 `X-Project` 的 `/api/*` 请求返回 `PROJECT_NOT_FOUND`
  - [x] 二次启动被单实例锁拦截（附加热重载：端口 19810→20001 动态重绑且进程常驻）
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-003-守护进程骨架-成功.md`
- **产出文件**：`cmd/daemon/main.go`（改造）、`internal/api/server.go`、`internal/api/middleware.go`

### TF-004 项目管理服务（P1，依赖 TF-001、TF-002）

- **涉及模块**：`internal/task`（或独立 `internal/project` 子包）、`internal/config`
- **描述**：项目注册表业务层：导入目录为项目（无 `.taskboard/` 则初始化：建目录 + `meta.db` + 默认 `config.yaml` + 默认状态机与默认 Agent 只读权限）；项目列表；移除记录（**绝不删除磁盘数据**）；`last_opened_at` 维护。
- **验收标准**：
  - [x] 单测覆盖：导入/列表/移除/重复导入；移除后磁盘数据完好
  - [x] 初始化时写入默认状态机与默认权限（`task.read/graph.read/skill.read/project.read/permission.read=true`）
  - [x] 与 TF-003 联调：导入目录后 X-Project 校验放行（真实 daemon 端到端）
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-004-项目管理服务-成功.md`
- **产出文件**：`internal/project/project.go`、`internal/project/project_test.go`（独立子包，QA Q10）

---

## P2 任务核心域（M2：任务引擎可用）

### TF-005 任务基础 CRUD（P0，依赖 TF-001、TF-004）

- **涉及模块**：`internal/task`
- **描述**：Task 模型（已有 `task.go`）之上的 CRUD：创建（UUID v4、默认 `todo`、priority 字符串别名归一化）、读取（详情 + 树形返回 + `filter[status]`/`q`/分页）、更新（详情字段，禁止 status，Q8 独立 `ChangeStatus`）、删除语义（见 TF-007）。先定义 Interface，三端共享。项目识别以 `{workdir}/.taskboard/meta.db` 元数据为准（Q2-B，不依赖全局注册表）；语义细节见 `docs/TASK-SEMANTICS.md`。
- **验收标准**：
  - [x] 单测覆盖：Create/Get/List（树形、过滤、搜索、分页）/Update 全路径（含 ChangeStatus、parent 环、写钩子）
  - [x] 列表排序 `priority DESC, created_at ASC`（同值 `id ASC` 稳定）
  - [x] 项目隔离：跨项目数据互不可见（一项目一库文件）
- **产出文件**：`internal/task/service.go`、`internal/task/service_test.go`、`internal/task/repo.go`、`internal/task/errors.go`、`internal/task/priority.go`、`app/src/types/task.ts`、`docs/TASK-SEMANTICS.md`（新增）
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-005-任务基础CRUD-成功.md`
- **覆盖率**：`internal/task` 91.3%（> 90% 门槛）

### TF-006 状态机校验（P0，依赖 TF-005）

- **涉及模块**：`internal/task`
- **描述**：项目级可配置状态机：从项目 `config.yaml` 的 `state_machine` 节加载（默认四态 `todo/doing/done` + 系统保留 `archived`）；状态流转校验（非法流转 `INVALID_TRANSITION`）；状态机编辑时**有任务占用的状态不可移除/重命名**（`STATUS_IN_USE`，返回占用任务数）。流转校验语义（QA Q1-B 宽松 / Q3-A 空规则特例 / Q2-A 同态幂等）见 `docs/TASK-SEMANTICS.md` §5.1/§5.2。
- **验收标准**：
  - [x] 单测覆盖：合法/非法流转矩阵、回退允许、archived 不可参与普通流转（含 Q1-B/Q3-A/Q2-A 语义用例）
  - [x] 占用状态移除被拒，错误携带占用数（STATUS_IN_USE，Message 内嵌数量）
  - [x] 状态机配置缺失时回退默认四态
- **产出文件**：`internal/task/state_machine.go`、`internal/task/state_machine_test.go`、`internal/task/errors.go`（追加 INVALID_TRANSITION / STATUS_IN_USE）、`internal/task/service.go`（ChangeStatus 接入流转校验 + Get/UpdateStateMachine）
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-006-状态机校验-成功.md`
- **覆盖率**：`internal/task` 90.7%（> 90% 门槛）

### TF-007 归档 / 还原 / 物理删除（P0，依赖 TF-005）

- **涉及模块**：`internal/task`
- **描述**：删除语义落地：归档（`status=archived` + 记录 `archived_from`，**幂等 Q2-B**）；还原（回到 `archived_from`，缺失回退 todo，目标状态已删除可 `FallbackTodo`）；物理删除仅限回收站（archived）任务；**归档/物理删除父任务时子任务级联置空 `parent_id`**（事务原子，`ChildrenCleared` 返回数量）；物理删除子任务严格禁止（Q8-A：仅级联置空，不提供级联删除）；归档被依赖任务时返回"被 N 个任务依赖"提示（`DependentCount`，不阻断）。删除语义细节见 `docs/TASK-SEMANTICS.md` §8。
- **验收标准**：
  - [x] 单测覆盖：归档/还原往返、级联置空（含孙任务不受影响）、回收站物理删除、删除被依赖任务的提示
  - [x] 物理删除非 archived 任务被拒绝（`DELETE_NOT_ALLOWED`）
  - [x] 幂等归档（Q2-B）、`RestoreOptions.FallbackTodo`（Q5）、事务原子
- **产出文件**：`internal/task/archive.go`、`internal/task/archive_test.go`、`internal/task/repo.go`（dbtx 事务化 + Delete + ClearParentsByParentID）、`internal/task/errors.go`（追加 DELETE_NOT_ALLOWED）、`internal/task/service.go`（Archive/Restore/Delete 接口）
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-007-归档还原物理删除-成功.md`
- **覆盖率**：`internal/task` 88.8%（错误分支缺口登记至 TF-009 收口）

### TF-008 依赖关系与无环校验（P1，依赖 TF-005）

- **涉及模块**：`internal/task`
- **描述**：`depends_on` JSON 数组读写；Create/Update 时循环依赖检测（环 → `CIRCULAR_DEPENDENCY`，写操作直接拒绝）；依赖任务不存在时校验策略明确（`DEPENDENCY_NOT_FOUND` 报错）。方向语义（A 依赖 B）、多跳环检测、校验先于写入等细节见 `docs/TASK-SEMANTICS.md` §9。
- **验收标准**：
  - [x] 单测覆盖：合法依赖、A→B→A 环拒绝、自依赖拒绝（含多跳环、依赖不存在、依赖 archived 允许）
  - [x] 依赖校验在写入前完成（单语句原子，语义等价事务），拒绝时不产生脏数据
- **产出文件**：`internal/task/dependency.go`、`internal/task/dependency_test.go`、`internal/task/errors.go`（追加 DEPENDENCY_NOT_FOUND / CIRCULAR_DEPENDENCY）、`internal/task/service.go`（Create/Update 接入）
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-008-依赖关系与无环校验-成功.md`

### TF-009 任务域覆盖率收口（P0，依赖 TF-005~TF-008）

- **涉及模块**：`internal/task`（含测试）
- **描述**：补全 P2 全部边界测试，使 `internal/task` 行覆盖率 ≥ 90%（含状态机、归档/还原、依赖校验、草稿导入流程），本地跑 `scripts/check_coverage.sh` 通过。错误分支注入策略（Q9-A）：meta.db 目录注入 projectDB 失败、关闭连接注入 repo 错误、config.yaml 损坏注入状态机加载失败。
- **验收标准**：
  - [x] `./scripts/check_coverage.sh` 通过（THRESHOLD=90）
  - [x] `CGO_ENABLED=0 go test -cover ./internal/task/...` 覆盖率 ≥ 90%（实测 92.1%）
- **产出文件**：`internal/task/coverage_test.go`（错误注入补测）
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-009-任务域覆盖率收口-成功.md`

> **✅ M2 里程碑达成（2026-08-06）**：P2 任务核心域全部完成，`internal/task` 覆盖率 92.1%（> 90% 门槛），质量门禁打勾见 MASTER-PLAN §7。

---

## P3 传输层与安全（M3：API 可用）

### TF-010 来源识别中间件（P0，依赖 TF-002）

- **涉及模块**：`internal/auth`
- **描述**：按 `REQUIREMENTS.md §7.2` 实现 5 级来源判定：`X-UI-Token`+回环 → ui；MCP stdio → agent（客户端名）；远程 Bearer Token → agent（X-Actor 或 unknown）；本地 X-Actor（CLI 默认 human）→ agent；无法识别 → unknown。识别结果供权限中间件与审计使用。
- **验收标准**：
  - [x] 单测覆盖 5 级判定全路径（含回环/非回环、Token 缺失/错误）
  - [x] UI 凭据仅回环有效；远程无 Token → 401
- **产出文件**：`internal/auth/identify.go`、`internal/auth/identify_test.go`、`internal/auth/token.go`
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-010-来源识别中间件-成功.md`

### TF-011 权限模型与中间件（P0，依赖 TF-010、TF-004）

- **涉及模块**：`internal/auth`、`internal/task`（permissions 读写）
- **描述**：`permissions` 表（`project_id, action, allowed`）读写；新项目默认只读权限（TF-004 已建）；中间件：`actor_class=ui` 直接放行，其余查表，未授权 403；`PUT /api/permissions` 仅 UI 凭据 + 回环（Agent 不可改权限表）；`GET /api/permissions` 返回自身范围。
- **验收标准**：
  - [x] 单测：默认权限、授权/未授权矩阵、UI 放行、权限修改端点双条件校验
  - [x] MCP/CLI 请求无法修改权限表（403）
- **产出文件**：`internal/auth/permission.go`、`internal/auth/permission_test.go`、`internal/auth/middleware.go`
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-011-权限模型与中间件-成功.md`

### TF-012 异步审计（P1，依赖 TF-010）

- **涉及模块**：`internal/audit`
- **描述**：`audit_log` 表异步写入（Create/Update/Archive/Restore/StatusChange/Import/Export/权限与状态机修改 + **被拒请求 result=denied**）；读取不记录；审计查询与 `audit.log` 导出能力（导出端点放 TF-014）。
- **验收标准**：
  - [x] 单测：写操作触发审计、读取不触发、异步不阻塞业务、denied 记录
  - [x] 审计表只追加、无更新端点
- **产出文件**：`internal/audit/audit.go`、`internal/audit/audit_test.go`、`internal/task/service.go` 等（WriteHook 签名扩展）
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-012-异步审计-成功.md`

### TF-013 HTTP API 核心端点（P0，依赖 TF-003、TF-005~TF-008、TF-011、TF-012）

- **涉及模块**：`internal/api`
- **描述**：核心路由：`GET/POST /api/projects`、`DELETE /api/projects/:id`、`GET/POST /api/tasks`、`GET/PATCH /api/tasks/:id`、`POST /api/tasks/:id/archive|restore`、`DELETE /api/tasks/:id`、`GET/PUT /api/state-machine`、`GET /api/permissions`、`PUT /api/permissions`。统一响应 `{code, data}` / 错误 `{code, message, detail}`。
- **验收标准**：
  - [x] 单元 + 集成冒烟：项目→建任务→状态流转→归档→还原 全链路
  - [x] 错误码符合约定（PROJECT_NOT_FOUND / INVALID_TRANSITION / CIRCULAR_DEPENDENCY / STATUS_IN_USE / PERMISSION_DENIED）
  - [x] 传输层为薄封装，无重复业务逻辑
- **产出文件**：`internal/api/handlers_tasks.go`、`internal/api/handlers_projects.go`、`internal/api/handlers_state_machine.go`、`internal/api/handlers_permissions.go`、`internal/api/errors.go`、`internal/api/handlers_test.go`
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-013-HTTP核心端点-成功.md`

### TF-014 WebSocket 与其余端点（P1，依赖 TF-013、TF-012、TF-020、TF-019）

- **涉及模块**：`internal/api`
- **描述**：`/ws/events?project=` 事件推送（`task.* / import.* / export.* / project.* / permission.* / skill.* / state_machine.*`，事件结构 `{type, project, data, ts}`）；导入/导出/图/Skill/审计端点：`POST /api/import`、草稿三件套、`POST /api/export`、`POST /api/export/template/generate`、`GET /api/graph`、`GET /api/skills`、`GET /api/skills/:name`、`GET /api/audit`、`GET /api/audit/export`。WS 建连校验对应项目 `task.read`。
- **验收标准**：
  - [x] WS 客户端可收到写操作事件（task.created 等）
  - [x] 远程 WS 无 Token → 401；未授权项目 → 403
  - [x] graph 返回全量元数据（服务端不聚簇）
- **产出文件**：`internal/api/ws.go`、`internal/api/handlers_graph.go`、`internal/api/handlers_audit.go`、`internal/api/handlers_placeholder.go`、`internal/api/handlers_ws_test.go`（import/export/skill 端点随 P4 TF-018/019/020 替换占位，QA P3-3）
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-014-WebSocket与其余端点-成功.md`

> **✅ M3 里程碑达成（2026-08-06）**：P3 传输层与安全全部完成，真实 daemon 冒烟通过（curl 全链路 + WS 事件可达），质量门禁打勾见 MASTER-PLAN §7。

---

## P4 Agent 能力（M4：Agent 可用）— ✅ 已关闭（2026-08-06，用户人工验证通过，git 基线 v0.4.0）

### TF-015 LLM 客户端封装（P1，独立）

- **涉及模块**：`internal/llm`、`internal/config`
- **描述**：多协议 LLM HTTP 客户端（QA P4-1 扩展：`api_kind` = openai / anthropic / responses 三协议）；base_url、api_key（空回退环境变量 `DEEPSEEK_API_KEY`）、model、timeout、重试（网络/超时/5xx/429，4xx 不重试）、max_tokens、并发控制；`Complete` 文本 + `CompleteJSON` 结构化输出（openai 走 response_format，其余 prompt 约束 + 后处理提取首个平衡 JSON 块，提取失败整次失败）；仅 JSON 结构化通信；供 parser/exporter 复用。
- **验收标准**：
  - [x] 单测（mock HTTP server）：三协议请求构造与响应解析、超时重试（500/429 重试、400 不重试）、JSON 提取（围栏/数组/非法）、错误映射、并发度限制、环境变量回退
  - [x] 不依赖具体厂商 SDK，配置可指向 Ollama 等本地模型与 DeepSeek（QA P4-1：base https://api.deepseek.com，model deepseek-v4-flash）
- **产出文件**：`internal/llm/client.go`、`internal/llm/client_test.go`；`internal/config/config.go`/`global.go`（LLMConfig 增 `api_kind` 与默认值）；`docs/TASK-SEMANTICS.md` §14
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-015-LLM客户端封装-成功.md`

### TF-016 MCP 服务框架（P0，依赖 TF-003、TF-005、TF-010）

- **涉及模块**：`internal/mcp`、`cmd/cli`（mcp 子命令）、`internal/api`（/mcp 路由）、`internal/auth`
- **描述**：MCP 双传输（QA P4-1）：**stdio**（同二进制子命令 `tangoforge mcp --config`，直连业务层）+ **HTTP 远程**（daemon 挂载 `POST /mcp` Streamable HTTP，remote_access + Bearer 鉴权，MCP 恒为 agent 身份）。mark3labs/mcp-go v0.57.0（纯 Go，goproxy 验证可达）；统一执行骨架 exec（actor=clientInfo.name → project 必填 → PermissionStore.Require → 业务调用 → JSON 文本）；先落地 `task_read` / `task_create`；业务错误放 result（isError）。
- **验收标准**：
  - [x] stdio 集成测试（io.Pipe 驱动真实 Listen）：initialize → tools/list（≥2 工具）→ tools/call task_read（成功）/task_create（默认权限 denied）→ 缺 project 明确报错 → 未导入项目 PROJECT_NOT_FOUND
  - [x] HTTP 传输测试：回环调用成功、远程无 Bearer 401、denied 审计（actor=clientInfo.name）
  - [x] 真实 daemon 冒烟：`POST /mcp` initialize 200 + session + tools/list 返回工具声明
  - [x] 未携带 `project` → 明确报错（TASK_INVALID）
- **产出文件**：`internal/mcp/mcp.go`、`internal/mcp/tools_task.go`、`internal/mcp/mcp_stdio_test.go`、`cmd/cli/cmd_mcp.go`、`internal/api/handlers_mcp_test.go`；`internal/auth`（Require 方法 + ErrPermissionDenied + SecureEqual 导出）
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-016-MCP服务框架-成功.md`

### TF-017 MCP 工具全集（P1，依赖 TF-016 + 各业务模块）

- **涉及模块**：`internal/mcp`、`internal/task`（Graph）、`internal/project`（Init/ImportExisting/Create）、`internal/api`（graph 薄封装）
- **描述**：补齐 v1 固定工具集（19 个）：`project_list/import/init` + **`project_create`**（QA P4-1 Q6：import 仅导入 / init 仅初始化 / create 先 init 后 import）、`task_list/update/archive/restore`、`import_preview/confirm/discard`、`export_markdown`、`graph_get`（复用 task.Graph）、`state_machine_get/update`、`skill_info`、`permission_list`；project 域豁免权限（与 HTTP /api/projects 组一致）。
- **验收标准**：
  - [x] 每个工具首个参数为 `project`（强制；project_list 全局豁免）
  - [x] 工具权限与 HTTP 等价（查同一 permissions 表，denied 审计）
  - [x] 集成测试（stdio）：tools/list 全 19 工具；project_create→任务全生命周期→graph/状态机/import/export 全链路；denied 路径（task_update/state_machine_update 默认拒绝）
  - [x] task.Graph 单测（排除 archived、parent/dependency 边、空项目）；project Init/ImportExisting/Create 单测
- **产出文件**：`internal/mcp/tools_*.go`（project/import/export/graph/state_machine/skill/permission + task 扩展）、`internal/mcp/mcp_tools_test.go`、`internal/task/graph.go`、`internal/task/graph_test.go`、`internal/project/project_init_test.go`、`docs/TASK-SEMANTICS.md` §16.4
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-017-MCP工具全集-成功.md`

### TF-018 Markdown 导入与草稿流（P1，依赖 TF-015、TF-004、TF-005）

- **涉及模块**：`internal/parser`、`internal/task`（ImportTasks）、`internal/api`、`internal/llm`（ErrorCode）
- **描述**：LLM 解析 Markdown → 严格 JSON Schema 约束的嵌套结构（QA P4-1）；status 严格映射（key/label 匹配失败整次失败）；缺 title/status 或 JSON 不合规 → **整次失败不落库**（返回错误 + LLM 原始输出）；成功 → `import_drafts` 草稿（preview）→ 确认后按 `source_file` **文件级全量覆盖**入库（task.ImportTasks 事务：归档旧任务 + 批量重建）；草稿可丢弃。
- **验收标准**：
  - [x] 单测（mock LLM）：成功（嵌套/依赖标题解析）/缺字段/坏 JSON/超时/LLM 未配置全路径；失败事件 import.failed
  - [x] 确认入库后旧任务归档（Archived 计数）、新任务保留 source_file/source_section
  - [x] 草稿丢弃后正式任务池无变化；草稿生命周期 pending→confirmed/discarded
  - [x] task.ImportTasks 事务原子 + WAL 铁律（事务外校验）+ 不触发 task.* 钩子
- **产出文件**：`internal/parser/parser.go`、`internal/parser/schema.go`、`internal/parser/parser_test.go`、`internal/task/import.go`、`internal/task/import_test.go`、`internal/api/handlers_imports.go`、`internal/api/handlers_imports_test.go`、`docs/TASK-SEMANTICS.md` §17
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-018-Markdown导入与草稿流-成功.md`

### TF-019 Markdown 导出与模板（P2，依赖 TF-015、TF-005）

- **涉及模块**：`internal/exporter`、`internal/api`
- **描述**：默认模板渲染（`internal/exporter/templates/default.tmpl` 已升级：Front Matter + 层级标题 + 状态/优先级/标签/负责人元数据行，**可被 TF-018 重新导入**）；`export.template_path` 自定义模板覆盖；`template_mode: default|llm`；`target: overwrite|copy`（写盘 + 响应 content）；LLM 生成模板（示例文档 → 模板 → Parse 校验 → generated-template.tmpl + config 更新）。
- **验收标准**：
  - [x] 单测：默认模板渲染（层级/状态/元数据行断言）、自定义模板覆盖、LLM 模板生成与非法拒绝（mock LLM）、llm 模式未生成报错、overwrite 缺 path、项目未导入、flattenTree 层级
  - [x] 导出产物往返格式（`- 状态: <key>` + `##/###` 层级）可被 parser 重新导入
- **产出文件**：`internal/exporter/exporter.go`、`internal/exporter/exporter_test.go`、`internal/exporter/templates/default.tmpl`（升级）、`internal/api/handlers_export.go`、`internal/api/handlers_export_test.go`、`docs/TASK-SEMANTICS.md` §18
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-019-Markdown导出与模板-成功.md`

### TF-020 Skill 扫描与索引（P1，依赖 TF-005）

- **涉及模块**：`internal/skill`、`internal/api`、`internal/db`（迁移 v2）
- **描述**：扫描 `{workdir}/.taskboard/skills/` 一级目录（YAML + Markdown 双格式，QA P4-1）；解析失败仅告警不阻断；`skills` 表仅作缓存同步（迁移 v2 扩展 version/description/instructions 列）；`skill_info` 查询；替换 HTTP 占位 handler。
- **验收标准**：
  - [x] 单测：YAML/Markdown 解析、解析失败容错、缓存同步、删除文件后索引同步、子目录/非支持扩展名忽略、项目未导入报错、目录缺失视为空
  - [x] 文件系统为唯一数据源（表改动不反写文件）；扫描时机 = 启动 + 每次查询重扫（无 fsnotify 常驻 watcher，QA P4-1）
- **产出文件**：`internal/skill/skill.go`、`internal/skill/skill_test.go`、`internal/api/handlers_skills.go`、`internal/api/handlers_skills_test.go`、`internal/db/migrate.go`（v2）、`docs/TASK-SEMANTICS.md` §15
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-020-Skill扫描与索引-成功.md`

### TF-021 CLI 子命令（P1，依赖 TF-013 端点就绪）

- **涉及模块**：`cmd/cli`
- **描述**：CLI 全部子命令转为 HTTP 调用（等价操作）：`projects`（list/import/remove）、`tasks`（list/get/create/update/status/archive/restore/delete）、`import`（preview/drafts/confirm/discard）、`export`（run/template）、`graph`、`state-machine`（get/update）、`skills`、`permission`、`audit`（query/export）；`--project` 强制（任务类）、`--actor` 覆盖（默认 human）、`--server`（默认 127.0.0.1:19810）、`--json`；自动拉起（QA P4-1 Q15-A：spawn 同目录 daemon 二进制 + 轮询 /ping ≤5s，找不到提示手动启动）。
- **验收标准**：
  - [x] CLI 冒烟与 HTTP 等价（同一守护进程：projects import → UI 授权 → tasks 全生命周期 → export → graph → state-machine → permission；结果一致）
  - [x] 缺 `--project` 报错；未运行守护进程时提示自动拉起（daemon 同名二进制自动拉起 / 找不到时提示手动启动）
  - [x] 真实冒烟脚本 `scripts/cli_smoke.sh`（SMOKE PASS）
- **产出文件**：`cmd/cli/main.go`（改造）、`cmd/cli/client.go`、`cmd/cli/cmd_tasks.go`、`cmd/cli/cmd_import.go`、`cmd/cli/cmd_other.go`、`scripts/cli_smoke.sh`、`docs/TASK-SEMANTICS.md` §19
- **状态**：已完成（2026-08-06）
- **总结文件**：`docs/record/TF-021-CLI子命令-成功.md`

---

## P5 前端应用（M5：UI 可用）

### TF-022 前端基础：shadcn-ui + 路由 + 主题（P0，独立）✅ 已完成

- **涉及模块**：`app/`
- **描述**：shadcn-ui 初始化（`components.json` 已存在，`npx shadcn@latest init` + 按需 `add` 基础组件）；Tailwind CSS v4 + CSS 变量深浅双主题；React Router v7 路由骨架；`app/src/styles/globals.css` 主题变量落地。
- **验收标准**：
  - [x] `pnpm typecheck` / `pnpm lint` 全绿（含 `format:check`）
  - [x] 深浅主题切换生效；业务代码无硬编码色值（grep 抽查通过，色值仅存在于 token 源头 `theme.ts`/`globals.css`）
  - [x] 基础组件（Button/Input/Dialog/Toast 等）在 `ui/` 封装层可用
- **产出文件**：`app/src/components/ui/*`（17 个）、`app/src/lib/theme.ts`、`app/src/components/theme/theme-toggle.tsx`、`app/src/components/layout/app-layout.tsx`、`app/src/App.tsx`（改造）、`app/src/styles/globals.css`、`app/src/features/{projects,tasks,settings}/*`（占位页）、`app/eslint.config.mjs`（ui 豁免）、`.npmrc`、`pnpm-lock.yaml`
- **总结文件**：`docs/record/TF-022-前端基础-成功.md`

### TF-023 API 客户端封装与类型对齐（P0，依赖 TF-022、TF-013）✅ 已完成

- **涉及模块**：`app/src/api`、`app/src/types`、`app/src/hooks`
- **描述**：HTTP 客户端（携带 `X-UI-Token` + `X-Project`，统一 `{code,data}` 处理与错误码映射）；WebSocket 客户端（指数退避重连、事件→Query 失效映射表、多项目切换断开/重建）；`app/src/types/` 与后端 DTO 字段级同步；TanStack Query hooks（useTasks/useProjects/useGraph…）。
- **验收标准**：
  - [x] 单测（MSW 拦截 HTTP + mock WS）：hooks、错误映射、断线重连
  - [x] 类型与后端结构体字段一致（接口变更需同步）
- **产出文件**：`app/src/api/client.ts`、`app/src/api/ws.ts`、`app/src/types/models.ts`、`app/src/hooks/*`（10 个）、`app/src/stores/project.ts`、`app/src/test/server.ts`
- **总结文件**：`docs/record/TF-023-API客户端封装-成功.md`

### TF-024 项目管理界面与守护进程集成（P1，依赖 TF-023、TF-004）✅ 已完成

- **涉及模块**：`app/src/features/projects`、`app/electron`
- **描述**：项目列表/导入界面（选择目录→导入→初始化引导）；Electron 主进程：启动检测 `127.0.0.1:19810` 探活 → 拉起内嵌 daemon → 等待 `/ping` → 读取全局配置 `ui_token` 并注入渲染进程；退出不关守护进程。
- **验收标准**：
  - [x] 应用启动可自动拉起守护进程并进入项目列表（daemon.ts 探活/spawn/轮询 + bootstrap 注入 token；mac 实测在 P5 关闭前人工验证）
  - [x] 导入目录后 `.taskboard/` 自动初始化（后端 Import 语义确认：未注册目录自动 init；前端选目录→import→跳转）
  - [x] preload 白名单 IPC（daemon:ensureRunning / config:readUiToken / dialog:selectDirectory）
- **产出文件**：`app/electron/daemon.ts`（实装）、`app/electron/main.ts`、`app/electron/preload.ts`（+dialog）、`app/src/features/projects/WorkspacePage.tsx`（实装+4 测试）、`app/src/lib/bootstrap.ts`
- **总结文件**：`docs/record/TF-024-项目管理界面-成功.md`

### TF-025 看板视图（P0，依赖 TF-023、TF-006、TF-007）✅ 已完成

- **涉及模块**：`app/src/features/tasks`、`app/src/components/kanban`
- **描述**：按状态机动态生成列；卡片（标题、优先级色带红高灰低、标签徽章哈希着色、assignee）；拖拽触发状态流转 Mutation，`INVALID_TRANSITION` 回滚 + toast；虚拟滚动（≥ 1000 任务不卡）；按标签/状态过滤、搜索。
- **验收标准**：
  - [x] 组件测试（RTL）：渲染、拖拽调用（drag-logic 单测 + useKanban 乐观回滚 3 例）、非法流转回滚提示（INVALID_TRANSITION toast + 回滚）
  - [ ] 5,000 条 mock 数据滚动流畅（虚拟滚动生效，mac 实测项；实现：列内 @tanstack/react-virtual，estimateSize 108）
- **产出文件**：`app/src/components/kanban/*`（5 个）、`app/src/features/tasks/KanbanView.tsx`、`app/src/hooks/useKanban.ts`
- **总结文件**：`docs/record/TF-025-看板视图-成功.md`

### TF-026 任务详情与导航视图（P1，依赖 TF-023、TF-008）✅ 已完成

- **涉及模块**：`app/src/features/tasks`
- **描述**：任务详情/编辑（字段编辑、归档/还原、依赖编辑 + 无环提示）；导航三视图：树形列表（折叠/搜索）、时间线、状态分类。
- **验收标准**：
  - [x] 组件测试：详情编辑（TaskForm 5 例 + TaskDetail 3 例）、依赖环错误展示（CIRCULAR_DEPENDENCY → toast 无环提示）
  - [x] 树形视图与后端树结构一致（TreeNav 直接消费后端 tree，4 例）
- **产出文件**：`app/src/features/tasks/TaskDetail.tsx`、`app/src/features/tasks/TaskForm.tsx`、`app/src/features/tasks/NavViews.tsx`、`app/src/components/common/TreeNav.tsx`、`app/src/features/tasks/constants.ts`
- **总结文件**：`docs/record/TF-026-任务详情与导航-成功.md`

### TF-027 导入导出 UI（P1，依赖 TF-023、TF-018、TF-019）✅ 已完成

- **涉及模块**：`app/src/features/imports`
- **描述**：导入草稿流：提交 Markdown → 草稿预览（结构化展示）→ 确认/丢弃；导出：选择模板模式（default/llm）与目标（overwrite/copy）→ 渲染预览 → 执行；LLM 生成模板入口。
- **验收标准**：
  - [x] 组件测试：草稿预览、确认、丢弃全流程（DraftsPanel 4 例 + ExportDialog 3 例）
  - [ ] 与后端草稿三端点联调通过（mac 实测项；hooks 已对齐三端点）
- **产出文件**：`app/src/features/imports/*`、`app/src/features/tasks/ExportDialog.tsx`
- **总结文件**：`docs/record/TF-027-导入导出UI-成功.md`

### TF-028 全景地图 + 权限/Skill 界面（P2，依赖 TF-023、TF-014、TF-020）✅ 已完成

- **涉及模块**：`app/src/components/graph`、`app/src/features/permissions`、`app/src/features/skills`
- **描述**：全景图：`GET /api/graph` 全量数据，D3/vis-network 力导向渲染（useRef 管理实例、节点颜色映射状态、缩放拖拽、超阈值前端聚簇）；权限管理界面（仅 UI，勾选 action）；Skill 浏览（列表 + skill_info 详情）。
- **验收标准**：
  - [x] 组件测试：图渲染数据正确（circle=节点数、聚簇>300）、实例销毁无泄漏（卸载不抛错）
  - [x] 权限修改仅 UI 可操作（界面勾选 + PUT 全量覆盖；接口层已双重校验）
- **产出文件**：`app/src/components/graph/*`、`app/src/features/permissions/*`、`app/src/features/skills/*`、`app/src/features/tasks/GraphPage.tsx`、`app/src/features/settings/SettingsPage.tsx`（实装）
- **总结文件**：`docs/record/TF-028-全景地图与权限Skill-成功.md`

---

## P5.5 项目设置页（M5.5：config.yaml 可视化编辑）

### TF-032 项目设置页（P1，依赖 TF-006、TF-013、TF-023）✅ 已完成

- **涉及模块**：`internal/config`、`internal/api`、`internal/task`、`app/src/features/settings`、`app/src/components/project`
- **描述**：项目设置页 `/project/:projectId/settings`（tab「设置」入口）可视化编辑项目 `config.yaml`：
  - 后端：`config.UpdateProjectFile` 部分更新（**保留未知节**，与 `SaveProject` 全量替换区分）；`GET/PUT /api/project-config`（GET 权限 `state_machine.read`，**PUT 仅 UI**）；校验复用 `task.ValidateStateMachineUpdate`（编辑校验 + `STATUS_IN_USE`，从 `UpdateStateMachine` 抽取，持久化统一迁移部分更新）；审计 `project_config.updated` + WS `project_config.changed`。
  - 前端：状态机编辑器（states 增删改 + 颜色取色器 + transitions from/to 编辑 + 恢复默认）+ 导出模板路径（可编辑，留空默认）+ 高级区 YAML 原文（与表单同 draft 双视图，手动编辑保存以 YAML 为准，兼容磁盘 snake_case / DTO PascalCase 键）；显式保存（sticky 底部操作栏：保存/放弃），校验失败 toast 并保留修改。
- **QA 决策**：结构化表单 + YAML 兜底；显式保存；导出模板展示 + 可编辑；后端新增端点写仅 UI（见 `docs/TASK-SEMANTICS.md` §20）。
- **验收标准**：
  - [x] Go：config/task/api 单测全绿（未知节保留、非 UI PUT 403、非法流转 400、占用 422 STATUS_IN_USE、审计/WS 事件）
  - [x] 前端：ProjectSettingsPage 8 例（回显/表单提交/导出提交/YAML 提交/YAML 非法/占用失败保留/删除状态/放弃修改）；全量 140 用例 + typecheck + lint 全绿
- **产出文件**：`internal/config/project.go`（UpdateProjectFile）、`internal/api/handlers_project_config.go`、`internal/api/handlers_project_config_test.go`、`internal/task/state_machine.go`（抽取校验 + 部分更新）、`app/src/hooks/useProjectConfig.ts`、`app/src/features/settings/ProjectSettingsPage.tsx`(+test)、`app/src/components/project/project-panel.tsx`（tab）、`app/src/App.tsx`（路由）、`app/src/stores/project.ts`（sections）
- **总结文件**：`docs/record/TF-032-项目设置页-成功.md`

---

## P6 测试与交付（M6：可交付）

### TF-029 集成测试套件（P1，依赖 P1~P4 关键任务）

- **涉及模块**：`test/integration`
- **描述**：启动临时 Daemon（真实 HTTP 客户端）覆盖核心链路：项目导入→任务 CRUD→状态流转→归档/还原→依赖环拒绝→导入草稿流→导出→审计记录→权限 denied；文件头 `//go:build integration`。
- **验收标准**：
  - [ ] `go test -tags=integration ./test/integration/...` 全绿
  - [ ] 覆盖 denied/error 路径（403/401/错误码）
- **产出文件**：`test/integration/*_test.go`（替换 README 占位）

### TF-030 CI 工作流实跑（P1，依赖多数任务）

- **涉及模块**：`.github/workflows`
- **描述**：backend-ci（Go 1.22/1.23 矩阵：gofmt→vet→lint→`CGO_ENABLED=0` test→覆盖率→集成测试→build-all）+ frontend-ci（install→typecheck→lint→format→test→build）；从 P1 起渐进启用路径触发。
- **验收标准**：
  - [ ] GitHub Actions 双 CI 全绿（或本地等价 `make check` 全绿）
  - [ ] 新增 Go 依赖在 PR 说明中注明纯 Go 依据
- **产出文件**：`.github/workflows/backend-ci.yml`、`.github/workflows/frontend-ci.yml`（实跑修正）

### TF-031 交叉编译与打包（P2，依赖 TF-030 通过）

- **涉及模块**：`Makefile`、`app/`（electron-builder）
- **描述**：`make build-all` 产出 Windows/macOS/Linux × x64/arm64 共 6 个静态二进制并校验 `CGO_ENABLED=0`；electron-builder 打包 APP（内嵌对应平台 daemon），产物安装冒烟。
- **验收标准**：
  - [ ] `make build-all` 6 产物生成成功且 file/ldd 校验无动态依赖
  - [ ] APP 打包产物可安装启动并完成冒烟
- **产出文件**：构建产物目录（dist/release）、打包配置修正

---

## 附录 A：任务状态更新操作（每完成一个任务）

1. 本文件：状态置 `已完成`，更新「状态速览」表；
2. `docs/task/OVERVIEW.md`：同步统计与看板；
3. 新建 `docs/record/TF-XXX-<标题>-<结果>.md`（模板见 `docs/task/MASTER-PLAN.md` §6）；
4. 新建/追加 `docs/log/TF-XXX-<标题>.md`；
5. 提交信息带 `TF-XXX`（如 `feat(task): TF-005 任务基础 CRUD`）。

*（文档完）*
