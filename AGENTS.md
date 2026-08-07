# TangoForge — AGENTS.md

> **适用对象**：AI 编程助手（Vibe Coding / Cursor / Copilot / Claude Dev）与开发者
> **版本**：v1.4（新增「§13 任务文档规范」：任务总结 `docs/record`、任务日志 `docs/log`、临时任务豁免；开发计划见 `docs/task/`；配套 `docs/REQUIREMENTS.md` v1.1、`docs/TECHNICAL.md` v1.0、`docs/TASK-SEMANTICS.md` v1.0（任务域定义语义，TF-005 起）；四者冲突时以 `docs/REQUIREMENTS.md` 为准）
> **核心理念**：本地优先 · 等价操作 · 锻造而非展示
> **文档位置**：本文件为**权威版本**（仓库根目录，AI 工具默认读取）；`docs/AGENTS.md` 为同步副本；技术落地细节见 `docs/TECHNICAL.md`；**定义语义（字段取值、参数指针语义、过滤/排序/分页规则、错误码、职责边界）统一沉淀于 `docs/TASK-SEMANTICS.md`，实现与语义冲突时以该文件为准并向其登记**

---

## 1. 项目世界观（先读我）

TangoForge 不是传统的看板 UI 工具，而是一个**人机协作的操作系统级任务中间件**。

- **数据是主人**：所有数据自包含在用户选定的本地 `.taskboard/` 目录中，系统没有任何云端依赖。
- **界面是仆人**：Electron GUI 只是访问数据的渠道之一，CLI、MCP 和 API 拥有**完全等价**的操作权。
- **AI 是头等公民**：Agent 不是"插件"，而是通过 MCP 协议以原生方式操作任务池的一等成员，其权限范围由用户单独配置。
- **单守护进程 · 多项目**：一个守护进程常驻管理多个项目；项目以**工作目录路径**为唯一标识；所有跨端调用必须**显式携带项目目录**（HTTP `X-Project` 头 / CLI `--project` 参数 / MCP 工具参数 `project`），禁止隐式依赖"当前项目"。

**所有代码必须尊重"多端等价"原则——绝不能编写仅在 GUI 中可用的逻辑，而绕过 API 或 MCP。**

---

## 2. 编码核心约束（铁律）

### 2.1 零信任依赖
- **禁止**引入任何需要 CGO 或外部动态链接库的 Go 依赖。
- 数据库必须使用 `modernc.org/sqlite`（纯 Go 实现）。
- 所有二进制产物必须可通过 `CGO_ENABLED=0` 编译通过（CI 中强制校验）。
- 打包目标：Windows / macOS / Linux × x64 / arm64 共 6 个产物（`make build-all`）。

### 2.2 接口先行
- 所有功能模块（创建、更新、删除、归档/还原、导入导出、权限校验、状态机）必须先定义清晰的 Interface。
- HTTP Handler、CLI Command、MCP Tool 必须**共享同一套业务层实现**，不得重复造轮子。

### 2.3 显式优于隐式
- 禁止在代码中硬编码路径（如 `~/.config`）。所有路径必须基于用户传入的 `--workdir` 或环境变量动态拼接。
- **项目标识必须显式**：任何访问任务池的调用（HTTP / CLI / MCP / WS）都必须携带项目目录，未携带或目录未注册为项目时返回 `PROJECT_NOT_FOUND`。
- 配置分为两层，严禁混淆：
  - **全局配置**（`~/.taskboard-app/config.yaml`）：存放 LLM 密钥、监听端口、远程访问开关、API Token、UI 会话凭据。
  - **项目配置**（`{workdir}/.taskboard/config.yaml`）：仅存放导出模板、状态机、自定义字段等业务配置。

