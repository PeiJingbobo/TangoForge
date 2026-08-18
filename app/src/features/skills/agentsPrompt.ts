/**
 * 「放入 AGENTS.md 的推荐提示词」模板（中/英）。
 *
 * 唯一事实来源：SkillsPanel（项目 Skills 配置页）与 SkillStep（项目导入引导）
 * 必须共用本文件，禁止在各自组件内复制模板副本（两端渲染与复制行为完全一致）。
 *
 * 占位符（由调用方替换）：
 *   {{ daemon_port }}        守护进程端口（默认 19810）
 *   {{ project_dir }}        项目工作目录绝对路径
 *   {{ skills_list }}        已安装技能包名列表
 *   {{ state_machine_list }} 项目状态机 key(label) 列表
 *
 * 内容与根 AGENTS.md §5.3 同源：知识库使用说明 + 任务状态同步硬约束，
 * 两处（UI 提示词 / AGENTS.md）修改时必须保持同步。
 */

export const AGENTS_PROMPT_ZH = `## TangoForge 任务管理

TangoForge 是本项目任务数据的事实来源。它是本地守护进程，默认端口为 \`{{ daemon_port }}\`。

当需要创建、查询、更新、流转、归档、还原、导入导出项目任务，或检索、关联任务资料（知识库）时：

1. 先读取已安装的 \`{{ skills_list }}\` 技能，例如 \`.claude/skills/taskboard-basic/SKILL.md\`。
2. 优先使用 TangoForge MCP 工具。若当前宿主已暴露 \`tangoforge\` MCP 工具，直接调用 \`guide\`、\`project_list\`、\`task_list\`、\`task_read\`、\`state_machine_get\`、\`task_update\`、\`knowledge_search\`、\`knowledge_link\` 等工具；所有项目级 MCP 调用必须显式传入：

   \`\`\`text
   project={{ project_dir }}
   \`\`\`

3. 若宿主未直接暴露 MCP 工具，但本机可执行 \`tangoforge\`，优先尝试 stdio MCP：启动 \`tangoforge mcp\`，完成 MCP \`initialize\` 后调用 \`tools/list\` 确认存在 \`task_list\`、\`task_read\`、\`state_machine_get\`、\`knowledge_search\` 等工具，再用 \`tools/call\` 传入 \`project\` 读取或更新任务。实测 \`tangoforge mcp\` 会返回 \`TangoForge 1.0.0\` 和完整工具清单。
4. MCP 不可用时使用 CLI。CLI 调用必须使用 \`--project {{ project_dir }}\`，优先加 \`--json\` 便于解析；例如：

   \`\`\`bash
   tangoforge --json tasks list --project {{ project_dir }}
   tangoforge --json state-machine get --project {{ project_dir }}
   tangoforge --json knowledge search --project {{ project_dir }} --q "关键词"
   tangoforge --json knowledge link --project {{ project_dir }} --task_id <任务ID> --path <相对路径>
   tangoforge --json knowledge scan --project {{ project_dir }}
   \`\`\`

5. 只有 MCP 和 CLI 都不可用或需要诊断守护进程时，才使用 HTTP 作为最后兜底。读取最新指南：

   \`\`\`bash
   curl -H 'X-Project: {{ project_dir }}' \\
     http://127.0.0.1:{{ daemon_port }}/api/guide
   \`\`\`

6. 如果普通命令访问 \`127.0.0.1:{{ daemon_port }}\` 失败，但用户浏览器或提权/非沙箱环境确认该地址可打开，优先判断为 当前Agent 执行环境的 localhost 或沙箱网络隔离问题；不要据此认定 TangoForge 不可用。CLI 自动拉起守护进程超时也不能单独证明 TangoForge 不可用；需先按优先级复试 MCP、CLI，并在必要时用带 \`X-Project\` 的 HTTP 指南端点诊断。
7. HTTP 请求必须带：

   \`\`\`text
   X-Project: {{ project_dir }}
   \`\`\`

### 知识库（Knowledge Base）使用

知识库 = 文档引用注册表 + 任务关联 + 语义索引：任务相关资料（Markdown / 代码 / 文本）注册到知识库后可关联到任务，并支持语义检索。相关工具/命令（均需携带 project）：

- **检索**：\`knowledge_search\`（MCP）/ \`tangoforge knowledge search --q "关键词"\`（CLI）→ 命中文档与片段（需 \`knowledge.read\`，默认只读）。
- **关联任务资料**：\`knowledge_link\`（MCP）/ \`tangoforge knowledge link --task_id <ID> --path <路径>\`（CLI）；解除用 \`knowledge_unlink\`，重关联用 \`knowledge_relink\`（需 \`knowledge.write\` 授权）。
- **浏览与维护**：\`knowledge_list\` / \`knowledge_read\` / \`knowledge_edit\` / \`knowledge_scan\`（手动扫描；文件变化也会自动重索引）。
- 任务详情「资料」区展示已关联文档；向量检索依赖 embedding 配置，未配置时文档仍可注册/摘要，检索自动禁用。
- 归档的知识库文档从默认列表/检索隐藏，任务引用与文件保留。

任务操作规则：

- 不直接读写 \`.taskboard/meta.db\`、WAL 或 SHM 文件。
- 不臆造任务 ID、状态、优先级、依赖或完成情况。
- 流转前查询项目状态机；只执行合法流转。
- 当前项目状态机为：{{ state_machine_list }}; 建议需要修改状态时,使用工具查询最新状态机列表保证实施性
- **状态同步铁律**：只要本次工作对应 TangoForge 中的任务，任务状态追踪就是交付物的一部分，与产出代码同等重要，**不得遗漏**。状态名一律以**项目状态机实际定义**为准（流转前用 \`state_machine_get\` 查询，不得使用状态机中不存在的状态）：
  - 开工前：必须先用工具查询并定位对应任务；确认合法后**立即**流转到当前执行阶段（即状态机中表示"进行中"的状态），而不是等交付时一次性补状态。
  - 过程中：每完成一个阶段或关键切片，**立即**按真实进度流转状态或更新任务说明；任务板必须始终反映实时状态，禁止跨阶段静默。
  - 交付前：宣告完成**之前**必须复核任务板状态与实际工作一致；任何不一致都必须先补做合法流转。完成判定以项目状态机为准：若状态机提供"待核验/审查中"类状态，需要人工审查的工作必须先流转到该状态，审查通过后再流转到完成状态；项目无审查环节或工作无需验证时，可直接流转到完成状态。**不得跳过审查直接把需核验的工作标为完成。**
- 每次状态流转都应附带可追踪的进展说明，例如已完成内容、正在验证的范围、阻塞原因或返工原因；说明必须基于实际操作，不得用笼统文案代替真实状态。
- 长时间任务应在完成关键切片后主动更新 TangoForge 状态或说明，让任务板展示各任务状态的实时阶段。
- 删除任务使用归档语义，除非指南明确提供其他受支持操作。
- 只有在工作完成并通过验证后，才能把对应任务标记为完成。
- TangoForge 不可用时，报告不可用事实和已按 MCP、CLI、HTTP 顺序尝试过的入口；若涉及 \`127.0.0.1:{{ daemon_port }}\` 连接失败，必须说明是否已在非沙箱/提权环境用带 \`X-Project\` 的 HTTP 指南端点复试。不要退回到自建 JSON、Markdown 清单或虚构任务数据，也不要声称任务状态已经更新。下一次继续相关工作时应重新尝试连接并补做合法流转。
- 用户明确要求产出需求文档、计划文档或验收文档时，可以正常创建文档，但不得把文档内容冒充 TangoForge 中的实时任务状态。

不要因为普通编码请求而擅自创建任务；只有用户要求任务管理，或请求明确引用 TangoForge 中已有任务时才操作任务数据。

`

