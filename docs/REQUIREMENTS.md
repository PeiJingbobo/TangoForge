# 人机协作任务看板系统（TangoForge）——需求文档与技术方案

> **版本**：v1.1（需求评审修订版）
> **修订说明**：基于 v1.0 的 35 项需求评审问答（见 `REQUIREMENTS-REVIEW.md`）逐项确认后修订。本文件为**最高优先级需求基线**；`AGENTS.md` 中的约束若与本文件冲突，以本文件为准（差异清单见 §10）。
> **关键词**：本地优先 · 多端等价 · 单守护进程多项目 · Agent 一等公民

---

## 一、产品概述

TangoForge 是一个**人机协作的任务看板系统**。用户通过 APP 图形界面管理任务，AI Agent 通过 CLI、HTTP API 或 MCP 协议以**受权限约束的等价能力**操作同一任务池。

- **数据是主人**：所有数据自包含在用户选定的本地 `.taskboard/` 目录中，无云端依赖。
- **界面是仆人**：APP 只是访问数据的渠道之一，CLI、MCP、HTTP API 拥有完全等价的操作权。
- **AI 是头等公民**：Agent 通过 MCP 协议原生操作任务池，其权限范围由用户单独配置。
- 支持将非结构化的 Markdown 任务文档经 LLM 解析为结构化数据，并可随时从结构化数据重新生成 Markdown。

---

## 二、核心功能需求

### 1. 项目与配置管理

#### 1.1 单守护进程 · 多项目模型

- 系统采用**单守护进程管理多个项目**的架构，不存在端口冲突问题（守护进程固定监听 19810，或按 §4.6 热重载调整）。
- 项目以**工作目录路径**为唯一标识。Agent 在调用 MCP / CLI / HTTP 接口时，**必须显式指明当前操作的项目目录**（标识约定见 §5.2）。
- 权限、状态机、审计均**以项目（工作目录）为单位**隔离。

#### 1.2 工作目录与 `.taskboard/`

用户选择任一本地文件夹作为项目工作目录，目录下自动创建隐藏目录 `.taskboard/`：

| 文件/目录 | 说明 |
|-----------|------|
| `meta.db` | SQLite 数据库：任务、权限、审计、草稿、技能索引 |
| `config.yaml` | **仅限当前项目的业务配置**：导出模板、状态机、自定义字段、标签颜色等；不含全局/安全设置 |
| `skills/` | 已废弃（TF-033）：技能包改内置 embed + 全局技能库 `~/.taskboard-app/skills/` |
| `audit.log` | 审计日志的**可导出文件**（数据本体在 `audit_log` 表，导出见 §7.5） |

#### 1.3 全局配置（APP 级）

APP 独立维护跨项目的全局配置文件 `~/.taskboard-app/config.yaml`：

- 守护进程监听端口（默认 `19810`）。
- LLM 服务配置（接口地址、认证密钥、模型名、超时、重试次数、max_tokens、并发数）。
- **允许远程访问开关**（`remote_access`）。
- **API Token**（远程访问凭证，见 §7.3）。
- **UI 会话凭据**（`ui_token`，由守护进程首次启动时生成，用于来源识别，见 §7.2）。
- 其他全局偏好。

**全局配置支持热重载**（见 §3.7）：修改端口、远程开关、LLM 配置后**无需重启守护进程**即自动生效。

#### 1.4 项目生命周期

- **导入项目**：任何目录均可随时"导入为项目"。若目录中不存在 `.taskboard/` 元数据，则引导用户完成**初始化**（创建 `.taskboard/` 及 `meta.db`、默认配置、默认状态机）。
- **项目列表**：仅是一条记录（项目名称 + 工作目录路径），项目**并非从 APP 中新建**。
- **移除项目记录**：仅删除 `projects` 表中的记录，**绝不删除、修改工作目录中的任何元数据或项目文件**；删除后再次导入同一目录，原有数据完整保留。
- **数据删除**：只有用户手动删除 `.taskboard/` 目录才会真正销毁数据（系统不做任何自动物理清理）。

### 2. 任务管理

#### 2.1 任务字段

`id`（UUID v4）、`project_id`、`parent_id`（父子关系）、`title`、`description`、`status`、`priority`、`tags`、`assignee`、`depends_on`、`archived_from`、`source_file`、`source_section`、`created_at`、`updated_at`。

#### 2.2 状态机（可配置 · 每项目独立）

- **每个项目单独维护状态机**，定义在项目 `.taskboard/config.yaml` 的 `state_machine` 节（状态列表 + 流转规则），是**项目业务配置**的一部分。
- 默认状态机（四态 + 系统保留态）：

  ```
  待办(todo) → 进行中(doing) → 已完成(done)
      ↑___________|____________↓（允许回退）
  archived（已归档）：系统保留状态，由"归档/还原"操作专用，不可出现在普通流转编辑中
  ```

