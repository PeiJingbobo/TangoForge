# TangoForge — 任务域语义说明（TASK-SEMANTICS.md）

> **版本**：v1.0（2026-08-06，随 TF-005 建立）
> **定位**：**任务域「定义语义」的统一权威文档**——凡是在代码/接口中约定的语义（字段取值、参数指针语义、过滤/排序/分页规则、错误码、职责边界），一律在此沉淀。`AGENTS.md` 已引用本文件；实现与文档冲突时，以本文件为准并向本文件登记。
> **配套**：`docs/REQUIREMENTS.md` v1.1（需求基线）、`docs/TECHNICAL.md` v1.0（技术落地）、`docs/task/TASKS.md`（任务清单）。
> **建立背景（QA TF-005 确认）**：用户明确要求——**定义语义的相关说明必须形成文档，并在 AGENTS.md 中引用**。此约定自 TF-005 起长期生效，后续任务涉及任何新的"定义语义"（如导入草稿流、导出模板、状态机编辑）均须在此登记。

---

## 1. 项目识别语义（TF-005 QA Q2-B）

- **任务域业务层不依赖全局注册表**（`projects` 表）。项目识别依据**工作目录中的 `.taskboard/meta.db` 元数据存在性**：
  - `{workdir}/.taskboard/meta.db` 存在 → 视为有效项目，可操作；
  - 不存在 → 返回 `PROJECT_NOT_FOUND`（"该目录尚未导入为项目"）。
- **`tasks.project_id` 固定写 1**（与 `permissions` 表一致）：项目库内 `project_id` 仅作文档性冗余，一致性由应用层维护（双库模型，见 `internal/db` 包文档）。
- **项目隔离由"一项目一库文件"天然保证**：不同工作目录对应不同 `meta.db`，跨项目数据互不可见。
- 因此：**删除全局注册表记录（`projects` 表）不影响 `.taskboard/` 目录数据**；重新导入同一目录时，TF-004 的 Import 幂等逻辑会按已有元数据直接复用，目录内容保持原样。
- 传输层（TF-003 中间件）的 `X-Project` 校验仍按全局注册表执行（已验收行为），业务层校验独立按元数据执行；两层语义不一致时以元数据为准的宽松方向收敛（后续 TF 可评估中间件对齐）。

## 2. Task 字段语义

| 字段 | 类型 | 语义 |
|------|------|------|
| `id` | string (UUID v4) | 服务端生成，不可由调用方指定 |
| `project_id` | int64 | 固定 1（见 §1），服务端写入 |
| `parent_id` | *string | nil = 顶层任务；指向同项目内存在的任务 ID |
| `title` | string | 必填，去空白后非空 |
| `description` | string | 默认为空串 |
| `status` | string | 项目状态机 key；创建默认 `todo`；`archived` 为系统保留态（归档/还原专用） |
| `priority` | int | 0–5，0=无优先级/最低，5=最高；别名归一化见 §3 |
| `tags` | []string | JSON 数组；创建时去重 + 去空串 + 保持插入顺序 |
| `assignee` | string | 自由文本（用户名/邮箱/角色），可为空 |
| `depends_on` | []string | 被依赖任务 ID 数组；TF-005 仅存储**不做存在性/环校验**（TF-008 落地 `DEPENDENCY_NOT_FOUND` / `CIRCULAR_DEPENDENCY`） |
| `archived_from` | string | 归档前状态，归档/还原专用（TF-007） |
| `source_file` / `source_section` | string | LLM 导入映射，内部字段 |
| `created_at` / `updated_at` | time.Time | RFC3339 本地时区；`updated_at` 每次写操作刷新 |

## 3. 创建（Create）语义

- `title` 必填：去空白后为空 → `TASK_INVALID`。
- `status`：缺省默认 `todo`；显式传入则校验**存在于项目状态机 states**（读取项目 `config.yaml`，缺失回退默认四态），不存在 → `STATUS_NOT_FOUND`。`archived` 不可作为创建状态（系统保留态）。
- `parent_id`：非空时校验存在且同项目 → `PARENT_NOT_FOUND`；父任务为 `archived` 时允许挂载（TF-007 归档父任务时会级联置空子任务 `parent_id`）。
- `depends_on`：允许写入，不校验（见 §2）。
- `priority` 归一化（严格模式，非法值拒绝 → `TASK_INVALID`，不静默 fallback）：

  | 输入 | 归一值 |
  |------|--------|
  | `0`–`5` 整数 / 数字字符串 | 原值（必须 0–5） |
  | `lowest` / `none` | 0 |
  | `low` | **1**（QA Q5 确认，区间 1–2 取低值） |
  | `normal` / `default` | 3 |
  | `high` | 4 |
  | `highest` / `critical` / `urgent` | 5 |

- `tags`：去重、去空串、保序（QA Q6 确认）。

## 4. 更新（Update）语义（QA Q7 / Q8）

### 4.1 职责边界（Q8）

- **`Update`（任务详情更新）禁止修改 `status`**——UpdateInput 不含 status 字段；
- **状态更新必须走独立的 `ChangeStatus` 接口**（见 §5）；
- 归档/还原走 TF-007 的 archive / restore 接口（`status=archived` 不通过 ChangeStatus）。

### 4.2 部分更新指针语义（Q7-A）

UpdateInput 采用**全指针字段**，nil = 该字段不更新：