### 2.4 审计日志不可篡改
- 所有写操作（Create / Update / Archive / Restore / StatusChange / Import / Export / 权限与状态机修改）必须异步写入 `audit_log` 表（数据本体），`audit.log` 文件仅为按需导出物。
- 审计字段：`id, ts, actor, actor_class, action, target, result, detail`；**仅写操作记录**，读取不记录。
- **来源识别（Actor 判定）**，按以下优先级：
  1. 请求携带 `X-UI-Token` 且来源为回环 → `actor_class=ui`（App UI，全权限）。
  2. MCP stdio 会话 → `actor_class=agent`，`actor` 为客户端名称。
  3. 远程请求携带 `Authorization: Bearer <api_token>` → `actor_class=agent`，`actor` 取 `X-Actor` 或回退 `unknown`。
  4. 本地 HTTP / CLI（含 `X-Actor` 头，CLI 默认 `human`）→ `actor_class=agent`（最小信任，视同 Agent 查权限表）。
  5. 无法识别的请求 → `actor_class=unknown`，按 Agent 权限表检查并记录审计。

---

## 3. 架构与模块边界

### 3.1 模块划分

```
internal/
├── config/     # 全局配置与项目配置加载、合并、热重载（仅加载合并，不做业务判断）
├── db/         # SQLite 初始化、迁移（migrate up/down）、连接池管理
├── task/       # ⭐ 任务核心：模型、CRUD、状态机校验、归档/还原、依赖校验
├── parser/     # LLM 交互：Markdown → 结构化 JSON（草稿生成）
├── exporter/   # 从数据库重建 Markdown（模板渲染、LLM 生成模板）
├── auth/       # 来源识别（ui/agent/unknown）、Token 校验、权限中间件
├── api/        # HTTP / WebSocket 路由与处理器
├── mcp/        # MCP 工具注册与执行（v1 固定工具集，不动态注册）
├── skill/      # Skill 技能包（内置 embed + 全局库）+ 宿主安装/卸载/状态
├── guide/      # AI 使用说明书（免鉴权，HTTP/MCP/CLI 三端复用）
├── llm/        # LLM HTTP 客户端封装（供 parser/exporter 复用，仅 JSON 结构化通信）
└── audit/      # 审计日志异步写入与导出
```

### 3.2 开发规范（铁律）
- `task / parser / exporter / skill` 等业务层**禁止**引用 `api`、`mcp`、`cmd` 包。
- 数据库事务边界必须在业务层控制，`db` 层仅提供原生 SQL 或 Query Builder。
- 传输层（api / mcp / cmd/cli）为薄封装，只做参数解析与响应格式化，禁止重复业务逻辑。

---

## 4. 数据模型细节（开发必看）

### 4.1 Task 结构体（Go）

```go
type Task struct {
    ID            string    `json:"id" db:"id"`                     // UUID v4
    ProjectID     int64     `json:"project_id" db:"project_id"`     // 所属项目
    ParentID      *string   `json:"parent_id" db:"parent_id"`
    Title         string    `json:"title" db:"title"`
    Description   string    `json:"description" db:"description"`
    Status        string    `json:"status" db:"status"`             // 项目状态机 key（默认 todo/doing/done/archived）
    Priority      int       `json:"priority" db:"priority"`         // 0-5，0=最低
    Tags          []string  `json:"tags" db:"tags"`                 // JSON 数组
    Assignee      string    `json:"assignee" db:"assignee"`
    DependsOn     []string  `json:"depends_on" db:"depends_on"`     // JSON 数组，存储 Task ID
    ArchivedFrom  string    `json:"archived_from" db:"archived_from"` // 归档前状态（还原用）
    SourceFile    string    `json:"source_file" db:"source_file"`   // 原始 Markdown 路径
    SourceSection string    `json:"source_section" db:"source_section"` // LLM 解析映射段落
    CreatedAt     time.Time `json:"created_at" db:"created_at"`
    UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}
```

> 模型已初始化于 `internal/task/task.go`，后续改动必须保持与 `docs/TECHNICAL.md` §3.2 及前端 `app/src/types/` 字段级同步。

### 4.2 核心表（DDL 见 REQUIREMENTS.md §四.4）