- 状态机可编辑：新增/重命名/调整流转规则，但**若有任务处于某状态，则该状态不可移除**（服务端校验拒绝，并返回占用任务数）。
- 状态流转必须满足 `transitions` 定义，非法流转由 service 层拒绝（错误码 `INVALID_TRANSITION`）。
- AI Agent 读取状态机定义与执行状态流转均受权限控制。

#### 2.3 归档与回收站（删除语义）

- **删除 = 归档**：删除操作统一命名为"归档"，任务迁移至 `archived` 状态，并记录 `archived_from`（归档前状态）。
- **还原**：回收站（已归档列表）中的任务可一键还原，恢复到 `archived_from` 记录的状态。
- **级联规则**：归档父任务时，子任务**级联置空 `parent_id`**（成为顶层任务），绝不物理删除子任务。
- **物理删除（永久删除）**：仅回收站中的任务可被物理删除；物理删除父任务时子任务同样**禁止物理删除**，改为级联置空 `parent_id`。
- 归档/还原/物理删除均计入审计日志。

#### 2.4 优先级

- **存储**：整数 `0–5`，**0 = 最低**（无优先级），5 = 最高。API 同时接受字符串别名并归一化为整数：

  | 别名 | 值 |
  |------|----|
  | `lowest` / `none` | 0 |
  | `low` | 1–2 |
  | `normal` / `default` | 3 |
  | `high` | 4 |
  | `highest` / `critical` / `urgent` | 5 |

- **UI 展示**：不同颜色标识，**红色最高、灰色最低**，中间档位渐变色。
- **排序**：`priority DESC, created_at ASC`。
- **AI 协作**：Agent 可通过 API 读取/修改优先级，受"修改任务字段"权限控制；导入 Markdown 时由 LLM 语义推断（如"【紧急】修复登录页" → `priority: 5`）。

#### 2.5 指派（assignee）

- **自由文本字符串**：可存用户名、邮箱、团队角色名，或留空（未分配）。示例：`张三`、`zhangsan@example.com`、`前端组`。
- **作用**：按人筛选与分工；Agent 可通过 API 查看/修改指派（受"修改任务字段"权限控制），如将任务转交给特定成员。
- **导入推断**：LLM 从文本推测，如"@李四 完成接口文档" → `assignee: 李四`。

#### 2.6 标签（tags）

- **存储**：`tags` 字段为 JSON 字符串数组（如 `["bug", "v2.0", "ui"]`）。
- **UI 功能**：看板支持按标签过滤；标签以**小徽章形式**展示在卡片上，徽章颜色 v1 由 UI 按标签名稳定哈希生成（无需存储），自定义颜色映射留 v2（存项目 config）。
- **AI 协作**：Agent 创建/更新任务时可指定标签（受"修改任务字段"权限控制）；导入时 LLM 从内容推断（如"修复登录页崩溃" → 标签 `bug`）。

#### 2.7 依赖关系（depends_on）

- `depends_on` 为 JSON 数组，存储被依赖的 Task ID。
- **循环依赖校验**：Create/Update 时若引入环（A→B→A），**写操作直接拒绝**并返回明确错误（错误码 `CIRCULAR_DEPENDENCY`）。
- 删除（归档）被依赖任务时：不做强制阻断，但归档接口返回"被 N 个任务依赖"的提示信息，由调用方决定是否继续。

### 3. Markdown 导入（LLM 驱动）

#### 3.1 解析流程

1. 用户（任意端）提交 Markdown 文件路径或内容。
2. 守护进程将全文发送给 LLM，LLM 提取任务、层级、状态、优先级、标签、指派、依赖等，返回**严格 JSON Schema 约束**的结构化结果。
3. 解析**不依赖固定语法或正则**，全部由 LLM 语义理解完成。

#### 3.2 草稿确认流程

- LLM 解析成功的结果**先进入"草稿"（不直接入库）**：写入 `import_drafts` 表，返回 `draft_id` 与草稿任务列表。
- 用户在任意端**预览核对后显式确认**，确认后批量写入 `tasks` 表（保留 `source_file` / `source_section` 映射）。
- 草稿可被**丢弃**；草稿在项目切换/任务变更时不影响正式任务池。

#### 3.3 失败处理

- 任一环节失败（JSON 不合规 / 缺少 `title` / 缺少 `status` / LLM 超时）：**整次导入失败，不落库**，返回错误信息 + LLM 原始输出，供人工排查；禁止补默认值。
- 不存在"部分成功"状态。

#### 3.4 重复导入策略

- **文件级全量覆盖**：以 `source_file` 为同步单元，同一文件再次导入并确认后，删除该文件来源的全部旧任务（软删除→归档），按新结果重建。
- 增量合并（按标题/ID 匹配）列为 V2 候选（§5.1）。

