import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { Check, CheckCircle2, Copy, ExternalLink, Loader2, Wrench } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useConfig } from '@/hooks/useConfig'
import { useStateMachine } from '@/hooks/useStateMachine'
import { useSkillPackages, useSkillInstall, useSkillStatus } from '@/hooks/useSkills'
import { SKILL_HOSTS, DEFAULT_SKILL_HOST } from '@/features/skills/hosts'

const AGENTS_PROMPT_ZH = `## TangoForge 任务管理

TangoForge 是本项目任务数据的事实来源。它是本地守护进程，默认端口为 \`{{ daemon_port }}\`。

当需要创建、查询、更新、流转、归档、还原或导入导出项目任务时：

1. 先读取已安装的 \`{{ skills_list }}\` 技能，例如 \`.claude/skills/taskboard-basic/SKILL.md\`。
2. 优先使用 TangoForge MCP 工具。若当前宿主已暴露 \`tangoforge\` MCP 工具，直接调用 \`guide\`、\`project_list\`、\`task_list\`、\`task_read\`、\`state_machine_get\`、\`task_update\` 等工具；所有项目级 MCP 调用必须显式传入：

   \`\`\`text
   project={{ project_dir }}
   \`\`\`

3. 若宿主未直接暴露 MCP 工具，但本机可执行 \`tangoforge\`，优先尝试 stdio MCP：启动 \`tangoforge mcp\`，完成 MCP \`initialize\` 后调用 \`tools/list\` 确认存在 \`task_list\`、\`task_read\`、\`state_machine_get\` 等工具，再用 \`tools/call\` 传入 \`project\` 读取或更新任务。实测 \`tangoforge mcp\` 会返回 \`TangoForge 1.0.0\` 和完整工具清单。
4. MCP 不可用时使用 CLI。CLI 调用必须使用 \`--project {{ project_dir }}\`，优先加 \`--json\` 便于解析；例如：

   \`\`\`bash
   tangoforge --json tasks list --project {{ project_dir }}
   tangoforge --json state-machine get --project {{ project_dir }}
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

任务操作规则：

- 不直接读写 \`.taskboard/meta.db\`、WAL 或 SHM 文件。
- 不臆造任务 ID、状态、优先级、依赖或完成情况。
- 流转前查询项目状态机；只执行合法流转。
- 当前项目状态机为：{{ state_machine_list }}; 建议需要修改状态时,使用工具查询最新状态机列表保证实施性
- 凡是用户请求对应 TangoForge 中的具体任务或阶段任务，开始实施前必须先查询并定位任务；确认合法后立即流转到当前执行阶段，而不是等交付时一次性补状态。
- 任务执行过程中按真实阶段及时流转，使任务板能反映实时状态：
- 每次状态流转都应附带可追踪的进展说明，例如已完成内容、正在验证的范围、阻塞原因或返工原因；说明必须基于实际操作，不得用笼统文案代替真实状态。
- 长时间任务应在完成关键切片后主动更新 TangoForge 状态或说明，让任务板展示各任务状态的实时阶段。

- 删除任务使用归档语义，除非指南明确提供其他受支持操作。
- 只有在工作完成并通过验证后，才能把对应任务标记为完成。
- TangoForge 不可用时，报告不可用事实和已按 MCP、CLI、HTTP 顺序尝试过的入口；若涉及 \`127.0.0.1:{{ daemon_port }}\` 连接失败，必须说明是否已在非沙箱/提权环境用带 \`X-Project\` 的 HTTP 指南端点复试。不要退回到自建 JSON、Markdown 清单或虚构任务数据，也不要声称任务状态已经更新。下一次继续相关工作时应重新尝试连接并补做合法流转。
- 用户明确要求产出需求文档、计划文档或验收文档时，可以正常创建文档，但不得把文档内容冒充 TangoForge 中的实时任务状态。

不要因为普通编码请求而擅自创建任务；只有用户要求任务管理，或请求明确引用 TangoForge 中已有任务时才操作任务数据。

`