| 字段 | 类型 | 语义 |
|------|------|------|
| `title` | *string | nil=不改；提供则校验非空 |
| `description` | *string | nil=不改；`&""`=清空 |
| `priority` | *any | nil=不改；提供则按 §3 归一化 |
| `tags` | *[]string | nil=不改；`&[]`=清空；提供则去重去空保序 |
| `assignee` | *string | nil=不改；`&""`=清空 |
| `depends_on` | *[]string | nil=不改；`&[]`=清空 |
| `parent_id` | **string | **三重态**：nil=不改；`&nil`=置为顶层；`&str`=改为该父任务（存在且同项目校验 + 父链环校验 `PARENT_CYCLE`） |

- **内部字段保护**：`id / project_id / status / archived_from / source_file / source_section / created_at / updated_at` 不接受 Update 修改（UpdateInput 不含这些字段，JSON 传入被忽略）。
- `updated_at` 在任意字段实际变更时刷新。

## 5. 状态更新（ChangeStatus）语义

- 独立接口，输入：任务 ID + 目标状态 key。
- 校验：任务存在（`TASK_NOT_FOUND`）；目标状态存在于状态机 states 且 ≠ `archived`（`STATUS_NOT_FOUND`）。
- **transitions 流转校验属 TF-006**（非法流转 → `INVALID_TRANSITION`）；TF-005 先落地"存在性校验 + 更新"，TF-006 在同一接口内追加流转校验，接口签名不变。
- `archived` 只能由 archive/restore（TF-007）设置。

## 6. 列表（List）语义（QA Q10 / Q11 / Q12）

### 6.1 返回结构

- **不传分页参数** → 返回**全量任务树**：`TaskTreeNode{ Task 字段平铺 + children: []*TaskTreeNode }`，每层内部排序。
- **显式传 `page`/`size`** → 返回**扁平分页**：全部匹配任务全局排序后分页，响应 `{items, total, page, size}`。`page` 从 1 起，`size` 默认 100、上限 500（超出按 500 截断）。

### 6.2 过滤 / 搜索（Q11-A）

- `filter[status]`：单值过滤。
- `q`：匹配 `title` 或 `description`（大小写不敏感包含）。
- **树形模式**采用"**祖先保留、后代过滤**"：父任务即使自身不匹配 filter/q，也作为容器保留在树中（其 `children` 仅含匹配者）。
- **默认排除 `archived`**：List 不带 `filter[status]=archived` 时排除归档任务；显式传 `archived` 则只返回归档任务。
- 分页模式（扁平）不做祖先补齐。

### 6.3 排序（Q12-A）

- 全局规则：`priority DESC, created_at ASC`（同值再按 `id ASC` 保证稳定）。
- 树形模式：**每层内部**按该规则排序；分页模式：全局按该规则排序后切片。

## 7. 详情（Get）语义（Q13-A）

- 返回 Task 本体，**不含 children**（任务树由 List 提供）。

## 8. 错误码清单

| 错误码 | 语义 |
|--------|------|
| `PROJECT_NOT_FOUND` | workdir 无 `.taskboard/meta.db`（未导入为项目） |
| `TASK_NOT_FOUND` | 任务不存在（含跨项目查询——不同项目库本就查不到） |
| `TASK_INVALID` | 参数非法：title 空 / priority 非法 / 其他入参错误 |
| `PARENT_NOT_FOUND` | parent_id 指向的任务不存在或不属于该项目 |
| `PARENT_CYCLE` | parent_id 变更引入父链环（A 的父为自身后代） |
| `STATUS_NOT_FOUND` | 目标状态不在项目状态机 states（或为 archived 保留态） |

> 错误定义于 `internal/task/errors.go`（哨兵错误 + Code() 映射）；HTTP 状态码映射在 TF-013 落地。
> TF-006 追加 `INVALID_TRANSITION` / `STATUS_IN_USE`；TF-007 追加归档相关语义；TF-008 追加 `DEPENDENCY_NOT_FOUND` / `CIRCULAR_DEPENDENCY`。

## 9. 并发与钩子

- **并发写**：最后写入生效，不加乐观锁（REQUIREMENTS-REVIEW Q27-A），靠审计追溯（审计 TF-012 落地）。
- **写钩子（Q14-A）**：Service 构造时可选注入 `OnWrite(ctx, action, target)` 回调，写操作成功后调用（当前为 nil 安全）；TF-012 异步审计、TF-014 WS 事件经此钩子接入，**不改 Service 签名**。
- 连接管理（Q1-A）：Service 内部按 workdir 打开并缓存项目库连接（map + mutex，`SetMaxOpenConns(1)`），方法签名携带 workdir；不依赖全局注册表连接（Q2-B）。

## 10. 与后续任务的边界

| 任务 | 本文件涉及的边界 |
|------|------------------|
| TF-006 状态机校验 | ChangeStatus 追加 transitions 校验；状态机编辑校验 `STATUS_IN_USE` |
| TF-007 归档/还原 | archive/restore 接口、级联置空 parent_id、物理删除规则 |
| TF-008 依赖校验 | depends_on 存在性（`DEPENDENCY_NOT_FOUND`）与无环校验（`CIRCULAR_DEPENDENCY`） |
| TF-009 覆盖率收口 | 本文件 §1–§9 语义全部纳入测试断言 |
| TF-012 审计 / TF-014 WS | 经 §9 写钩子接入 |

*（文档完）*