- **projects**：`id, name, workdir(UNIQUE), created_at, last_opened_at`——仅"项目名称 + 工作目录"记录；移除记录**绝不删除磁盘 `.taskboard/` 数据**。
- **tasks**：见上；`project_id` 外键 + `(project_id, status)` 索引。
- **permissions**：`(project_id, action, allowed)`——**仅存 Agent 权限范围**；action 为命名空间字符串（`task.read` / `task.create` / `import.run` …）；UI 不查表。
- **import_drafts**：`id, project_id, source_file, parsed_json, status(pending/confirmed/discarded)`——LLM 解析草稿，确认后入库。
- **skills 表已移除**（迁移 v3 drop，TF-033）：技能包改为内置 embed + 全局技能库 `~/.taskboard-app/skills/`，安装状态实时扫描宿主位置，无项目库依赖。
- **audit_log**：`id, ts, actor, actor_class, action, target, result, detail`。

### 4.3 约束（铁律）

- **状态机**：由项目 `.taskboard/config.yaml` 的 `state_machine` 节驱动（每项目独立）；状态流转必须满足 `transitions`，非法流转返回 `INVALID_TRANSITION`；**有任务占用的状态不可移除**（删除/重命名校验）。
- **删除 = 归档**：删除操作统一为归档（`status=archived` 并记录 `archived_from`），回收站可还原到归档前状态。
- **级联规则**：归档/物理删除父任务时，所有子任务**级联置空 `parent_id`**（成为顶层任务）；**物理删除子任务严格禁止**，物理删除仅限回收站（archived）中的任务。
- **depends_on 无环**：Create/Update 若引入循环依赖，**写操作直接拒绝**（`CIRCULAR_DEPENDENCY`），不允许写入带环数据。
- 导入走**草稿流程**：解析结果先入 `import_drafts`，用户确认后按 `source_file` **文件级全量覆盖**入库；解析失败（缺 title/status / JSON 不合规）整次失败不落库。

---

## 5. AI Agent（LLM）交互规范

### 5.1 Markdown 解析入库
- 调用 LLM 的 Prompt 必须包含严格的 JSON Schema 约束。
- 如果 LLM 返回的 JSON 缺少 `title` 或 `status` 字段，**必须拒绝入库**并返回明确错误，禁止补默认值（由上游用户处理）。
- 解析成功 → 生成**草稿**（`import_drafts`）→ 调用方显式确认后入库；提供 `import_preview / import_confirm / import_discard` 等价端点。

### 5.2 Markdown 导出模板
- 默认模板位于 `internal/exporter/templates/default.tmpl`。
- 用户自定义模板通过项目配置 `config.yaml` 中的 `export.template_path` 覆盖。
- 支持**LLM 生成模板**：用户提供示例文档，由 LLM 生成贴近原始文档风格的模板（`POST /api/export/template/generate`）。
- 模板引擎使用 Go `text/template`，禁止引入其他模板语言（如 Jinja 或 Handlebars）。

---

## 6. 网络与安全红线

### 6.1 绑定地址与来源过滤（热切换）

```go
// 伪代码逻辑，严禁违反
// 守护进程始终监听 0.0.0.0:19810（端口可由全局配置热重载调整）
// 远程访问开关为内存标志，切换即时生效，无需重启、无需重绑地址
if !globalConfig.RemoteAccess && !isLoopback(remoteAddr) {
    reject(403) // "remote access disabled"
}
```

### 6.2 权限中间件

- 所有 `/api/*` 路由必须经过 `auth` 中间件，按 §2.4 完成**来源识别**。
- `actor_class == "ui"` → 直接放行（不查权限表）；其余一律查询 `permissions` 表中该项目的 `action` 开关，未授权返回 403。
- **AI Agent（MCP）在任何情况下都不可修改权限表**。权限修改 API（`PUT /api/permissions`）必须额外校验：携带 `X-UI-Token` 且来源 IP 为回环（即使 `RemoteAccess` 开启）。
- 远程连接必须携带 `Authorization: Bearer <api_token>`，否则 401。

---

## 7. 命名与代码风格