#### 3.5 同步方向

- **单向显式同步**：库 → MD 只在用户显式导出时发生；MD → 库 只在用户显式导入时发生。**无文件变化自动监听**。

### 4. Markdown 导出

#### 4.1 按需生成

- 任务数据修改后**不会自动写回**原 Markdown。
- 提供"从结构化数据重新生成 Markdown"的导出功能：基于数据库当前状态，按模板渲染；可选择覆盖原文件或另存为副本。

#### 4.2 模板系统（两种模式）

- **默认模板**：内置模板（Front Matter + 标题层级表达父子 + checkbox 表达状态），结构草案：

  ```markdown
  ---
  title: "{{project_name}}"
  generated_at: "{{timestamp}}"
  ---

  ## {{任务标题}}          <!-- 顶层任务：##；子任务逐级增加 # -->
  - [ ] 描述……            <!-- [ ] 待办 [~] 进行中 [x] 已完成 -->
  - 优先级: {{priority}} | 标签: {{tags}} | 负责人: {{assignee}}
  ```

- **LLM 生成模板**：用户可提供示例文档，由 LLM 分析其结构生成贴近原始文档风格的导出模板，存入项目 `config.yaml` 的 `export.template_path`。
- 模板引擎使用 Go `text/template`，禁止引入其他模板语言。

### 5. 多端等价操作

#### 5.1 交互端（功能完全等价）

| 端 | 说明 |
|----|------|
| **APP GUI** | Electron + React，内嵌守护进程二进制 |
| **CLI** | 单一静态二进制，所有子命令转为 HTTP 调用 |
| **HTTP REST API** | 完整业务能力 |
| **MCP 服务** | stdio 模式，固定工具集（§8.3） |

所有操作（创建、读取、更新、归档、状态流转、导入导出、项目管理）均通过四端提供。

#### 5.2 项目标识约定（多项目）

Agent / 客户端在每次调用中**必须显式指明项目目录**：

| 端 | 标识方式 |
|----|----------|
| HTTP | 请求头 `X-Project: <工作目录绝对路径>`，或查询参数 `?project=`（URL 编码） |
| CLI | 子命令参数 `--project <工作目录>`（强制） |
| MCP | 每个工具的参数 `project`（强制） |
| WebSocket | 连接查询参数 `?project=` |

- 项目路径未在 `projects` 表中注册 → 返回 `PROJECT_NOT_FOUND`，并提示"该目录尚未导入为项目"。
- 所有权限校验按该项目（工作目录）的权限配置执行。

#### 5.3 实时变更（WebSocket）

- 路径 `/ws/events?project=<dir>`，事件类型带命名空间前缀：

  | 命名空间 | 事件 |
  |----------|------|
  | `task.*` | `task.created / task.updated / task.archived / task.restored / task.status_changed` |
  | `import.*` | `import.draft_ready / import.draft_confirmed / import.draft_discarded / import.failed` |
  | `export.*` | `export.complete` |
  | `project.*` | `project.imported / project.removed` |
  | `permission.*` | `permission.changed` |
  | `skill.*` | （TF-033 起无 DB 缓存，状态实时扫描，无事件推送） |
  | `state_machine.*` | `state_machine.changed` |

- 事件结构含 `{type, project, data, ts}`，远程连接需 Token（§7.3）。

### 6. 任务导航与全景地图

- **导航视图**：交互式树形列表、时间线或状态分类展示，支持过滤、搜索、折叠。
- **全景地图**：将全部任务及父子/依赖关系渲染为节点图（思维导图/力导向图），节点颜色映射状态，支持缩放拖拽；**仅用于全局概览，不进行任务推荐或优先级计算**。
- **数据规模**：单项目任务量级预期 **1,000 – 5,000**。
- **聚簇边界**：`GET /api/graph` 返回**全量元数据，服务端不聚簇**；聚簇仅在 **APP UI 展示层**进行（如节点超过阈值时前端分片/聚簇渲染），保证 Agent 拿到的永远是完整数据。
- 视图对应的结构化数据与文本描述均可通过 API 获取。

### 7. 权限与安全

#### 7.1 权限模型（仅约束 Agent）

- **只为 Agent 规定权限范围**；用户 App UI 操作默认拥有全部权限（不查权限表）。
- 权限**以项目为单位**，存储在 `permissions` 表：`(project_id, action, allowed)`，action 采用命名空间字符串。
- v1 Agent 权限动作清单：

  | action | 说明 |
  |--------|------|
  | `project.read` | 查看项目列表/信息 |
  | `task.read` | 读取任务（含树、图、搜索） |
  | `task.create` | 创建任务 |
  | `task.update` | 修改任务字段（标题/描述/优先级/标签/指派/依赖） |
  | `task.update_status` | 状态流转 |
  | `task.delete` | 归档（删除） |
  | `task.restore` | 从回收站还原 |
  | `import.run` | 提交 Markdown 解析（生成草稿） |
  | `import.confirm` | 确认草稿入库 |
  | `export.run` | 导出/重新生成 Markdown |
  | `graph.read` | 获取全景图数据 |
  | `skill.read` | 查询 skill_info |
  | `state_machine.read` | 读取状态机定义 |
  | `state_machine.write` | 修改状态机（默认关闭） |
  | `audit.read` | 查询审计日志（默认关闭） |
  | `permission.read` | 查询自身权限范围（默认开启） |