export const AGENTS_PROMPT_EN = `## TangoForge Task Management

TangoForge is the source of truth for project task data. It is a local daemon that listens on port \`{{ daemon_port }}\` by default.

When you need to create, query, update, transition, archive, restore, import, or export project tasks, or search and link task materials (knowledge base):

1. First, read the installed \`{{ skills_list }}\` skill(s), for example \`.claude/skills/taskboard-basic/SKILL.md\`.
2. Prefer using the TangoForge MCP tools. If the host environment already exposes the \`tangoforge\` MCP tools, call \`guide\`, \`project_list\`, \`task_list\`, \`task_read\`, \`state_machine_get\`, \`task_update\`, \`knowledge_search\`, \`knowledge_link\`, and similar tools directly. Every project-scoped MCP call must explicitly include:

   \`\`\`text
   project={{ project_dir }}
   \`\`\`

3. If the host does not directly expose MCP tools but the \`tangoforge\` executable is available locally, fall back to stdio MCP: launch \`tangoforge mcp\`, complete the MCP \`initialize\` handshake, call \`tools/list\` to confirm the presence of \`task_list\`, \`task_read\`, \`state_machine_get\`, \`knowledge_search\`, and other expected tools, then use \`tools/call\` with the \`project\` parameter to read or update tasks. In practice, \`tangoforge mcp\` returns \`TangoForge 1.0.0\` along with the full tool list.
4. If MCP is unavailable, use the CLI. CLI invocations must include \`--project {{ project_dir }}\` and should prefer the \`--json\` flag for structured output. Examples:

   \`\`\`bash
   tangoforge --json tasks list --project {{ project_dir }}
   tangoforge --json state-machine get --project {{ project_dir }}
   tangoforge --json knowledge search --project {{ project_dir }} --q "keywords"
   tangoforge --json knowledge link --project {{ project_dir }} --task_id <task-id> --path <relative/path>
   tangoforge --json knowledge scan --project {{ project_dir }}
   \`\`\`

5. Only use HTTP as a last resort when neither MCP nor CLI is accessible, or when you need to diagnose the daemon. To retrieve the latest guide:

   \`\`\`bash
   curl -H 'X-Project: {{ project_dir }}' http://127.0.0.1:{{ daemon_port }}/api/guide
   \`\`\`

6. If accessing \`127.0.0.1:{{ daemon_port }}\` from a typical command fails, but the user's browser or a privileged/non-sandboxed environment confirms the endpoint is reachable, treat this as a localhost or sandbox network isolation issue specific to the current agent's execution environment; do not conclude that TangoForge is unavailable. A timeout when the CLI attempts to auto-start the daemon also does not by itself prove unavailability. You must reattempt MCP and CLI in the priority order described above, and when necessary use the HTTP guide endpoint with the \`X-Project\` header for diagnostics.
7. All HTTP requests must carry the header:

   \`\`\`text
   X-Project: {{ project_dir }}
   \`\`\`

### Knowledge Base Usage

The knowledge base is a document reference registry plus task linking plus semantic indexing: project materials (Markdown / code / text) are registered into the knowledge base, linked to tasks, and searchable semantically. All tools/commands require the \`project\` parameter:

- **Search**: \`knowledge_search\` (MCP) / \`tangoforge knowledge search --q "keywords"\` (CLI) → matching documents and snippets (requires \`knowledge.read\`, read-only by default).
- **Link task materials**: \`knowledge_link\` (MCP) / \`tangoforge knowledge link --task_id <id> --path <path>\` (CLI); unlink with \`knowledge_unlink\`, relink with \`knowledge_relink\` (requires \`knowledge.write\` authorization).
- **Browse and maintain**: \`knowledge_list\` / \`knowledge_read\` / \`knowledge_edit\` / \`knowledge_scan\` (manual scan; file changes are also re-indexed automatically).
- The "Materials" section in task details shows linked documents; vector search depends on the embedding configuration. Without it, documents can still be registered/summarized, but search is disabled.
- Archived knowledge documents are hidden from default lists and search; task links and files are preserved.

Task operation rules:

- Never directly read or write \`.taskboard/meta.db\`, WAL, or SHM files.
- Do not fabricate task IDs, statuses, priorities, dependencies, or completion claims.
- Before transitioning a task, query the project's state machine; only perform valid transitions.
- The current project state machine is: {{ state_machine_list }}. When you need to modify status, query the latest state machine using the tools to ensure accuracy.
- **Status synchronization is a hard requirement**: whenever the current work corresponds to a task in TangoForge, tracking the task status is part of the deliverable, as important as the code itself, and must not be skipped. Always use the statuses **actually defined by the project state machine** (query it with \`state_machine_get\` before any transition; never use statuses that do not exist in it):
  - Before starting: look up and locate the corresponding task with the tools; once confirmed and legal, **immediately** advance it to the current execution stage (the in-progress status defined by the state machine) — do not batch status updates only at delivery time.
  - During work: after each stage or key milestone, **immediately** transition the status or update the task note to reflect real progress; the board must always show the live status — no silent gaps between stages.
  - Before delivery: **before** declaring completion, re-check that the board status matches the actual work; any mismatch must be fixed with legal transitions first. Completion criteria follow the project state machine: if it provides a "needs review / under review" style status, work requiring human review must first transition to that status and only to a completion status after the review passes; if the project has no review stage or the work needs no verification, it may transition directly to a completion status. **Never mark work that requires review as completed without the review.**
- Every state transition must include a traceable progress note describing, for example, what has been completed, the scope under verification, blocking reasons, or rework causes. The note must be grounded in actual operations; do not substitute generic text for genuine status.
- For long-running tasks, proactively update the TangoForge status or description after completing key milestones so the board shows the real-time stage of each task.
- Use archive semantics when deleting tasks, unless the guide explicitly provides another supported operation.
- Mark a task with a completion status only after the work is finished and verified.
- If TangoForge is unavailable, report that fact along with the attempts made in MCP → CLI → HTTP order. If the failure involves connecting to \`127.0.0.1:{{ daemon_port }}\`, explicitly state whether you have retried using the HTTP guide endpoint with the \`X-Project\` header in a non-sandboxed or privileged environment. Do not fall back to self-created JSON, Markdown lists, or fabricated task data, and do not claim that task statuses have been updated. When you resume related work later, re-attempt the connection and perform any pending legal transitions.
- When the user explicitly asks you to produce requirements documents, planning documents, or acceptance documents, you may create them normally, but you must not present their content as the live task state inside TangoForge.

Do not create tasks autonomously for ordinary coding requests. Only manipulate task data when the user asks for task management, or when a request explicitly references existing tasks within TangoForge.

`