const AGENTS_PROMPT_EN = `## TangoForge Task Management

TangoForge is the source of truth for project task data. It is a local daemon that listens on port \`{{ daemon_port }}\` by default.

When you need to create, query, update, transition, archive, restore, import, or export project tasks:

1. First, read the installed \`{{ skills_list }}\` skill(s), for example \`.claude/skills/taskboard-basic/SKILL.md\`.
2. Prefer using the TangoForge MCP tools. If the host environment already exposes the \`tangoforge\` MCP tools, call \`guide\`, \`project_list\`, \`task_list\`, \`task_read\`, \`state_machine_get\`, \`task_update\`, and similar tools directly. Every project-scoped MCP call must explicitly include:

   \`\`\`text
   project={{ project_dir }}
   \`\`\`

3. If the host does not directly expose MCP tools but the \`tangoforge\` executable is available locally, fall back to stdio MCP: launch \`tangoforge mcp\`, complete the MCP \`initialize\` handshake, call \`tools/list\` to confirm the presence of \`task_list\`, \`task_read\`, \`state_machine_get\`, and other expected tools, then use \`tools/call\` with the \`project\` parameter to read or update tasks. In practice, \`tangoforge mcp\` returns \`TangoForge 1.0.0\` along with the full tool list.
4. If MCP is unavailable, use the CLI. CLI invocations must include \`--project {{ project_dir }}\` and should prefer the \`--json\` flag for structured output. Examples:

   \`\`\`bash
   tangoforge --json tasks list --project {{ project_dir }}
   tangoforge --json state-machine get --project {{ project_dir }}
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

Task operation rules:

- Never directly read or write \`.taskboard/meta.db\`, WAL, or SHM files.
- Do not fabricate task IDs, statuses, priorities, dependencies, or completion claims.
- Before transitioning a task, query the project's state machine; only perform valid transitions.
- The current project state machine is: {{ state_machine_list }}. When you need to modify status, query the latest state machine using the tools to ensure accuracy.
- Whenever a user requests work on a specific task or phase tracked in TangoForge, you must first look up and locate that task. Once the task is confirmed and the transition is legal, immediately advance it to the appropriate in-progress stage—do not batch status updates only at delivery time.
- Transition tasks through their real stages promptly during execution so the board reflects the live status.
- Every state transition must include a traceable progress note describing, for example, what has been completed, the scope under verification, blocking reasons, or rework causes. The note must be grounded in actual operations; do not substitute generic text for genuine status.
- For long-running tasks, proactively update the TangoForge status or description after completing key milestones so the board shows the real-time stage of each task.
- Use archive semantics when deleting tasks, unless the guide explicitly provides another supported operation.
- Mark a task as done only after the work is finished and verified.
- If TangoForge is unavailable, report that fact along with the attempts made in MCP → CLI → HTTP order. If the failure involves connecting to \`127.0.0.1:{{ daemon_port }}\`, explicitly state whether you have retried using the HTTP guide endpoint with the \`X-Project\` header in a non-sandboxed or privileged environment. Do not fall back to self-created JSON, Markdown lists, or fabricated task data, and do not claim that task statuses have been updated. When you resume related work later, re-attempt the connection and perform any pending legal transitions.
- When the user explicitly asks you to produce requirements documents, planning documents, or acceptance documents, you may create them normally, but you must not present their content as the live task state inside TangoForge.

Do not create tasks autonomously for ordinary coding requests. Only manipulate task data when the user asks for task management, or when a request explicitly references existing tasks within TangoForge.

`

/**
 * 引导 Step 4：为工作目录安装 Skill（TF-041 简化版）。
 * 宿主选择与项目 Skills 配置页共用 SKILL_HOSTS（TF-042：全部目录型 .xxx/skills，
 * 无单文件 .md 宿主），按项目级/用户级分组展示全部目标宿主，支持**多选**批量安装；
 * 默认选中 .claude/skills。
 */
