# TangoForge — 技术说明文档（TECHNICAL.md）

> **版本**：v1.0
> **定位**：本文档由 `AGENTS.md` 中的技术性章节抽离而成，并补充前端 **React 19 + shadcn-ui** 技术细节。
> **配套关系**：与 `REQUIREMENTS.md` v1.1、`AGENTS.md` v1.1 配套；三者冲突时以 `REQUIREMENTS.md` 为准。
> **阅读对象**：开发者与 AI 编程助手。`AGENTS.md` 保留理念与工作流约束，本文档只讲技术落地。

---

## 1. 技术栈总览

### 1.1 后端（守护进程 & CLI）

| 组件 | 技术选型 | 说明 |
|------|----------|------|
| 语言 | Go 1.22+ | 单一静态二进制，无 CGO |
| 数据库 | `modernc.org/sqlite` | 纯 Go 实现，禁止 CGO/外部动态链接依赖 |
| HTTP 框架 | Go `net/http` + chi | 轻量路由 |
| WebSocket | `nhooyr.io/websocket` | 实时变更推送 |
| MCP | `mark3labs/mcp-go` 或自研 | stdio 传输、固定工具集（不动态注册） |
| 配置热重载 | `fsnotify` + 原子替换 | 监听全局配置文件，切换即时生效 |
| 日志 | `slog`（Go 标准库） | Debug / Info / Error 三级 |
| 模板引擎 | Go `text/template` | 禁止引入其他模板语言 |

### 1.2 前端（Electron + React）

| 组件 | 技术选型 | 说明 |
|------|----------|------|
| 桌面框架 | Electron | 主进程 + 渲染进程，内嵌守护进程二进制 |
| 构建工具 | Vite（electron-vite 统一管理） | main / preload / renderer 三端构建 |
| UI 框架 | React 19 | 并发特性、`use()`、Actions、Compiler |
| 组件体系 | **shadcn-ui / ui** | 基于 Radix UI + Tailwind CSS，组件源码随项目维护 |
| 样式 | Tailwind CSS v4 + CSS 变量 | 深浅主题双模式 |
| 类型 | TypeScript 5.x（严格模式） | 与后端 DTO 强对齐 |
| 客户端状态 | Zustand | 仅 UI 局部状态 |
| 服务端状态 | TanStack Query v5 | 任务/项目/图数据的缓存与失效 |
| 路由 | React Router v7 | 页面级路由 |
| 图可视化 | D3.js / vis-network | 全景地图（力导向图） |
| 实时通信 | WebSocket 客户端封装 | 对接 `/ws/events` |
| 打包分发 | electron-builder | 与 Go 交叉编译产物合并发布 |

### 1.3 交付物

- 守护进程 / CLI：Go 交叉编译，**6 个产物**（Windows / macOS / Linux × x64 / arm64），全部 `CGO_ENABLED=0`。
- APP：electron-builder 打包，内嵌对应平台守护进程二进制。

---

## 2. 系统架构与模块边界

### 2.1 架构总览（单守护进程 · 多项目）

```
                         ┌───────────────────────────────────┐
                         │         守护进程 (Go, 单实例)        │
┌──────────────┐  HTTP/WS │  ┌────────────┐  ┌──────────────┐  │
│  APP (UI)    │◄─────────┼─►│ 项目注册表   │  │  权限/来源识别  │  │
└──────────────┘  X-UI-Token│  └────────────┘  └──────────────┘  │
                            │  ┌────────────┐  ┌──────────────┐  │
┌──────────────┐  HTTP+Token│  │  SQLite    │  │  LLM 客户端   │  │
│  CLI (Go)    │◄─────────┼─►│  (meta.db)  │  └──────────────┘  │
└──────────────┘ X-Project │  └────────────┘                     │
                            │  ┌────────────┐  ┌──────────────┐  │
┌──────────────┐  stdio MCP  │  │ Skill 索引  │  │ 审计 (异步)   │  │
│ AI MCP 客户端 │◄─────────┼─►│  └────────────┘  └──────────────┘  │
└──────────────┘   project  │                                     │
                            └───────────────────────────────────┘
```