- **新项目默认值**：Agent 默认只读——`task.read / graph.read / skill.read / project.read / permission.read = true`，其余 `false`。用户可在 APP 中调整。

#### 7.2 来源识别策略（UI 与 Agent 的安全区分）

由于 UI 与 Agent 授权范围不同，守护进程对每个请求执行**来源识别（actor_class）**：

| 来源类别 | 识别方式 | 权限规则 |
|----------|----------|----------|
| `ui`（用户 App UI） | 请求携带 `X-UI-Token` 会话凭据，且来源为回环地址；凭据由守护进程启动时生成、存入全局配置、App 启动时读取 | **不查权限表**，默认全权限（权限修改端点除外，见 7.4） |
| `agent`（MCP 客户端） | MCP stdio 会话由守护进程直接管理，客户端名称即 actor | 查 `permissions` 表（Agent 范围） |
| `agent`（远程 HTTP） | 携带 API Token（`Authorization: Bearer <token>`） | 查 `permissions` 表；同时校验 Token |
| `agent`（本地 HTTP / CLI） | 携带 `X-Actor` 头（CLI 默认 `human`，可由 `--actor` 覆盖） | 查 `permissions` 表（视同 Agent，最小信任） |
| `unknown` | 无任何凭据的本地请求 | 按 Agent 权限表检查，且审计中记为 `unknown` |

- **安全默认**：无法证明是 UI 的请求一律按 Agent 权限处理。
- CLI 虽为人类工具，但请求可被脚本伪造，因此**本地 HTTP/CLI 请求也走权限表检查**（而非直接放行）。
- 每次请求的识别结果（actor + actor_class）写入审计日志。

#### 7.3 网络层访问控制

- **默认仅回环**：仅接受来自 `127.0.0.1` 的连接，外部请求直接拒绝。
- **远程访问开关**（全局配置 `remote_access`）：开启后接受非回环连接，但**远程连接必须携带 API Token**，否则 `401`。
- Token 同时作为远程请求的身份绑定凭证；本地回环免 Token。
- v1 传输为明文 HTTP + Token；**TLS/HTTPS 列入 V2**（§5.1）。
- 热重载实现：守护进程始终监听 `0.0.0.0:19810`，由中间件按"来源 IP + `remote_access` 开关"动态放行/拒绝，开关切换即时生效、无需重启。

#### 7.4 权限修改通道

- **仅 APP GUI 可修改权限**（权限修改端点额外校验：`X-UI-Token` + 回环来源）。
- CLI / MCP / HTTP API 均**不提供**权限修改端点（仅 `permission.read` 可查询自身范围）。
- 权限变更实时推送 `permission.changed` 事件并写入审计。

#### 7.5 审计日志

- **数据本体存 `audit_log` 表**（可查询、可过滤），`audit.log` 文件仅为**按需导出物**。
- 字段：`id, ts, actor, actor_class(ui/agent/unknown), action, target, result(ok/denied/error), detail`。
- **仅写操作记录**（Create / Update / Archive / Restore / StatusChange / Import / Export / 权限与状态机修改），读取操作不记录。
- 所有写操作**异步**写入，不阻塞业务响应。
- 导出：`GET /api/audit/export` 生成 `audit.log` 文本文件（含被拒绝的请求，result=denied）。

### 8. Skill 与 Agent 引导

#### 8.1 分工约定

- **`AGENTS.md`（项目根目录，用户手写）**：要求 AI Agent **通过 TangoForge 工具进行项目管理**——即"**何时**使用工具"（场景判断）。App 提供**推荐提示词**（中/英文，一键复制）供用户粘贴。
- **Skill 技能包（内置 + 全局技能库）**：告诉 AI **具体功能如何使用**——即"**如何**操作"（工具用法、字段语义、状态机说明）。App 可将技能包**安装到各类 Agent 宿主约定位置**（AGENTS.md / CLAUDE.md / .cursor/rules / copilot / ~/.claude/skills / ~/.workbuddy/skills），建立可发现性。
- **AI 说明书端点（免鉴权）**：`GET /api/guide`（HTTP）/ MCP `guide` 工具 / CLI `tangoforge guide`——AI 未安装任何 Skill 时，先读说明书即可掌握系统全部调用方式（端点表/工具表/语义速查）。
- 协作机制：AI Agent 在需要进行项目管理时，**自动激活对应 Skill** 完成操作（命中场景 → 读 SKILL.md → 按流程调 cli/mcp/http/scripts）；守护进程提供技能包安装/状态查询能力。