- **Go**：遵循标准 `gofmt`，变量名尽量简短（`t` 代表 Task，`db` 代表 Database）。
- **错误处理**：使用 `fmt.Errorf("...: %w", err)` 进行包裹，便于溯源。
- **日志**：使用 `slog`（Go 标准库结构化日志），级别分为 `Debug`（开发）、`Info`（生产默认）、`Error`（异常）。
- **API 响应格式**：
  - 成功：`{"code":0, "data": {...}}`
  - 失败：`{"code":非零, "message": "可读错误", "detail": "技术细节(可选)"}`
  - 业务错误码约定：`PROJECT_NOT_FOUND` / `INVALID_TRANSITION` / `CIRCULAR_DEPENDENCY` / `STATUS_IN_USE` / `PERMISSION_DENIED` / `IMPORT_FAILED` 等。
- **前端**：TypeScript 严格模式；组件/目录规范见 `docs/TECHNICAL.md` §4（shadcn-ui 铁律：业务组件禁止直接用 Radix 原语、颜色一律 CSS 变量、D3/vis-network 命令式渲染经 `useRef` 管理）。

---

## 8. 测试要求（AI 生成代码时必须遵守）

- **单元测试**：所有业务层逻辑必须附带 `_test.go`，使用 `sqlite:memory:` 模式进行隔离测试，不允许依赖本地文件系统。
- **集成测试**：放在 `/test/integration` 下（文件头声明 `//go:build integration`），启动临时 Daemon 并调用真实 HTTP 客户端；运行：`go test -tags=integration ./test/integration/...`。
- **测试覆盖率**：核心 `internal/task` 覆盖率不低于 **90%**（含状态机校验、归档/还原、依赖校验、草稿导入流程）；由 `scripts/check_coverage.sh` 强制门槛。
- **前端测试**：Vitest + React Testing Library；服务端数据用 MSW 拦截 HTTP，WebSocket 用 mock 客户端模拟事件；覆盖 Query/Mutation hooks、shadcn-ui 封装组件、关键业务组件（看板拖拽、草稿确认流）。运行：`pnpm test`（覆盖率门槛见 `app/vitest.config.ts`）。
- **新代码必须附带测试才能提交**（见 §10 完成标准）。

---

## 9. 工程结构总览（初始化完成）

> 本仓库已按 `docs/TECHNICAL.md` 完成目录初始化；新增代码必须落在对应目录，禁止在根目录堆叠散文件。

```
TangoForge/
├── AGENTS.md                  # 本文件（AI 助手权威约束，根入口）
├── Makefile                   # 统一构建/测试/质量入口（make fmt/lint/test/check/...）
├── go.mod                     # Go module（module 路径为占位，push 前替换为真实仓库地址）
├── package.json               # pnpm workspace 根（husky + lint-staged + 前端脚本编排）
├── pnpm-workspace.yaml        # workspace: ["app"]
├── .golangci.yml              # Go lint 配置（govet/staticcheck/errcheck/gofumpt/revive...）
├── .editorconfig / .gitignore / .lintstagedrc.json
├── .husky/pre-commit          # 提交前钩子（gofmt + 前端 ESLint/Prettier）
├── cmd/
│   ├── daemon/main.go         # 守护进程入口（最小骨架：--workdir、slog、:19810、/ping）
│   └── cli/main.go            # CLI 入口（最小骨架：version/help）
├── internal/
│   ├── config/ db/ task/ parser/ exporter/ auth/ api/ mcp/ skill/ llm/ audit/
│   └── exporter/templates/default.tmpl
├── test/integration/          # 集成测试（临时 Daemon + 真实 HTTP 客户端）
├── app/                       # Electron + React 19 + shadcn-ui 前端工程
│   ├── electron/              # main.ts / preload.ts / daemon.ts（安全基线：contextIsolation + sandbox）
│   ├── src/
│   │   ├── components/{ui,kanban,graph,common}/
│   │   ├── features/{tasks,projects,imports,permissions,skills}/
│   │   ├── stores/ api/ hooks/ lib/ types/ styles/ test/
│   ├── components.json        # shadcn-ui 配置（new-york / cssVariables）
│   ├── vite.config.ts         # electron-vite 三端构建（main/preload/renderer）
│   ├── vitest.config.ts       # Vitest + jsdom + MSW 就绪
│   └── eslint.config.mjs / .prettierrc.json / .lintstagedrc.json
├── scripts/check_coverage.sh  # internal/task 覆盖率门槛（默认 90%）
├── .github/workflows/         # backend-ci.yml + frontend-ci.yml（push/PR 自动触发）
└── docs/                      # 文档目录（使用说明见 docs/README.md）
    ├── README.md              # docs 目录索引与各文件/目录使用方式（新增文档须同步登记）
    ├── REQUIREMENTS.md        # 需求基线（v1.1，最高优先级）
    ├── REQUIREMENTS-REVIEW.md # 需求评审 35 项 QA 记录
    ├── TECHNICAL.md           # 技术落地说明（v1.0）
    ├── AGENTS.md              # 本文件同步副本
    ├── task/                  # 开发任务计划：MASTER-PLAN.md / TASKS.md / OVERVIEW.md
    ├── record/                # 任务总结：TF-XXX-<标题>-<结果>.md（必写，见 §13.2）
    └── log/                   # 任务日志：TF-XXX-<标题>.md（按需写，临时任务豁免，见 §13.3/13.4）
```

