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
- 校验链（TF-005 存在性 + TF-006 流转校验）：
  1. 任务存在（`TASK_NOT_FOUND`）；
  2. **同态流转（目标 = 当前）→ 幂等成功**：直接返回，不校验、不刷新 `updated_at`（Q2-A）；
  3. 目标状态存在于状态机 states 且 ≠ `archived`（`STATUS_NOT_FOUND`）；
  4. **流转校验**（§5.1）：非法 → `INVALID_TRANSITION`。
- `archived` 只能由 archive/restore（TF-007）设置。

### 5.1 状态机流转校验（TF-006）

- 规则（QA Q1-B 宽松 + Q3-A 特例）：
  - **transitions 整体为空**（states 自定义但无任何规则）→ **拒绝所有普通流转**（`INVALID_TRANSITION`，安全默认）；
  - **transitions 非空**：目标状态在 `from` 规则的 `to` 列表中 → 放行；在 `from` 规则中但目标不在 `to` → `INVALID_TRANSITION`；
  - **from 未定义规则** → 放行任意流转（宽松，Q1-B）。
- 同态流转幂等（Q2-A）见 §5；`archived` 不参与任何普通流转。
- 默认状态机（config.yaml 缺失/state_machine 节缺失回退）：`todo→[doing,done]`、`doing→[todo,done]`、`done→[doing,todo]`（回退允许）。

### 5.2 状态机编辑（GetStateMachine / UpdateStateMachine，TF-006）

- `GetStateMachine(ctx, workdir)`：读取当前定义（缺失回退默认四态）。
- `UpdateStateMachine(ctx, workdir, sm)`：编辑校验 → 占用校验 → 持久化 `config.yaml`（**替换 state_machine 节，保留 export 等其它配置节**，Q8-A）。
- **编辑校验规则**（Q5-A）：
  - `states` 至少 1 个；key 必填、去空白、**唯一**（重复 → 参数非法）；
  - key **不得为 `archived`**（系统保留态）；
  - `transitions` 的 `from` / 每个 `to` 必须存在于 `states`（否则参数非法）；`to` 去重；`to` 可为空（该状态不可流转出去）。
- **STATUS_IN_USE（Q7-A 口径）**：项目库中 `status = key` 的任务数（archived 任务不参与统计）；**占用状态不可删除、不可重命名 key**（`STATUS_IN_USE`，错误 Message 携带占用数）；**占用状态允许修改 label/color/transitions**（key 不变）。
- 写入成功后触发写钩子 `state_machine.changed`（TF-014 WS 事件接入点）。

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

## 8. 归档 / 还原 / 物理删除语义（TF-007）

### 8.1 归档（Archive）

- 签名：`Archive(ctx, workdir, id) (ArchiveResult, error)`；`ArchiveResult{ Task, DependentCount, ChildrenCleared }`。
- 行为：`status → archived`，记录 `archived_from = 归档前状态`，刷新 `updated_at`；**直接子任务（`parent_id = id`）级联置空为顶层**（`ChildrenCleared` 返回数量）；**全部在事务内原子完成**（QA Q3：任一步失败即整体回滚，无半状态）。
- **幂等（Q2-B）**：已归档任务再次归档 → 直接返回当前状态（不重复记录、不触发钩子）。
- **DependentCount（Q1-A）**：未归档任务中 `depends_on` 包含该任务 ID 的个数——**不阻断归档**，仅提示（调用方/UI 决定是否继续）。
- 钩子：`task.archived`（级联置空为副作用，不单独触发）。
- **UI 确认流（Q3）**：App UI 归档前若检测到存在子任务（可依据 `ChildrenCleared` 或列表查询），先弹确认框由用户选择归档方式（确认级联置空 / 取消）。

### 8.2 还原（Restore）

- 签名：`Restore(ctx, workdir, id, opts RestoreOptions) (Task, error)`；`RestoreOptions{ FallbackTodo bool }`。
- 校验：仅 `archived` 任务可还原（非归档 → `TASK_INVALID`）；任务不存在 → `TASK_NOT_FOUND`。
- 目标状态：`archived_from` 记录的状态；**`archived_from` 为空（异常数据）→ 回退 `todo`**（Q6-A）。
- **目标状态已从状态机删除（Q5）**：默认拒绝 `STATUS_NOT_FOUND`；`FallbackTodo: true` 时回退 `todo`。
- 还原后**清空 `archived_from`**，刷新 `updated_at`；钩子：`task.restored`。
- **UI 确认流（Q5）**：App UI 还原前检查 `archived_from` 状态是否存在（或捕获 `STATUS_NOT_FOUND`），由用户选择：取消（完善状态机后重试）或 回退 todo（以 `FallbackTodo: true` 重调）。

### 8.3 物理删除（Delete）

- 签名：`Delete(ctx, workdir, id) (Task, error)`；返回**被删任务快照**（Q9-A）。
- 校验：**仅 `archived`（回收站）任务可物理删除**；非归档 → `DELETE_NOT_ALLOWED`（新错误码）。
- **级联规则（Q8-A）**：物理删除父任务时，子任务**不可一并物理删除**（无级联删除能力），仅**级联置空 `parent_id`** 保留为顶层；回收站中子任务自身仍可单独物理删除；删除与置空在事务内原子完成。
- 钩子：`task.deleted`。
- **UI 确认流（Q8）**：App UI 物理删除前检测子任务，由用户选择处置方式（确认级联置空 / 取消）。