#### 8.2 Skill 技能包格式（v2，SKILL.md）

- **SKILL.md**（Anthropic Agent Skills 规范靠拢）：YAML frontmatter + 正文 instructions：

  ```yaml
  ---
  name: taskboard-basic          # 唯一标识
  description: 任务操作指引
  version: "1.0.0"               # 安装状态比对依据
  hosts: [AGENTS.md, CLAUDE.md]  # 适用宿主（空 = 全部）
  when_to_use: 需要管理任务时激活
  ---
  # 正文：场景 → 调用方式（HTTP/MCP/CLI）→ 字段语义
  ```

- **技能包来源**：内置包（随 daemon embed 分发）+ 全局技能库（`~/.taskboard-app/skills/<name>/`，用户自定义/下载落点，同名覆盖内置）+ 全局默认模板（`_template/SKILL.md`，全局设置页可编辑）。
- **安装 = 分发到宿主位置**：marker 宿主（AGENTS.md/CLAUDE.md/copilot）用 `<!-- tangoforge:skill:<name>:begin/end -->` 标记段（多包共存、可撤销）；目录宿主（~/.claude/skills 等）建 `<name>/`；状态实时扫描（missing/current/stale）。
- 解析失败（缺 name / 坏 frontmatter）→ 拒绝或跳过，不阻断。

#### 8.3 MCP 工具集（v1 固定）

v1 MCP 暴露**固定核心工具集**，Skill 仅提供描述性引导，**不动态注册工具**（动态注册列入 V2）：

```
project_list / project_import / project_init / project_create
task_list / task_read / task_create / task_update / task_archive / task_restore
import_preview / import_confirm / import_discard
export_markdown
graph_get
state_machine_get / state_machine_update
skill_info / skill_install / skill_status / skill_uninstall
guide（免鉴权说明书）
permission_list
```

每个工具第一个参数均为 `project`（工作目录路径，强制）。

---

## 三、非功能需求

| 编号 | 需求 | 说明 |
|------|------|------|
| N1 | **零外部依赖** | 守护进程、CLI 均为单一静态可执行文件，`CGO_ENABLED=0` 编译，无运行时依赖 |
| N2 | **本地优先** | 所有数据/配置/技能文件自包含于工作目录 |
| N3 | **实时性** | APP 与守护进程间 WebSocket 即时同步 |
| N4 | **可扩展** | 解析与生成模板可配置；模块化架构支持未来功能扩展（§5） |
| N5 | **性能** | 单项目任务量级 1,000–5,000；`/api/tasks` 树查询 P95 < 200ms（本地回环） |
| N6 | **常驻与自动拉起** | 守护进程**常驻后台**（APP 退出不关闭）；MCP / CLI / APP UI 访问时若发现守护进程未运行，**自动拉起**（§4.5） |
| N7 | **热重载** | 全局配置（端口 / remote_access / LLM 配置）修改后即时生效，无需重启 |
| N8 | **测试要求** | 所有 service 层逻辑附带单元测试（`sqlite:memory:` 隔离，不依赖本地文件系统）；集成测试置于 `/test/integration`（启动临时守护进程调用真实 HTTP 客户端）；核心任务服务覆盖率 **≥ 90%** |
| N9 | **平台矩阵** | Windows / macOS / Linux 均需 **x64 与 arm64** 产物（共 6 种目标） |
| N10 | **审计完整** | 全部写操作可追溯，审计不可篡改（只追加，无修改端点） |

---

## 四、技术方案

### 1. 系统架构（单守护进程 · 多项目）

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

- 所有外部交互统一由**单一守护进程**处理，项目通过 `X-Project` / `--project` / 工具参数 `project` 标识。
- 守护进程内置进程级**单实例锁**（监听端口 + PID 文件双重检测），避免多实例。

### 2. 技术栈

| 组件 | 技术选型 | 说明 |
|------|----------|------|
| 守护进程 & CLI | Go 1.22+ | 单一静态二进制，无 CGO |
| 数据库 | `modernc.org/sqlite` | 纯 Go 实现 |
| HTTP 框架 | Go `net/http` + chi | 轻量路由 |
| WebSocket | nhooyr.io/websocket | |
| MCP | mark3labs/mcp-go 或自研 | stdio 传输、工具注册 |
| 配置热重载 | fsnotify + 原子替换 | 监听全局配置文件 |
| APP 前端 | Electron + React + D3/vis-network | 跨平台桌面应用 |
| LLM | HTTP Client + 用户配置 | OpenAI 兼容接口（含 Ollama 等本地模型） |
| 打包分发 | Go 交叉编译 + electron-builder | 6 平台产物（x64/arm64 × 3 OS） |