---

## 10. 开发与测试流程（vibe coding 工作流）

> 本节是本次初始化的重点：**所有 AI 生成的代码与人工代码走同一套流程**。

### 10.1 命令速查

| 目的 | 命令 | 说明 |
|------|------|------|
| 全部检查（提交前必跑） | `make check` | fmt + vet + lint + test + 覆盖率 + 集成测试 + build + typecheck |
| Go 单元测试 | `go test ./...` | `sqlite:memory:` 隔离 |
| Go 覆盖率门槛 | `./scripts/check_coverage.sh` | internal/task ≥ 90%（`THRESHOLD` 可覆盖） |
| 集成测试 | `go test -tags=integration ./test/integration/...` | 临时 Daemon + 真实 HTTP |
| Go lint | `golangci-lint run` | 配置见 `.golangci.yml` |
| 交叉编译 6 产物 | `make build-all` | Windows/macOS/Linux × x64/arm64，`CGO_ENABLED=0` |
| 前端类型检查 | `pnpm typecheck` | tsc 严格模式（node + web 两套配置） |
| 前端测试 | `pnpm test` / `pnpm test:coverage` | Vitest + RTL + MSW |
| 前端 lint / 格式化 | `pnpm lint` / `pnpm format` | ESLint flat config + Prettier |
| 前端开发 / 构建 | `pnpm dev` / `pnpm build` | electron-vite 三端 |
| 安装依赖 | `pnpm install`（根目录） | workspace 模式，husky 自动注册 |

> Windows 本地未装 make 时，直接执行等价的 `go ...` / `pnpm ...` 命令即可。

### 10.2 提交规范（Conventional Commits）

- 格式：`<type>(<scope>): <subject>`，如 `feat(task): 实现状态机流转校验`、`fix(api): 修复 X-Project 解析`。
- type 允许：`feat / fix / docs / refactor / test / chore / perf / build / ci`。
- **禁止**直接推送 `main`；改动走分支 → PR → Review → 合并（见 §10.6）。
- 提交前 pre-commit 钩子自动执行：Go 文件 `gofmt`，前端文件 ESLint `--fix` + Prettier；若失败则提交被拦截。

### 10.3 CI 流程（GitHub Actions，push/PR 自动触发）

- **backend-ci.yml**（路径触发 `cmd|internal|test|go.mod|Makefile|.golangci.yml`）：
  1. Go 1.22 / 1.23 双版本矩阵；
  2. `gofmt` 校验（未格式化即失败）→ `go vet` → golangci-lint；
  3. `CGO_ENABLED=0 go test -cover ./...`（零信任依赖铁律）；
  4. `scripts/check_coverage.sh`（internal/task ≥ 90%）；
  5. 集成测试；
  6. `make build-all` 交叉编译 6 产物并校验。
