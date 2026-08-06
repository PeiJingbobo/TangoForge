# tangoforge-usage

> TangoForge 人机协作任务看板 — Agent 接入与操作指南（项目级 Skill）。
> 版本：1.0.0　|　适用：本项目的任务池读写、导入导出、状态机与权限管理。

## 1. 接入方式

你有两条通道操作本项目任务：

- **MCP 工具**（推荐）：已配置 stdio（命令 `tangoforge mcp`）或远程 `http://127.0.0.1:19810/mcp`。
  `tools/list` 返回 19 个固定工具：`project_*` / `task_*` / `import_*` / `export_markdown` /
  `graph_get` / `state_machine_*` / `skill_info` / `permission_list`。
- **CLI**（等价 HTTP 调用）：`tangoforge <子命令> --project <工作目录>`（如 `tangoforge tasks list --project ...`）。

**铁律：除 `project_list` 外，每个工具的第一个参数都是 `project`（本项目工作目录绝对路径），必填。**

## 2. 权限模型（重要）

- 你的身份是 **agent**（actor = 客户端名），对本项目**默认只有只读权限**。
- 写操作（task_create / task_update / task_archive / import_* / export_markdown / state_machine_write 等）
  未授权时返回 `PERMISSION_DENIED`——不要重试，先查询 `permission_list` 确认可用范围，需要写操作时请用户在 UI/App 中为 Agent 勾选权限（授权后全通道即时生效）。
- 权限修改仅 UI 侧可执行。

## 3. 快速上手（按需组合）

### 3.1 了解项目
- `permission_list`（project 必填）→ 查看你可执行的 action 范围。
- `state_machine_get` → 查看本项目状态机（**所有 status 参数必须使用状态机 key**，如 todo/doing/done；也可用 label）。
- `graph_get` → 全景图（nodes=未归档任务，edges=父子/依赖边，服务端不聚簇）。

### 3.2 读取任务
- `task_list`（project 必填；可选 status / q 过滤）→ 树形列表（顶层 + 子任务）。
- `task_read`（project + id）→ 单个任务详情。

### 3.3 创建与更新
- `task_create`：title **必填**；priority 支持 0-5 整数或别名（lowest/low/normal/high/highest/critical）；tags 数组；assignee；depends_on 为被依赖任务 ID 数组（A 依赖 B → `depends_on: [B_ID]`）；parent_id 指定父任务。
- `task_update`：部分更新字段（**不含 status**）；`parent_id` 传空串 = 置为顶层。
- 状态流转使用 `task_update` 之外的状态机接口或 CLI `tasks status <id> <key>`（走 transitions 校验，非法流转返回 `INVALID_TRANSITION`）。

### 3.4 归档 / 还原
- `task_archive`（软删除，子任务级联置空 parent_id）；`task_restore`（fallback_todo 可选：原状态已从状态机删除时回退 todo）。

### 3.5 批量导入 Markdown（LLM 解析 → 草稿 → 确认）
1. `import_preview`：四形态输入（任选其一）
   - `file_path`（单文件）；`file_paths`（多文件数组，合并一次解析）；`directory`（递归扫描 `*.md/*.markdown`）；`content`+`source_file`。
   - 成功返回草稿（含 id、source_file、task_count），**不会直接入库**。
2. `import_drafts` → 查看 pending 草稿。
3. `import_confirm`（draft_id）→ 文件级全量覆盖入库（归档该 source_file 旧任务 + 批量重建，事务原子）。
   `import_discard` → 丢弃草稿。
- 解析语义：LLM 按标题层级生成嵌套任务；status 映射到状态机 key/label；缺 title/status 整次失败（不落库）。

### 3.6 导出
- `export_markdown`：template_mode `default`（内置或项目自定义模板）/ `llm`（须先生成模板）；target `overwrite`（path 必填）/ `copy`（缺省写 `{project}/.taskboard/export.md`）；返回 content + path。

### 3.7 项目引导（新目录）
- `project_create`（新目录，先初始化再注册，一条龙）；`project_init`（仅初始化元数据）；`project_import`（仅注册，要求已初始化）。

## 4. 常见错误与应对

| 错误码 | 含义 | 应对 |
|--------|------|------|
| `PERMISSION_DENIED` | 写操作未授权 | 查 `permission_list`；请用户 UI 授权后重试 |
| `PROJECT_NOT_FOUND` | 目录未初始化/未注册 | 用 `project_create` 引导 |
| `TASK_INVALID` | 参数缺失/非法 | 检查必填参数（title / project / id） |
| `INVALID_TRANSITION` | 状态流转非法 | 查 `state_machine_get` 的 transitions |
| `TASK_NOT_FOUND` / `DRAFT_NOT_FOUND` | 目标不存在 | 核实 ID |
| `IMPORT_FAILED` | 解析失败（LLM 输出不合规） | 检查文档结构；错误含 LLM 原始输出 |
| `LLM_TRUNCATED` | max_tokens 截断 | 请调大 llm.max_tokens（≥8192）或改用非推理模型 |

## 5. 约定

- 任务 ID 为 UUID；父子关系经 `parent_id`；依赖方向：`A.depends_on=[B]` 表示 **A 依赖 B**（B 完成后 A 才能开始）。
- 状态机可自定义（`state_machine_update`，全量覆盖；有任务占用的状态不可删）。
- 审计：所有写操作落 audit_log（含 actor）；`audit.read` 授权后可通过 CLI/HTTP 查询。
- 变更后端数据后，如需要保持 UI 实时同步，写操作请优先经 daemon（远程 MCP / HTTP）；stdio 直连进程不推送 WS 事件。