### 3. 模块划分（以本需求文档为准）

```
internal/
├── config/     # 全局配置与项目配置加载、合并、热重载（不做业务判断）
├── db/         # SQLite 初始化、迁移（migrate up/down）、连接池
├── task/       # ⭐ 任务核心：模型、CRUD、状态机校验、归档/还原、依赖校验
├── parser/     # LLM 交互：Markdown → 结构化 JSON（草稿生成）
├── exporter/   # 从数据库重建 Markdown（模板渲染、LLM 生成模板）
├── auth/       # 来源识别（ui/agent/unknown）、Token 校验、权限中间件
├── api/        # HTTP / WebSocket 路由与处理器
├── mcp/        # MCP 工具注册与执行（固定工具集）
├── skill/      # Skill 技能包（内置 embed + 全局库）+ 宿主安装/卸载/状态
├── guide/      # AI 使用说明书（免鉴权）
├── llm/        # LLM HTTP 客户端封装（供 parser/exporter 复用）
└── audit/      # 审计日志异步写入与导出
```

**开发规范**：
- `task / parser / exporter` 等业务模块**禁止引用** `api / mcp / cmd` 包。
- 数据库事务边界在业务层控制，`db` 层仅提供原生 SQL / Query Builder。
- 传输层（api / mcp / cli）为**薄封装**，全部复用业务层实现，禁止在传输层重复业务逻辑。

### 4. 数据模型（核心表 DDL 草案）

```sql
CREATE TABLE projects (          -- 项目注册表（仅记录，不含业务数据）
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL,
  workdir       TEXT NOT NULL UNIQUE,   -- 工作目录绝对路径，即项目唯一标识
  created_at    TEXT NOT NULL,
  last_opened_at TEXT
);

CREATE TABLE tasks (
  id            TEXT PRIMARY KEY,               -- UUID v4
  project_id    INTEGER NOT NULL REFERENCES projects(id),
  parent_id     TEXT REFERENCES tasks(id),      -- 层级；归档父任务时置空
  title         TEXT NOT NULL,
  description   TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL,                  -- 状态机 key（默认 todo/doing/done/archived）
  priority      INTEGER NOT NULL DEFAULT 0,     -- 0-5，0=最低
  tags          TEXT NOT NULL DEFAULT '[]',     -- JSON 数组
  assignee      TEXT NOT NULL DEFAULT '',
  depends_on    TEXT NOT NULL DEFAULT '[]',     -- JSON 数组（Task ID），无环约束
  archived_from TEXT,                           -- 归档前状态（还原用）
  source_file   TEXT,
  source_section TEXT,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);
CREATE INDEX idx_tasks_project ON tasks(project_id);
CREATE INDEX idx_tasks_status  ON tasks(project_id, status);

CREATE TABLE permissions (       -- 仅存 Agent 权限范围；UI 不查表
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id),
  action     TEXT NOT NULL,                    -- task.read / task.create / ...
  allowed    INTEGER NOT NULL DEFAULT 0,
  UNIQUE(project_id, action)
);

CREATE TABLE import_drafts (     -- LLM 解析草稿（确认后入库）
  id          TEXT PRIMARY KEY,
  project_id  INTEGER NOT NULL REFERENCES projects(id),
  source_file TEXT NOT NULL,
  parsed_json TEXT NOT NULL,                  -- LLM 结构化结果
  status      TEXT NOT NULL DEFAULT 'pending', -- pending/confirmed/discarded
  created_at  TEXT NOT NULL,
  confirmed_at TEXT
);

-- skills 表已移除（TF-033 v3 迁移 drop）；技能包改内置 embed + 全局库
  name       TEXT PRIMARY KEY,
  content    TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE audit_log (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  ts         TEXT NOT NULL,
  actor      TEXT NOT NULL,                   -- mcp 客户端名 / X-Actor / ui
  actor_class TEXT NOT NULL,                  -- ui / agent / unknown
  action     TEXT NOT NULL,
  target     TEXT NOT NULL,
  result     TEXT NOT NULL,                   -- ok / denied / error
  detail     TEXT
);
CREATE INDEX idx_audit_project ON audit_log(action, ts);
```

项目状态机与导出模板存于项目 `.taskboard/config.yaml`：

```yaml
state_machine:
  states:
    - { key: todo,   label: 待办,   color: "#9aa0a6" }
    - { key: doing,  label: 进行中, color: "#1a73e8" }
    - { key: done,   label: 已完成, color: "#34a853" }
  transitions:
    - { from: todo,  to: [doing, done] }
    - { from: doing, to: [todo, done] }
    - { from: done,  to: [doing, todo] }
  # archived 为系统保留状态，由归档/还原操作专用
export:
  template_path: ""   # 空 = 默认模板；可指向自定义 .tmpl
```