- **frontend-ci.yml**（路径触发 `app|package.json|pnpm-*`）：`pnpm install --frozen-lockfile` → typecheck → lint → prettier 格式检查 → Vitest 覆盖率 → electron-vite build。
- 本地等价物：`make check`（CI 不通过的代码视为未完成）。

### 10.4 质量门槛清单（通过 = 可提交）

- [ ] `gofmt` 无 diff（CI 强制）
- [ ] `go vet ./...` 无错误
- [ ] golangci-lint 无 error 级问题（govet/staticcheck/errcheck/ineffassign/unused/misspell/gofumpt/revive）
- [ ] `CGO_ENABLED=0 go test ./...` 全绿；`internal/task` 覆盖率 ≥ 90%
- [ ] 涉及业务层改动时附带对应 `_test.go`
- [ ] `pnpm typecheck`、`pnpm lint`、`pnpm format:check`、`pnpm test` 全绿
- [ ] 未引入 CGO / 外部动态链接依赖；新增 Go 依赖须在 PR 说明中注明纯 Go 依据
- [ ] 前端 DTO（`app/src/types/`）与后端结构体字段级同步（接口变更时）

### 10.5 分支与 PR 流程

1. 从 `main` 切功能分支：`git checkout -b feat/<feature>` 或 `fix/<bug>`。
2. 开发中随时小步提交（遵循 §10.2 规范）。
3. 推送分支 → 创建 PR（标题遵循提交规范；描述注明：改动内容、测试结果、覆盖率、依赖变更）。
4. CI 全绿 + 至少 1 人 Review 通过后合并；合并使用 squash。
5. 合入 `main` 后 CI 再次全量验证。

### 10.6 AI 生成代码的完成标准

AI 助手完成任何功能后，**在宣告"完成"之前**必须：

1. 代码落在正确的模块目录（§9），遵守分层铁律（§3.2）；
2. 业务层逻辑附带 `_test.go`（§8），必要时补集成测试；
3. 本地执行 `make check`（或逐项执行等价命令）并贴出结果；
4. 涉及前端时同步更新 `app/src/types/` DTO 与测试；
5. 更新受影响文档（本文件 / `docs/TECHNICAL.md` 如有接口或约束变化）。

**未通过 §10.4 门槛的代码不得提交、不得宣告完成。**

---

## 11. 当前开发阶段重点任务（AI 指引）

> **完整可执行的开发计划**见 `docs/task/TASKS.md`（逐任务清单）、`docs/task/MASTER-PLAN.md`（执行规则与完成定义）、`docs/task/OVERVIEW.md`（进度看板）。**被召唤写代码时，优先按计划取任务执行**（任务编号 `TF-XXX`，遵守 §13 记录规范）。以下为计划首批任务摘要（与 `TF-001` 等对应）：

1. **完成 `internal/db` 迁移脚本**（= TF-001），确保 `meta.db` 自动创建全部表：`projects / tasks / permissions / import_drafts / skills / audit_log`（DDL 以 REQUIREMENTS.md §四.4 为准）。
2. **实现 `internal/task` 的基础 CRUD**（= TF-005，含状态机校验 TF-006、归档/还原 TF-007、depends_on 无环校验 TF-008），先不要依赖 LLM 功能，用 Mock 数据测试通过（覆盖率 ≥ 90%）。
3. **搭建 `cmd/daemon` 的最小骨架**（= TF-003）：单实例常驻、监听 `19810` 端口、返回 Health Check（`GET /ping`）、实现来源过滤中间件与 `X-Project` 项目解析。（`/ping` 已就绪，其余待实现）
4. **编写 MCP 的 `list_tools` 和 `call_tool` 基本框架**（= TF-016），暂时仅实现 `task_read` 和 `task_create`（均含 `project` 必填参数）。
5. **搭建 `internal/auth` 来源识别中间件**（= TF-010）：区分 `ui`（X-UI-Token + 回环）与 `agent`（MCP / Token / X-Actor），并接入 `audit` 异步写入。
6. **前端**（= TF-022/023）：初始化 shadcn-ui 基础组件（`npx shadcn@latest init` + 按需 `add`），搭建路由骨架与 `api/` HTTP+WS 客户端封装。