- 所有外部交互统一由**单一守护进程**处理；项目以**工作目录路径**为唯一标识。
- 守护进程内置进程级**单实例锁**（监听端口 + PID 文件双重检测）。
- 守护进程常驻后台（APP 退出不关闭）；MCP / CLI / APP UI 访问时若发现未运行则**自动拉起**。

### 2.2 后端模块划分

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
├── skill/      # Skill 文件扫描、索引、skill_info
├── llm/        # LLM HTTP 客户端封装（供 parser/exporter 复用，仅 JSON 结构化通信）
└── audit/      # 审计日志异步写入与导出
```

### 2.3 分层约束（铁律）

- **业务层**（`task / parser / exporter / skill`）**禁止**引用 `api`、`mcp`、`cmd` 包。
- **数据库事务边界**必须在业务层控制，`db` 层仅提供原生 SQL 或 Query Builder。
- **传输层**（`api / mcp / cmd/cli`）为薄封装：只做参数解析与响应格式化，**禁止重复业务逻辑**。
- HTTP Handler、CLI Command、MCP Tool 必须**共享同一套业务层实现**，不得重复造轮子（接口先行）。

---

## 3. 后端（Go）技术规范

### 3.1 编码铁律

1. **零信任依赖**：禁止引入任何需要 CGO 或外部动态链接库的依赖；数据库仅用 `modernc.org/sqlite`；所有产物须 `CGO_ENABLED=0` 编译通过。
2. **接口先行**：所有功能模块（CRUD、归档/还原、导入导出、权限校验、状态机）先定义清晰 Interface，三端共享实现。
3. **显式优于隐式**：
   - 禁止硬编码路径，一律基于 `--workdir` 或环境变量动态拼接。
   - 所有跨端调用必须显式携带项目目录（HTTP `X-Project` / CLI `--project` / MCP 参数 `project`），未携带或未注册返回 `PROJECT_NOT_FOUND`。
   - 配置分层严禁混淆：
     - **全局配置** `~/.taskboard-app/config.yaml`：LLM 密钥、监听端口、远程访问开关、API Token、UI 会话凭据。
     - **项目配置** `{workdir}/.taskboard/config.yaml`：导出模板、状态机、自定义字段等业务配置。

### 3.2 数据模型

**Task 结构体（Go）**

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

**核心表**（DDL 以 `REQUIREMENTS.md` §四.4 为准）

| 表 | 关键字段 | 说明 |
|----|----------|------|
| `projects` | `id, name, workdir(UNIQUE), created_at, last_opened_at` | 仅记录"名称 + 工作目录"；移除记录**绝不删除磁盘数据** |
| `tasks` | 见上；`project_id` 外键 + `(project_id, status)` 索引 | |
| `permissions` | `(project_id, action, allowed)` | **仅存 Agent 权限范围**；UI 不查表 |
| `import_drafts` | `id, project_id, source_file, parsed_json, status(pending/confirmed/discarded)` | LLM 解析草稿，确认后入库 |
| `skills` | — | 仅缓存，数据源为 `{workdir}/.taskboard/skills/` 文件系统 |
| `audit_log` | `id, ts, actor, actor_class, action, target, result, detail` | 仅写操作记录，读取不记录 |

**业务约束（铁律）**

- **状态机**：由项目 `config.yaml` 的 `state_machine` 节驱动（每项目独立）；状态流转必须满足 `transitions`，非法流转返回 `INVALID_TRANSITION`；**有任务占用的状态不可移除**。
- **删除 = 归档**：删除统一为归档（`status=archived` 并记录 `archived_from`），回收站可还原到归档前状态。
- **级联规则**：归档/物理删除父任务时，所有子任务**级联置空 `parent_id`**；**物理删除子任务严格禁止**，物理删除仅限回收站中的任务。
- **depends_on 无环**：Create/Update 引入循环依赖时**写操作直接拒绝**（`CIRCULAR_DEPENDENCY`）。
- **导入走草稿流程**：解析结果先入 `import_drafts`，确认后按 `source_file` **文件级全量覆盖**入库；解析失败（缺 title/status / JSON 不合规）整次失败不落库。

### 3.3 网络与安全红线

**绑定地址与来源过滤（热切换）**

```go
// 守护进程始终监听 0.0.0.0:19810（端口可由全局配置热重载调整）
// 远程访问开关为内存标志，切换即时生效，无需重启、无需重绑地址
if !globalConfig.RemoteAccess && !isLoopback(remoteAddr) {
    reject(403) // "remote access disabled"
}
```

**来源识别（Actor 判定，按优先级）**

1. 请求携带 `X-UI-Token` 且来源为回环 → `actor_class=ui`（App UI，全权限）。
2. MCP stdio 会话 → `actor_class=agent`，`actor` 为客户端名称。
3. 远程请求携带 `Authorization: Bearer <api_token>` → `actor_class=agent`，`actor` 取 `X-Actor` 或回退 `unknown`。
4. 本地 HTTP / CLI（含 `X-Actor` 头，CLI 默认 `human`）→ `actor_class=agent`（最小信任，视同 Agent 查权限表）。
5. 无法识别的请求 → `actor_class=unknown`，按 Agent 权限表检查并记录审计。

**权限中间件**

- 所有 `/api/*` 路由必须经过 auth 中间件完成来源识别。
- `actor_class == "ui"` → 直接放行（不查权限表）；其余一律查询 `permissions` 表对应 `action` 开关，未授权返回 403。
- **AI Agent（MCP）在任何情况下都不可修改权限表**：`PUT /api/permissions` 额外校验 `X-UI-Token` + 回环 IP，双条件缺一即拒（即使 `RemoteAccess` 开启）。
- 远程连接必须携带 `Authorization: Bearer <api_token>`，否则 401。

### 3.4 API / WebSocket / 项目标识

**项目标识约定（每次调用必须显式）**

| 端 | 标识方式 |
|----|----------|
| HTTP | 请求头 `X-Project: <工作目录绝对路径>`，或查询参数 `?project=`（URL 编码） |
| CLI | 子命令参数 `--project <工作目录>`（强制） |
| MCP | 每个工具的参数 `project`（强制） |
| WebSocket | 连接查询参数 `?project=` |

**API 响应格式（统一）**

- 成功：`{"code":0, "data": {...}}`
- 失败：`{"code":非零, "message": "可读错误", "detail": "技术细节(可选)"}`
- 业务错误码：`PROJECT_NOT_FOUND` / `INVALID_TRANSITION` / `CIRCULAR_DEPENDENCY` / `STATUS_IN_USE` / `PERMISSION_DENIED` / `IMPORT_FAILED` 等。

**WebSocket 实时事件**（`/ws/events?project=<dir>`，事件结构 `{type, project, data, ts}`）

| 命名空间 | 事件 |
|----------|------|
| `task.*` | `task.created / task.updated / task.archived / task.restored / task.status_changed` |
| `import.*` | `import.draft_ready / import.draft_confirmed / import.draft_discarded / import.failed` |
| `export.*` | `export.complete` |
| `project.*` | `project.imported / project.removed` |
| `permission.*` | `permission.changed` |
| `skill.*` | `skill.changed` |
| `state_machine.*` | `state_machine.changed` |

### 3.5 LLM 交互规范

**Markdown 解析入库（parser）**

- 调用 LLM 的 Prompt 必须包含严格的 JSON Schema 约束。
- 若 LLM 返回的 JSON 缺少 `title` 或 `status` 字段 → **必须拒绝入库**并返回明确错误，**禁止补默认值**（由上游用户处理）。
- 解析成功 → 生成草稿（`import_drafts`）→ 调用方显式确认后入库；提供 `import_preview / import_confirm / import_discard` 等价端点。

**Markdown 导出模板（exporter）**

- 默认模板：`internal/exporter/templates/default.tmpl`。
- 用户自定义模板：项目配置 `config.yaml` 的 `export.template_path` 覆盖。
- 支持 LLM 生成模板：`POST /api/export/template/generate`（用户提供示例文档 → LLM 生成贴近原始风格的模板）。
- 模板引擎仅用 Go `text/template`。

### 3.6 审计日志（不可篡改）

- 所有写操作（Create / Update / Archive / Restore / StatusChange / Import / Export / 权限与状态机修改）必须**异步**写入 `audit_log` 表（数据本体）；`audit.log` 文件仅为按需导出物。
- 审计字段：`id, ts, actor, actor_class, action, target, result, detail`；**仅写操作记录**。

### 3.7 Go 代码风格

- 遵循标准 `gofmt`；变量名尽量简短（`t` 代表 Task，`db` 代表 Database）。
- 错误处理：`fmt.Errorf("...: %w", err)` 包裹，便于溯源。
- 日志：`slog`，级别 `Debug`（开发）/ `Info`（生产默认）/ `Error`（异常）。

### 3.8 测试要求

- **单元测试**：所有业务层逻辑必须附带 `_test.go`，使用 `sqlite:memory:` 模式隔离测试，**不允许依赖本地文件系统**。
- **集成测试**：位于 `/test/integration`，启动临时 Daemon 并调用真实 HTTP 客户端。
- **覆盖率**：核心 `internal/task` 不低于 **90%**（含状态机校验、归档/还原、依赖校验、草稿导入流程）。

---

## 4. 前端（React 19 + shadcn-ui）技术规范

> 本章为本次抽离时补充的前端技术细节，对应 REQUIREMENTS.md 中「APP GUI = Electron + React + D3/vis-network」。

### 4.1 工程结构与目录约定

```
app/                                # 前端工程（Electron + Vite）
├── electron/                       # 主进程 + 预加载脚本
│   ├── main.ts                     # 窗口创建、守护进程生命周期、安全策略
│   ├── preload.ts                  # contextBridge 暴露白名单 IPC API
│   └── daemon.ts                   # 内嵌守护进程：拉起/探活/单实例检查
├── src/
│   ├── main.tsx                    # React 入口
│   ├── App.tsx                     # 路由 + 全局布局
│   ├── components/
│   │   ├── ui/                     # ⭐ shadcn-ui 组件（CLI 生成，随项目维护）
│   │   ├── kanban/                 # 看板：列、卡片、拖拽
│   │   ├── graph/                  # 全景地图：D3 / vis-network 封装
│   │   └── common/                 # 业务通用组件（标签徽章、优先级色带等）
│   ├── features/                   # 按业务域组织
│   │   ├── tasks/                  # 任务 CRUD、状态流转、归档/还原
│   │   ├── projects/               # 项目管理、导入
│   │   ├── imports/                # 导入草稿预览/确认/丢弃
│   │   ├── permissions/            # Agent 权限管理（仅 UI）
│   │   └── skills/                 # Skill 浏览
│   ├── stores/                     # Zustand stores（UI 局部状态）
│   ├── api/                        # HTTP + WebSocket 客户端封装
│   ├── hooks/                      # TanStack Query hooks（useTasks 等）
│   ├── lib/                        # 工具：cn()、标签颜色哈希、日期格式化
│   ├── types/                      # TS 类型（与后端 DTO 一一对应）
│   └── styles/globals.css          # Tailwind + shadcn-ui 主题变量
├── components.json                 # shadcn-ui 配置文件
├── vite.config.ts                  # electron-vite 配置
└── package.json
```

### 4.2 shadcn-ui 组件体系规范

**使用方式（铁律）**

- 组件通过 `npx shadcn@latest add <component>` 安装，**源码复制进 `src/components/ui/`**，不是 npm 依赖——可按需修改，但修改需在代码评审中显式说明。
- 初始化时配置 `components.json`：`style: new-york`、`cssVariables: true`、`tailwind`（Tailwind CSS v4 走 CSS-first 配置）。
- **业务组件禁止直接使用 Radix UI 原语**，必须经由 `ui/` 封装层；新增组件前先检索 `ui/` 是否已有。

**主题定制**

- 颜色一律通过 CSS 变量驱动（`--background / --foreground / --primary / --destructive / --muted` 等），业务代码禁止硬编码色值。
- 支持深浅双主题：根节点切换 `class="dark"`，主题偏好持久化到本地（实现可参考 next-themes 思路，Electron 环境用自实现 class 切换即可）。
- 业务语义色（如优先级红→灰渐变、标签徽章色）基于 CSS 变量或工具类组合，标签徽章 v1 按标签名**稳定哈希**生成色相（无需存储），自定义映射留 v2。

**与命令式可视化库（D3 / vis-network）的结合**

- D3 / vis-network 属命令式渲染，React 组件只负责容器与数据传递：
  - `useRef` 持有图实例，`useEffect` 内创建/销毁；
  - 数据更新通过 `setData` / 实例方法增量更新，**避免全量重渲染**；
  - 组件卸载时显式销毁实例，防止内存泄漏。

### 4.3 状态管理与数据流（Zustand + TanStack Query + WebSocket）

**分层原则**

| 状态类型 | 归属 | 示例 |
|----------|------|------|
| 服务端状态 | TanStack Query | tasks、projects、graph、skills、import drafts |
| UI 局部状态 | Zustand | 看板过滤、当前选中项、模态框开关、拖拽中间态 |
| 派生状态 | React `useMemo` | 看板列分组、统计角标 |

- **组件不直接持有服务端数据**：一律经 Query hook 读取，经 Mutation 写入。
- **写操作统一走 Mutation 层**，配合乐观更新 + 失败回滚；状态机校验类错误（`INVALID_TRANSITION` 等）由后端返回，前端负责把错误态回滚并提示。

**WebSocket 事件 → 缓存同步（核心联动）**

- 渲染进程建立 `/ws/events?project=<dir>` 长连接（携带 `X-UI-Token`）。
- 事件驱动 Query 失效/更新，映射表：

| 事件 | 前端动作 |
|------|----------|
| `task.created / updated / archived / restored / status_changed` | `invalidateQueries(['tasks'])` 或按 id 精准 `setQueryData` |
| `import.draft_ready / confirmed / discarded / failed` | `invalidateQueries(['import-drafts'])` |
| `permission.changed` | `invalidateQueries(['permissions'])` |
| `state_machine.changed` | 重新拉取状态机配置，重建看板列 |
| `skill.changed` | `invalidateQueries(['skills'])` |

- **断线重连**：指数退避（1s → 2s → 4s → 上限 30s）；重连成功后全量刷新当前视图的查询。
- **多项目切换**：切换项目时断开旧连接、建立新连接，并清理该项目的查询缓存。

**HTTP 客户端约定**

- 所有请求携带：`X-UI-Token`（UI 会话凭据，App 启动时从全局配置读取）+ `X-Project`（当前项目绝对路径）。
- 统一响应处理：`code === 0` 取 `data`；否则按业务错误码映射为可读提示（错误码清单见 §3.4）。

### 4.4 Electron 集成

**进程模型与安全基线**

- `contextIsolation: true`、`nodeIntegration: false`、渲染进程启用 `sandbox`。
- 渲染进程**只能通过 preload 暴露的白名单 IPC API** 与主进程通信（如 `daemon:ensureRunning`、`config:readUiToken`），禁止直接暴露 Node 能力。
- 主进程负责：创建窗口、守护进程生命周期（拉起/探活/日志）、全局配置读取、应用菜单与快捷键。

**内嵌守护进程管理**

- App 启动时：主进程检测 `127.0.0.1:19810` 是否存活 → 未存活则 spawn 内嵌 daemon 二进制并等待 Health Check（`GET /ping`）通过。
- App 退出：**不关闭守护进程**（常驻后台，符合需求 N6）；提供"退出时关闭守护进程"的可选开关。
- 单实例检查：端口 + PID 文件双重检测，避免重复拉起。

### 4.5 关键视图实现要点

| 视图 | 要点 |
|------|------|
| **看板** | 按状态机动态生成列；拖拽卡片触发 `status_changed` Mutation，`INVALID_TRANSITION` 时回滚并 toast 提示；优先级色带**红色最高 → 灰色最低**渐变；标签徽章按标签名哈希着色 |
| **导航** | 树形列表 / 时间线 / 状态分类三种视图，支持过滤、搜索、折叠 |
| **全景地图** | `GET /api/graph` 取**全量**数据（服务端不聚簇）；节点颜色映射状态；前端在节点数超过阈值时**分片/聚簇渲染**；仅全局概览，不做任务推荐/优先级计算 |
| **导入导出** | 草稿确认流：`import_preview` 展示解析结果 → `import_confirm / import_discard`；Markdown 导出前提供渲染预览 |

### 4.6 性能要求（数据规模 1,000 – 5,000 任务）

- 看板/树形列表使用**虚拟滚动**（推荐 `@tanstack/react-virtual`），卡片组件 `React.memo` 化。
- TanStack Query 启用缓存 + 按需分页；切换项目时清理旧缓存。
- 全景地图大数据量渲染放入 Web Worker 或使用 Canvas 模式（D3/vis-network 均支持），避免阻塞主线程。

### 4.7 前端测试与质量

- **测试框架**：Vitest + React Testing Library；服务端数据用 MSW 拦截 HTTP，WebSocket 用 mock 客户端模拟事件。
- **测试范围**：Query/Mutation hooks（`renderHook`）、shadcn-ui 封装组件、关键业务组件（看板拖拽、草稿确认流）。
- **代码质量**：ESLint（`typescript-eslint` + `react-hooks` 规则集）+ Prettier；提交前 lint-staged 检查。
- **类型对齐**：`types/` 下 DTO 与后端 `Task` 等结构体保持字段级同步，接口变更需同步更新（可加 CI 契约校验）。

---

## 5. 抽离说明（与 AGENTS.md 的映射）

| AGENTS.md 章节 | 处理 | 去向 |
|----------------|------|------|
| §1 项目世界观 | 保留原文 | AGENTS.md（理念，非技术） |
| §2 编码核心约束 | 抽离 | 本文档 §3.1 |
| §3 架构与模块边界 | 抽离 | 本文档 §2 |
| §4 数据模型细节 | 抽离 | 本文档 §3.2 |
| §5 AI Agent（LLM）交互规范 | 抽离 | 本文档 §3.5 |
| §6 网络与安全红线 | 抽离 | 本文档 §3.3 |
| §7 命名与代码风格 | 抽离 | 本文档 §3.7 |
| §8 测试要求 | 抽离 | 本文档 §3.8 |
| §9 当前开发阶段重点任务 | 保留原文 | AGENTS.md（阶段性指引） |
| §10 遇到不确定怎么办 | 保留原文 | AGENTS.md（工作流约束） |
| —（新增）前端 React + shadcn-ui 细节 | 新增 | 本文档 §4 |

---

## 附录 A：QA 确认记录与待补充问题

**已确认（本次 QA）**

1. 构建方案：以项目文档为准 → Electron + React（REQUIREMENTS.md §5.1 / §四.2）。
2. React 版本：**React 19**。
3. 客户端状态管理：**Zustand + TanStack Query**。

**合理默认（按主流实践设定，可复核）**

- 构建：electron-vite（main / preload / renderer 三端统一构建）。
- 样式：Tailwind CSS v4 + CSS 变量主题（shadcn-ui 新版本默认方案）。
- 路由：React Router v7。
- 测试：Vitest + React Testing Library + MSW。
- 代码质量：ESLint + Prettier + lint-staged。
- 包管理器：pnpm。

**开放问题（待后续确认，不影响本文档主体）**

1. 全景地图可视化库：D3 与 vis-network 二选一，或按视图拆分使用？
2. 是否需要 `graph` 视图的大图分片策略阈值（建议 2,000 节点）？
3. 前端是否需要国际化（i18n）？v1 默认中文 UI？
4. 深浅主题是否跟随系统（`prefers-color-scheme`）还是仅手动切换？