## 9. 依赖关系（depends_on）语义（TF-008）

- **方向语义（Q1-A）**：`A.depends_on = [B]` 表示 **A 依赖 B**（B 是 A 的前置 / 被依赖者）；**环 = 沿 depends_on 图走回自身**（含自依赖 `X.depends_on` 含 X）。
- **校验位置（Q4-A）**：Create / Update 在**写入前校验**（写操作本身为单语句原子，校验失败直接返回，不产生任何脏数据——语义等价"事务内完成"）。
- **校验规则**：
  - **依赖任务必须存在**（`DEPENDENCY_NOT_FOUND`，Q2-A）——拒绝不存在的依赖 ID；
  - **依赖已归档（archived）任务允许**（Q3-A：归档不阻断依赖关系，与 TF-007 归档"被依赖仅提示不阻断"闭环一致）；
  - **自依赖**（`X.depends_on` 含 X）→ `CIRCULAR_DEPENDENCY`（Q7-A）；
  - **多跳环**（Q5-A）：基于"更新后的 depends_on 集合 + 其余任务现有集合"沿依赖图多跳 DFS，回到自身 → `CIRCULAR_DEPENDENCY`。
- Update 的 `DependsOn=nil`（不更新）与 `&[]`（清空）沿用 §4.2 指针语义；清空不触发环校验（空集无环）。

## 10. 错误码清单

| 错误码 | 语义 |
|--------|------|
| `PROJECT_NOT_FOUND` | workdir 无 `.taskboard/meta.db`（未导入为项目） |
| `TASK_NOT_FOUND` | 任务不存在（含跨项目查询——不同项目库本就查不到） |
| `TASK_INVALID` | 参数非法：title 空 / priority 非法 / 非归档任务还原 / 其他入参错误 |
| `PARENT_NOT_FOUND` | parent_id 指向的任务不存在或不属于该项目 |
| `PARENT_CYCLE` | parent_id 变更引入父链环（A 的父为自身后代） |
| `STATUS_NOT_FOUND` | 目标状态不在项目状态机 states（或为 archived 保留态 / 还原目标状态已删除） |
| `INVALID_TRANSITION` | 非法状态流转（transitions 为空拒绝一切 / 目标不在 from 规则的 to 列表） |
| `STATUS_IN_USE` | 状态被任务占用，不可删除/重命名（Message 携带占用任务数） |
| `DELETE_NOT_ALLOWED` | 物理删除仅限回收站（archived）任务，非归档任务拒绝 |
| `DEPENDENCY_NOT_FOUND` | depends_on 引用不存在的任务 |
| `CIRCULAR_DEPENDENCY` | depends_on 引入循环依赖（含自依赖） |

> 错误定义于 `internal/task/errors.go`（哨兵错误 + Code() 映射）；HTTP 状态码映射在 TF-013 落地。
> TF-006 已追加 `INVALID_TRANSITION` / `STATUS_IN_USE`；TF-007 已追加 `DELETE_NOT_ALLOWED`；TF-008 已追加 `DEPENDENCY_NOT_FOUND` / `CIRCULAR_DEPENDENCY`。

## 11. 并发与钩子

- **并发写**：最后写入生效，不加乐观锁（REQUIREMENTS-REVIEW Q27-A），靠审计追溯（审计 TF-012 落地）。
- **写钩子（Q14-A）**：Service 构造时可选注入 `OnWrite(ctx, action, target)` 回调，写操作成功后调用（当前为 nil 安全）；TF-012 异步审计、TF-014 WS 事件经此钩子接入，**不改 Service 签名**。动作：`task.created / task.updated / task.status_changed / task.archived / task.restored / task.deleted / state_machine.changed`。
- 连接管理（Q1-A）：Service 内部按 workdir 打开并缓存项目库连接（map + mutex，`SetMaxOpenConns(1)`），方法签名携带 workdir；不依赖全局注册表连接（Q2-B）。

## 12. 与后续任务的边界

| 任务 | 本文件涉及的边界 |
|------|------------------|
| ~~TF-006 状态机校验~~ | ✅ 已完成：§5.1 流转校验 + §5.2 状态机编辑（含 `INVALID_TRANSITION` / `STATUS_IN_USE`） |
| ~~TF-007 归档/还原/物理删除~~ | ✅ 已完成：§8 删除语义（幂等归档、级联置空、RestoreOptions、`DELETE_NOT_ALLOWED`） |
| ~~TF-008 依赖校验~~ | ✅ 已完成：§9 依赖语义（存在性、自依赖、多跳环，`DEPENDENCY_NOT_FOUND` / `CIRCULAR_DEPENDENCY`） |
| ~~TF-009 覆盖率收口~~ | ✅ 已完成：P2 结束时 `internal/task` 覆盖率 ≥ 90%（check_coverage.sh 强制） |
| TF-012 审计 / TF-014 WS | 经 §11 写钩子接入 |

*（文档完）*