---

## 12. 遇到不确定的事情怎么办？

- **先读需求文档**：`docs/REQUIREMENTS.md`（v1.1）优先级最高；冲突以它为准；技术落地以 `docs/TECHNICAL.md` 为准。
- **宁可报错，不可臆断**：当无法决定 LLM 输出结构或权限边界时，返回 `501 Not Implemented` 错误，并提示"需要人工设计"，不要自作聪明补全逻辑。
- **询问用户**：如果涉及破坏性变更（如修改数据库 Schema、修改状态机语义、改变 API 响应格式），必须暂停并请求确认。
- **遵守流程**：完成标准见 §10.6——测试、覆盖率、CI 门槛是硬性要求，不是可选项。

---

## 13. 任务文档规范（docs/task · docs/record · docs/log）

> 适用于所有开发任务（含 AI 生成的代码），与 §10 完成标准配套执行。任务编号以 `docs/task/TASKS.md` 为准。

### 13.1 文档体系

| 目录 | 内容 | 命名 | 必写 |
|------|------|------|------|
| `docs/task/` | 开发计划三件套：主行动计划 / 可执行任务清单 / 任务全景预览 | 固定文件名（MASTER-PLAN.md / TASKS.md / OVERVIEW.md） | 维护更新 |
| `docs/record/` | **任务总结**：任务完成后的结果归档 | `TF-XXX-<任务标题>-<结果>.md`，结果 ∈ `成功 / 失败 / 部分完成` | ✅ 必写 |
| `docs/log/` | **任务日志**：工作过程中的过程记录（进展、决策、踩坑） | `TF-XXX-<任务标题>.md`（同一任务可多次追加） | ⚠️ 按需写（豁免见 13.4） |
| `docs/README.md` | docs 目录使用说明 + 文件索引 | 固定文件名 | 新增文档须同步登记 |

- **任务编号**：`TF-XXX`（TangoForge 任务，三位数字），在 `docs/task/TASKS.md` 中分配，**全项目唯一、一经分配不重用**。
- **提交信息**必须引用任务编号，如 `feat(task): TF-005 任务基础 CRUD`。

### 13.2 任务总结（docs/record，必写）

每个正式任务完成后，必须创建 `docs/record/TF-XXX-<任务标题>-<结果>.md`：

- `<结果>` 取 `成功` / `失败` / `部分完成`；`失败` / `部分完成` 必须写明遗留问题与后续处理。
- 总结模板（按序）：**任务范围**（一句话）→ **交付内容**（文件清单 + 关键实现点 ≤5 条）→ **验证结果**（命令与输出摘要：测试/覆盖率/lint/冒烟）→ **遗留问题与后续**（无则写"无"）。

### 13.3 任务日志（docs/log，按需写）

正式任务进行中，将工作过程记录到 `docs/log/TF-XXX-<任务标题>.md`：进展、技术决策、踩坑与解决、验证过程。同一任务跨多日时按日期分节追加，**禁止覆盖已有内容**。

### 13.4 临时任务豁免（不需要日志记录）

以下**与修改源代码无关**的任务**不需要**任务日志，也不分配任务编号、不写任务总结：

- 修改配置文件（如 `.golangci.yml`、`app/package.json` 依赖版本、CI 工作流、Makefile）；
- 临时对代码进行验证（跑一次测试、起一次服务、查一次数据）；
- 纯只读操作（阅读文档、检索代码、查看数据）。

> 注：豁免仅免除 docs/log 日志；若操作最终改变了源代码行为，则回到正式任务流程，补任务编号、总结与日志。

---

**最后重申**：TangoForge 的目标是让 AI 像使用 Linux 命令行一样操作任务。保持简单、保持解耦、保持本地纯真。