### 5. API 核心端点

> 项目标识：请求头 `X-Project` 或查询参数 `?project=`（绝对路径）；权限修改类端点额外要求 `X-UI-Token` + 回环来源。

| 方法 | 路径 | 说明 | 权限 action |
|------|------|------|-------------|
| GET | `/ping` | 健康检查 | – |
| GET | `/api/projects` | 项目列表 | `project.read` |
| POST | `/api/projects/import` | 导入目录为项目（无元数据则引导初始化） | `project.read`（UI 不限） |
| DELETE | `/api/projects/:id` | 移除项目记录（不动磁盘） | 仅 UI |
| GET | `/api/tasks` | 任务树（`?filter[status]=&q=&page=&size=`） | `task.read` |
| POST | `/api/tasks` | 创建任务 | `task.create` |
| GET | `/api/tasks/:id` | 任务详情 | `task.read` |
| PATCH | `/api/tasks/:id` | 更新字段（含状态流转，校验 transitions） | `task.update` / `task.update_status` |
| POST | `/api/tasks/:id/archive` | 归档（删除） | `task.delete` |
| POST | `/api/tasks/:id/restore` | 还原 | `task.restore` |
| DELETE | `/api/tasks/:id` | 物理删除（仅回收站中任务） | `task.delete` |
| GET | `/api/state-machine` | 读取状态机定义 | `state_machine.read` |
| PUT | `/api/state-machine` | 更新状态机（校验状态占用） | `state_machine.write` |
| POST | `/api/import` | 提交 Markdown 解析 → 生成草稿 | `import.run` |
| GET | `/api/import/drafts` | 草稿列表 | `import.run` |
| POST | `/api/import/drafts/:id/confirm` | 确认草稿入库（文件级覆盖） | `import.confirm` |
| DELETE | `/api/import/drafts/:id` | 丢弃草稿 | `import.run` |
| POST | `/api/export` | 重新生成 Markdown（`template_mode: default\|llm`, `target: overwrite\|copy`） | `export.run` |
| POST | `/api/export/template/generate` | LLM 根据示例文档生成导出模板 | `export.run` |
| GET | `/api/graph` | 全景图全量数据（服务端不聚簇） | `graph.read` |
| GET | `/api/skills/packages[/{name}]` | 技能包列表/详情 | `skill.read` |
| GET | `/api/skills/status` | 宿主安装状态矩阵 | `skill.read` |
| POST | `/api/skills/install|uninstall` | 安装/卸载技能包 | `skill.install` |
| PUT | `/api/skills/packages/{name}` | 写自定义技能包 | 仅 UI |
| GET/PUT | `/api/skill-template` | 全局默认模板读写 | GET skill.read / PUT 仅 UI |
| GET | `/api/guide` | AI 使用说明书 | **免鉴权** |
| GET | `/api/permissions` | 查询 Agent 权限范围 | `permission.read` |
| PUT | `/api/permissions` | 修改权限（仅 UI 凭据 + 回环） | 仅 UI |
| GET | `/api/audit` | 审计查询（`?filter[actor]=&filter[action]=`） | `audit.read` |
| GET | `/api/audit/export` | 导出 audit.log 文件 | `audit.read` |
| WS | `/ws/events?project=` | 实时事件订阅 | 建立连接时校验对应项目 `task.read` |

### 6. 安全实现

- **来源识别中间件**：按 §7.2 判定 `actor_class`，UI 凭据仅回环有效；Agent 请求查权限表；无凭据一律最小信任。
- **网络控制**：守护进程监听 `0.0.0.0:19810`，`remote_access=false` 时中间件拒绝非回环来源（`403`）；开关为内存标志，热切换即时生效。
- **Token 校验**：远程请求必须带 `Authorization: Bearer <api_token>`，否则 `401`。
- **权限修改端点**：额外校验 `X-UI-Token` + 回环 IP，双条件缺一即拒。
- **审计**：所有写操作（含被拒请求）异步写入 `audit_log`。

### 7. 打包与部署

- Go 程序 `CGO_ENABLED=0` 交叉编译，产出 Windows / macOS / Linux × x64 / arm64 共 **6 个目标**的静态二进制。
- CLI 与守护进程为同一二进制（子命令 `daemon` / `cli` / `mcp`）。
- APP 使用 electron-builder 打包，内嵌对应平台守护进程二进制，首次启动自动拉起并生成 `ui_token`。
- 守护进程**常驻**：APP 退出不关闭；CLI / MCP 均可独立使用。
- **自动拉起**：任意端访问前先 `GET /ping`，未运行则自动拉起守护进程（App 内嵌启动；CLI/MCP 通过同二进制 `daemon` 子命令 spawn）。

---

## 五、V2 规划与可扩展性设计

### 5.1 V2 候选功能（当前版本明确不做，但架构已为其预留扩展点）