export function SkillStep({
  workdir,
  onReady,
}: {
  workdir: string
  onReady: (ok: boolean) => void
}) {
  const { data: packages, isLoading } = useSkillPackages(workdir)
  const { data: skillStatus } = useSkillStatus(workdir)
  const { data: config } = useConfig()
  const { data: stateMachine } = useStateMachine(workdir)
  const install = useSkillInstall(workdir)
  const [hosts, setHosts] = useState<Set<string>>(new Set([DEFAULT_SKILL_HOST]))
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [done, setDone] = useState(false)
  const [lang, setLang] = useState<'zh' | 'en'>('zh')
  const [copied, setCopied] = useState(false)

  const resolvedPrompt = useMemo(() => {
    const port = String(config?.port ?? 19810)
    const projectDir = workdir ?? '<项目绝对路径>'
    const installedSkills = new Set<string>()
    for (const hs of skillStatus ?? []) {
      for (const inst of hs.installed) {
        if (inst.state === 'current' || inst.state === 'stale') {
          installedSkills.add(inst.name)
        }
      }
    }
    const skillsList =
      installedSkills.size > 0 ? [...installedSkills].join(', ') : '<已安装并启用的 skill>'
    const smList =
      (stateMachine?.States ?? []).map((s) => `${s.Key}(${s.Label})`).join(', ') || '<状态机列表>'

    const vars = {
      daemon_port: port,
      project_dir: projectDir,
      skills_list: skillsList,
      state_machine_list: smList,
    }
    const replace = (tpl: string) =>
      tpl.replace(/\{\{\s*(\w+)\s*\}\}/g, (_, k: string) => vars[k as keyof typeof vars] ?? '')
    return {
      zh: replace(AGENTS_PROMPT_ZH),
      en: replace(AGENTS_PROMPT_EN),
    }
  }, [config, skillStatus, stateMachine, workdir])

  const copyPrompt = async () => {
    try {
      await navigator.clipboard.writeText(lang === 'zh' ? resolvedPrompt.zh : resolvedPrompt.en)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error('复制失败，请手动选择复制')
    }
  }

  const projectHosts = SKILL_HOSTS.filter((h) => h.scope === 'project')
  const userHosts = SKILL_HOSTS.filter((h) => h.scope === 'user')

  // 默认全选内置包（source=builtin）。
  useEffect(() => {
    if (packages && packages.length > 0) {
      setSelected(new Set(packages.filter((p) => p.source === 'builtin').map((p) => p.name)))
    }
  }, [packages])

  const toggleHost = (key: string) => {
    setHosts((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
    setDone(false)
  }

  const toggle = (name: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
    setDone(false)
  }

  /** 安装到全部选中宿主（每个宿主一次 POST，合并结果）。 */
  const doInstall = async () => {
    if (selected.size === 0 || hosts.size === 0) {
      onReady(true)
      return
    }
    let okCount = 0
    let failCount = 0
    let errMsg = ''
    for (const h of hosts) {
      try {
        const results = await install.mutateAsync({ host: h, packages: [...selected] })
        okCount += results.filter((r) => r.ok).length
        const fails = results.filter((r) => !r.ok)
        failCount += fails.length
        if (fails.length > 0 && errMsg === '') errMsg = fails[0].error ?? ''
      } catch (e) {
        failCount++
        if (errMsg === '') errMsg = e instanceof Error ? e.message : '安装失败'
      }
    }
    if (failCount > 0) {
      toast.error(`${failCount} 个包安装失败: ${errMsg}`)
    } else {
      toast.success(`已安装 ${okCount} 个技能包到 ${hosts.size} 个宿主`)
    }
    setDone(true)
    onReady(true)
  }

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" /> 加载技能包…
      </div>
    )
  }

  const HostGroup = ({ title, items }: { title: string; items: typeof SKILL_HOSTS }) => (
    <div>
      <div className="mb-1.5 text-label text-muted-foreground">{title}</div>
      <div className="flex flex-wrap gap-2">
        {items.map((h) => (
          <button
            key={h.key}
            type="button"
            onClick={() => toggleHost(h.key)}
            className={`rounded-full border px-3 py-1 text-xs font-medium transition-colors ${
              hosts.has(h.key)
                ? 'border-primary-400 bg-primary-50 text-primary-700'
                : 'border-divider text-muted-foreground hover:border-primary-300'
            }`}
          >
            {h.label}
          </button>
        ))}
      </div>
    </div>
  )

  return (
    <div className="space-y-4">
      <div className="flex items-start gap-2 rounded-xl border border-divider bg-muted/50 p-3">
        <Wrench className="mt-0.5 size-4 shrink-0 text-primary-600" />
        <p className="text-xs text-muted-foreground">
          将技能包写入宿主目录（如 .claude/skills、.cursor/skills 下的
          <code className="mx-1 rounded bg-muted px-1 font-mono">&lt;包名&gt;/SKILL.md</code>
          ），增强 Agent 对本项目的协作规范。可选步骤。
        </p>
      </div>

      {/* 宿主选择（全部目标宿主，按作用域分组） */}
      <HostGroup title="目标宿主 · 项目级" items={projectHosts} />
      <HostGroup title="目标宿主 · 用户级（全局）" items={userHosts} />

      {/* 技能包勾选 */}
      {packages && packages.length > 0 ? (
        <ul className="flex max-h-56 flex-col gap-1.5 overflow-y-auto pr-1">
          {packages.map((p) => (
            <li key={p.name}>
              <label className="flex cursor-pointer items-start gap-2.5 rounded-lg border border-divider px-3 py-2">
                <input
                  type="checkbox"
                  checked={selected.has(p.name)}
                  onChange={() => toggle(p.name)}
                  className="mt-1 size-3.5 accent-primary-500"
                />
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-1.5 text-sm">
                    <span className="truncate font-medium">{p.name}</span>
                    {p.source === 'builtin' && (
                      <span className="shrink-0 rounded-full bg-muted px-1.5 py-px text-[10px] text-muted-foreground">
                        内置
                      </span>
                    )}
                  </span>
                  {p.description && (
                    <span className="mt-0.5 line-clamp-1 block text-xs text-muted-foreground">
                      {p.description}
                    </span>
                  )}
                </span>
              </label>
            </li>
          ))}
        </ul>
      ) : (
        <p className="rounded-lg border border-dashed border-border px-4 py-6 text-center text-xs text-muted-foreground">
          暂无可用技能包，可跳过此步。
        </p>
      )}

      {/* 放入 AGENTS.md 的推荐提示词（与 Skills 页一致） */}
      <div className="rounded-xl bg-muted p-3">
        <div className="flex items-center gap-2">
          <ExternalLink className="size-3.5 text-primary-600" />
          <span className="text-sm font-medium">放入 AGENTS.md 的推荐提示词</span>
          <Badge
            role="button"
            variant={lang === 'zh' ? 'default' : 'outline'}
            className="ml-auto cursor-pointer px-2 py-0.5"
            onClick={() => setLang('zh')}
          >
            中文
          </Badge>
          <Badge
            role="button"
            variant={lang === 'en' ? 'default' : 'outline'}
            className="cursor-pointer px-2 py-0.5"
            onClick={() => setLang('en')}
          >
            English
          </Badge>
          <Button size="sm" variant="outline" onClick={() => void copyPrompt()}>
            {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
            {copied ? '已复制' : '复制'}
          </Button>
        </div>
        <pre className="mt-2 max-h-48 overflow-auto rounded-lg bg-background p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap">
          {lang === 'zh' ? resolvedPrompt.zh : resolvedPrompt.en}
        </pre>
        <p className="mt-1.5 text-[10px] text-muted-foreground">
          复制到项目 AGENTS.md 可让 AI 代理自动识别 TangoForge 并遵循任务管理规范。
        </p>
      </div>

      <div className="flex items-center justify-between border-t border-divider pt-4">
        <Button variant="ghost" size="sm" onClick={() => onReady(true)}>
          跳过此步
        </Button>
        <Button size="sm" onClick={() => void doInstall()} disabled={install.isPending || done}>
          {install.isPending ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : (
            <CheckCircle2 className="size-3.5" />
          )}
          {done
            ? '已安装'
            : selected.size === 0 || hosts.size === 0
              ? '不安装并继续'
              : `安装到 ${hosts.size} 个宿主`}
        </Button>
      </div>
    </div>
  )
}