| 功能 | 说明 | 预留的扩展点 |
|------|------|--------------|
| MCP 动态工具注册 | Skill 文件声明新工具，会话热注册 | 工具命名空间约定（`task_*` / `import_*` 前缀）；`skill_info` 协议版本化 |
| 增量合并导入 | 按标题/ID 匹配的增量同步 | `import_drafts` 表结构预留 `match_key` 语义；导入确认为独立流程，可替换合并策略 |
| 乐观锁 / 冲突检测 | 并发写返回 409 | `updated_at` 已存在；可在 PATCH 增加 `If-Match` 头而不改表 |
| TLS / HTTPS | 远程传输加密 | Token 校验层与传输层解耦，可平级插入 TLS 中间件 |
| 系统服务 / 托盘 | 开机自启、托盘管理 | 守护进程常驻模型已就绪（§4.5），仅需加启动方式 |
| 应用签名 / 自动更新 | 分发安全 | electron-builder 已支持，按需启用 |
| 成员管理 | assignee 字典与校验 | assignee 为自由文本，可升级为外键而不破坏数据 |
| 标签自定义颜色 | 标签颜色映射配置 | 项目 config.yaml 预留 `tags:` 配置节 |
| 双向自动同步 | 文件变化监听 | 导入/导出均为显式端点，可加 watcher 层自动触发 |
| CLI 交互式 TUI | 全键盘操作界面 | CLI 已抽象为 HTTP 调用，TUI 仅新前端 |

### 5.2 未来模块扩展（需求池 / 知识库 / 工作日志等）

后续版本将增加**需求池（requirements）**、**知识库（knowledge）**、**工作日志（work logs）**等任务管理平台常用功能，架构必须保持以下可扩展性约定：

1. **模块即插即用**：新业务模块 = 新 `internal/` 包（如 `internal/requirement`），遵循"业务层独立、传输层薄封装"；`api / mcp / cmd` 仅新增路由与工具注册，不改动既有核心。
2. **权限命名空间自动扩展**：`permissions` 表以 `action` 命名空间字符串存储（`task.*`、`import.*`……），新模块直接追加 `requirement.read`、`knowledge.write` 等动作，**无需改表结构**；默认 Agent 权限仍为只读。
3. **事件命名空间扩展**：WebSocket 事件类型按前缀扩展（`requirement.*`、`knowledge.*`、`log.*`），事件 schema 保持 `{type, project, data, ts}` 不变。
4. **跨模块数据关联**：新表统一携带 `project_id` 外键；任务与其他实体的关联使用 JSON 数组存 ID（与 `depends_on` 同模式），避免硬编码外键耦合。
5. **配置分区**：项目 `config.yaml` 按模块分区（`state_machine:` / `export:` / 未来 `requirements:` / `knowledge:`），互不影响。
6. **Skill 描述协议版本化**：Skill YAML 带 `version` 字段，未来新模块的技能描述向后兼容。
7. **数据库迁移机制**：`internal/db` 提供 migrate up/down，新增模块以迁移脚本形式落地，不破坏既有数据。
8. **MCP 工具前缀保留**：工具名以模块前缀命名（`task_*`、`import_*`、未来 `requirement_*`），防止工具名冲突。

---

## 六、与 AGENTS.md 的关系及待更新项

- **优先级**：本需求文档为需求基线，`AGENTS.md` 为开发约束；二者冲突时**以本文件为准**。
- 评审已确认（Q33）：模块划分**以本需求文档为准**，`AGENTS.md` 中 `internal/` 结构需同步更新为 §4.3 定义（原 `model / service / llm / server` 映射：`model→task 内聚`、`service→task/auth 等`、`server→api`，并新增 `parser / exporter / skill / audit`）。
- 需同步修订 `AGENTS.md` 的其余要点：
  1. §2.3：补充"单守护进程多项目"模型与 `X-Project` 标识约定。
  2. §2.4：审计日志以 `audit_log` 表为准（`audit.log` 为导出物）。
  3. §3：模块边界按本文件 §4.3。
  4. §6.1：绑定逻辑由"按开关二选一绑定"改为"始终监听 + 来源 IP 动态过滤"（热切换）。
  5. §6.2：权限中间件增加来源识别（UI 凭据 / Agent 权限表）；权限修改仅 UI。
  6. §4 数据模型：新增 `projects / import_drafts / audit_log` 表、`archived_from` 字段；`permissions` 表改为 `(project_id, action, allowed)`。
  7. 状态机：由硬编码 `ChangeStatus` 改为**项目级可配置状态机**驱动（仍由 service 层校验）。
  8. 删除语义：明确为"归档 + 回收站还原"（软删除），回收站内可物理删除（子任务仍禁止物理删）。
- **执行方式**：以上更新项经你确认后，将同步修改 `AGENTS.md`。

---

*（文档完）*
