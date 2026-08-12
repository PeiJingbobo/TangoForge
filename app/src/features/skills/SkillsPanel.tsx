import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import {
  BookOpen,
  Check,
  ChevronDown,
  Copy,
  Download,
  ExternalLink,
  Info,
  Loader2,
  Package,
  RefreshCw,
  Sparkles,
  Trash2,
  Wand2,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useConfig } from '@/hooks/useConfig'
import { useStateMachine } from '@/hooks/useStateMachine'
import { useProjectId } from '@/hooks/useProject'
import {
  useSkillInstall,
  useSkillPackageWrite,
  useSkillPackages,
  useSkillStatus,
  useSkillUninstall,
} from '@/hooks/useSkills'
import { cn } from '@/lib/utils'
import type { SkillPackage } from '@/types/models'
import { SKILL_HOSTS } from '@/features/skills/hosts'

/** 状态徽章样式 */
const STATE_META: Record<string, { label: string; cls: string }> = {
  missing: { label: '未安装', cls: 'bg-muted text-muted-foreground' },
  current: { label: '已安装', cls: 'bg-success-soft text-success-ink' },
  stale: { label: '可更新', cls: 'bg-warning-soft text-warning-ink' },
}

const STATE_LABEL: Record<string, string> = {
  missing: '未安装',
  current: '已安装',
  stale: '有新版',
}

/** Skill 管理（TF-033 重设计）：技能库 + 宿主安装矩阵 + AGENTS.md 提示词复制。 */
export function SkillsPanel() {
  const { data: packages, isLoading: loadingPackages } = useSkillPackages()
  const {
    data: status,
    isLoading: loadingStatus,
    refetch: refetchStatus,
    isFetching,
  } = useSkillStatus()
  const install = useSkillInstall()
  const uninstall = useSkillUninstall()

  const [selectedHosts, setSelectedHosts] = useState<Set<string>>(new Set())
  const [selectedPkgs, setSelectedPkgs] = useState<Set<string>>(new Set())
  const [confirmHost, setConfirmHost] = useState<string | null>(null)
  const [confirmPkgs, setConfirmPkgs] = useState<string[]>([])

  // 状态速查：host → pkg → state
  const stateMap = useMemo(() => {
    const m = new Map<string, Map<string, string>>()
    for (const hs of status ?? []) {
      const inner = new Map<string, string>()
      for (const inst of hs.installed) inner.set(inst.name, inst.state)
      m.set(hs.key, inner)
    }
    return m
  }, [status])

  if (loadingPackages || loadingStatus) return <Skeleton className="h-64 w-full" />

  const toggleHost = (key: string) => {
    setSelectedHosts((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  const togglePkg = (name: string) => {
    setSelectedPkgs((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  /** 批量安装到多个宿主：每个宿主一次 POST（后端为单宿主语义），合并结果统一提示。 */
  const doInstall = async (hosts: string[], pkgs: string[]) => {
    const perHost = await Promise.all(
      hosts.map(async (h) => {
        try {
          return await install.mutateAsync({ host: h, packages: pkgs })
        } catch (e) {
          return [
            {
              name: '',
              host: h,
              action: 'install',
              ok: false,
              error: e instanceof Error ? e.message : '安装失败',
            },
          ]
        }
      }),
    )
    const results = perHost.flat()
    const failed = results.filter((r) => !r.ok)
    const ok = results.filter((r) => r.ok)
    if (ok.length > 0) {
      toast.success(
        `已安装到 ${hosts.length} 个宿主：${ok.map((r) => `${r.host}/${r.name}`).join('、')}`,
      )
    }
    for (const f of failed) toast.error(`${f.host} ${f.name || ''} 安装失败：${f.error}`)
    setSelectedPkgs(new Set())
  }

  const doUninstall = (host: string, pkgs: string[]) => {
    uninstall.mutate(
      { host, packages: pkgs },
      {
        onSuccess: (results) => {
          const failed = results.filter((r) => !r.ok)
          if (failed.length === 0) toast.success(`已从 ${host} 卸载`)
          else for (const f of failed) toast.error(`${f.name} 卸载失败：${f.error}`)
        },
        onError: (e) => toast.error(e instanceof Error ? e.message : '卸载失败'),
      },
    )
    setConfirmHost(null)
    setConfirmPkgs([])
  }

  return (
    // 主体内部滚动（TF-042）：占满父容器剩余高度，仅此面板滚动，四周留细微间距
    <div className="flex h-full min-h-0 flex-col gap-6 overflow-y-auto p-1 pr-2">
      {/* ① 安装向导：先选宿主 → 勾选包 → 批量安装（QA Q3） */}
      <section className="rounded-2xl border border-divider bg-card p-5">
        <div className="flex items-center gap-2">
          <Wand2 className="size-4 text-primary-600" />
          <h3 className="text-sm font-semibold">安装向导</h3>
          <span className="text-caption text-muted-foreground">
            为指定 Agent 工具安装 Skill 技能包
          </span>
        </div>
        <Separator className="my-3" />
        <div className="space-y-4">
          <div>
            <div className="mb-1.5 text-label text-muted-foreground">① 选择目标宿主</div>
            <div className="flex flex-wrap gap-1.5">
              {SKILL_HOSTS.map((h) => (
                <Badge
                  key={h.key}
                  role="button"
                  variant={selectedHosts.has(h.key) ? 'default' : 'outline'}
                  className="cursor-pointer gap-1 px-3 py-1.5"
                  onClick={() => toggleHost(h.key)}
                >
                  {h.label}
                  <span
                    className={cn(
                      'rounded px-1 text-[10px]',
                      h.scope === 'project' ? 'bg-primary-soft text-primary-700' : 'bg-muted',
                    )}
                  >
                    {h.scope === 'project' ? '项目' : '用户'}
                  </span>
                </Badge>
              ))}
            </div>
          </div>
          <div>
            <div className="mb-1.5 text-label text-muted-foreground">② 勾选要安装的技能包</div>
            {!packages || packages.length === 0 ? (
              <p className="text-sm text-muted-foreground">暂无可用技能包。</p>
            ) : (
              <div className="flex flex-wrap gap-1.5">
                {packages.map((p) => {
                  // 多宿主选中时取第一个选中宿主的状态作提示（安装后状态矩阵自动刷新）。
                  const probeHost = [...selectedHosts][0]
                  const installed = probeHost ? stateMap.get(probeHost)?.get(p.name) : undefined
                  return (
                    <Badge
                      key={p.name}
                      role="button"
                      variant={selectedPkgs.has(p.name) ? 'default' : 'outline'}
                      className="cursor-pointer gap-1 px-3 py-1.5"
                      onClick={() => togglePkg(p.name)}
                      title={p.description}
                    >
                      <Package className="size-3" />
                      {p.name}
                      {installed && (
                        <span className="rounded bg-muted px-1 text-[10px] text-muted-foreground">
                          {STATE_LABEL[installed]}
                        </span>
                      )}
                    </Badge>
                  )
                })}
              </div>
            )}
          </div>
          <div className="flex items-center gap-2">
            <Button
              disabled={selectedHosts.size === 0 || selectedPkgs.size === 0 || install.isPending}
              onClick={() => void doInstall([...selectedHosts], [...selectedPkgs])}
            >
              {install.isPending && <Loader2 className="size-4 animate-spin" />}
              <Download className="size-4" />
              {install.isPending
                ? '安装中…'
                : selectedHosts.size === 0
                  ? '选择目标宿主'
                  : `安装到 ${selectedHosts.size} 个宿主`}
            </Button>
            {selectedHosts.size > 0 && selectedPkgs.size > 0 && (
              <span className="text-caption text-muted-foreground">
                {selectedPkgs.size} 个包 × {selectedHosts.size} 个宿主批量安装
              </span>
            )}
          </div>
        </div>
      </section>

      {/* ② 安装状态矩阵 */}
      <section className="rounded-2xl border border-divider bg-card p-5">
        <div className="flex items-center gap-2">
          <BookOpen className="size-4 text-primary-600" />
          <h3 className="text-sm font-semibold">安装状态</h3>
          <span className="text-caption text-muted-foreground">
            各宿主位置的技能包安装情况（实时扫描）
          </span>
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto"
            onClick={() => void refetchStatus()}
            disabled={isFetching}
          >
            <RefreshCw className={cn('size-3.5', isFetching && 'animate-spin')} />
            刷新
          </Button>
        </div>
        <Separator className="my-3" />
        {!status || status.length === 0 ? (
          <p className="text-sm text-muted-foreground">无宿主数据。</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[560px] text-sm">
              <thead>
                <tr className="border-b border-divider text-left text-label text-muted-foreground">
                  <th className="py-2 pr-3 font-medium">宿主</th>
                  {packages?.map((p) => (
                    <th key={p.name} className="px-2 py-2 font-medium">
                      {p.name}
                    </th>
                  ))}
                  <th className="py-2 pl-3 font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                {status.map((hs) => {
                  const hsPkgs = hs.installed
                  const stalePkgs = hsPkgs.filter((i) => i.state === 'stale').map((i) => i.name)
                  return (
                    <tr key={hs.key} className="border-b border-divider/60">
                      <td className="py-2 pr-3">
                        <div className="font-medium">{hs.label}</div>
                        <div className="text-caption text-muted-foreground">
                          {hs.scope === 'project' ? '项目级' : '用户级'} · {hs.key}
                        </div>
                      </td>
                      {hsPkgs.map((inst) => {
                        const meta = STATE_META[inst.state] ?? STATE_META.missing
                        return (
                          <td key={inst.name} className="px-2 py-2 text-center">
                            <span
                              className={cn(
                                'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium',
                                meta.cls,
                              )}
                            >
                              {inst.state === 'current' && <Check className="size-3" />}
                              {inst.state === 'stale' && <Info className="size-3" />}
                              {meta.label}
                            </span>
                          </td>
                        )
                      })}
                      <td className="py-2 pl-3">
                        <div className="flex items-center gap-1.5">
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={stalePkgs.length === 0 || install.isPending}
                            onClick={() => void doInstall([hs.key], stalePkgs)}
                            title={stalePkgs.length ? `更新 ${stalePkgs.join('、')}` : '均已是最新'}
                          >
                            <Download className="size-3" />
                            更新
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            className="text-destructive-ink"
                            disabled={!hsPkgs.some((i) => i.state !== 'missing')}
                            onClick={() => {
                              const installedNames = hsPkgs
                                .filter((i) => i.state !== 'missing')
                                .map((i) => i.name)
                              setConfirmHost(hs.key)
                              setConfirmPkgs(installedNames)
                            }}
                          >
                            <Trash2 className="size-3" />
                            卸载
                          </Button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* ③ 技能库 */}
      <SkillLibrary />

      {/* 卸载二次确认（QA Q5） */}
      <Dialog open={confirmHost !== null} onOpenChange={(open) => !open && setConfirmHost(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认卸载技能包</DialogTitle>
            <DialogDescription>
              将从宿主 <code className="font-mono">{confirmHost}</code> 移除以下技能包（移除标记段 /
              删除安装文件），Agent 将不再自动发现它们：
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-wrap gap-1.5">
            {confirmPkgs.map((n) => (
              <Badge key={n} variant="outline">
                {n}
              </Badge>
            ))}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmHost(null)}>
              取消
            </Button>
            <Button
              variant="destructive"
              disabled={uninstall.isPending}
              onClick={() => confirmHost && doUninstall(confirmHost, confirmPkgs)}
            >
              {uninstall.isPending && <Loader2 className="size-4 animate-spin" />}
              确认卸载
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

/* ---------- 技能库（内置 + 自定义，含宿主提示词复制） ---------- */

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

function SkillLibrary() {
  const { data: packages, isLoading } = useSkillPackages()
  const { data: skillStatus } = useSkillStatus()
  const { data: config } = useConfig()
  const { data: stateMachine } = useStateMachine()
  const projectId = useProjectId()
  const writePkg = useSkillPackageWrite()
  const [expanded, setExpanded] = useState<string | null>(null)
  const [draft, setDraft] = useState<string | null>(null) // 新建/编辑草稿
  const [editingName, setEditingName] = useState<string | null>(null)
  const [lang, setLang] = useState<'zh' | 'en'>('zh')
  const [copied, setCopied] = useState(false)

  const resolvedPrompt = useMemo(() => {
    const port = String(config?.port ?? 19810)
    const projectDir = projectId ?? '<项目绝对路径>'
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
  }, [config, skillStatus, stateMachine, projectId])

  if (isLoading) return <Skeleton className="h-40 w-full" />

  const copyPrompt = async () => {
    try {
      await navigator.clipboard.writeText(lang === 'zh' ? resolvedPrompt.zh : resolvedPrompt.en)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error('复制失败，请手动选择复制')
    }
  }

  const startEdit = (pkg: SkillPackage) => {
    setEditingName(pkg.name)
    setDraft(pkg.content)
  }

  const startNew = () => {
    setEditingName('')
    setDraft(
      '---\nname: my-skill\ndescription: 自定义技能\nversion: "0.1.0"\nhosts: [.claude/skills, .cursor/skills, .github/skills, user-claude, user-codebuddy]\nwhen_to_use: 何时使用本技能\n---\n# 技能正文\n\n（填写 AI 操作指引）\n',
    )
  }

  const saveDraft = () => {
    if (!draft || editingName === null) return
    const name = editingName || extractNameFromFrontmatter(draft)
    if (!name) {
      toast.error('frontmatter 中缺少 name 字段')
      return
    }
    writePkg.mutate(
      { name, content: draft },
      {
        onSuccess: () => {
          toast.success(`技能包 ${name} 已保存（全局技能库）`)
          setDraft(null)
          setEditingName(null)
        },
        onError: (e) => toast.error(e instanceof Error ? e.message : '保存失败'),
      },
    )
  }

  return (
    <section className="rounded-2xl border border-divider bg-card p-5">
      <div className="flex items-center gap-2">
        <Sparkles className="size-4 text-primary-600" />
        <h3 className="text-sm font-semibold">技能库</h3>
        <span className="text-caption text-muted-foreground">
          内置 + 全局技能库（~/.taskboard-app/skills/），可自定义编辑
        </span>
        <Button size="sm" variant="outline" className="ml-auto" onClick={startNew}>
          <Wand2 className="size-3.5" />
          新建技能包
        </Button>
      </div>
      <Separator className="my-3" />

      {/* AGENTS.md 推荐提示词（QA-S7 仅 UI 复制） */}
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
        <pre className="mt-2 overflow-auto rounded-lg bg-background p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap">
          {lang === 'zh' ? resolvedPrompt.zh : resolvedPrompt.en}
        </pre>
      </div>

      {/* 包列表 */}
      {!packages || packages.length === 0 ? (
        <p className="mt-3 text-sm text-muted-foreground">
          暂无技能包。内置包会随系统发布，自定义包写入后显示于此。
        </p>
      ) : (
        <ul className="mt-3 divide-y divide-divider/60">
          {packages.map((p) => (
            <li key={p.name}>
              <button
                type="button"
                className="flex w-full items-center gap-2 py-2.5 text-left"
                onClick={() => setExpanded(expanded === p.name ? null : p.name)}
              >
                <ChevronDown
                  className={cn(
                    'size-3.5 text-muted-foreground transition-transform',
                    expanded === p.name && 'rotate-180',
                  )}
                />
                <Package className="size-4 text-primary-600" />
                <span className="font-medium">{p.name}</span>
                <Badge variant="outline">v{p.version}</Badge>
                {p.source === 'builtin' ? (
                  <Badge variant="secondary" className="text-[10px]">
                    内置
                  </Badge>
                ) : (
                  <Badge variant="secondary" className="text-[10px]">
                    自定义
                  </Badge>
                )}
                <span className="ml-auto truncate text-caption text-muted-foreground">
                  {p.description}
                </span>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={(e) => {
                    e.stopPropagation()
                    startEdit(p)
                  }}
                >
                  编辑
                </Button>
              </button>
              {expanded === p.name && (
                <div className="pb-3 pl-5">
                  <p className="text-caption text-muted-foreground">
                    <span className="font-medium">触发场景：</span>
                    {p.when_to_use || '—'}
                  </p>
                  <p className="mt-1 text-caption text-muted-foreground">
                    <span className="font-medium">适用宿主：</span>
                    {p.hosts.join('、') || '全部'}
                  </p>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}

      {/* 新建/编辑对话框 */}
      <Dialog
        open={draft !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDraft(null)
            setEditingName(null)
          }
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{editingName ? `编辑技能包 ${editingName}` : '新建技能包'}</DialogTitle>
            <DialogDescription>
              编辑 SKILL.md（YAML frontmatter：name/description/version/hosts/when_to_use + 正文）。
              保存到全局技能库（~/.taskboard-app/skills/）。
            </DialogDescription>
          </DialogHeader>
          <textarea
            value={draft ?? ''}
            onChange={(e) => setDraft(e.target.value)}
            className="h-72 w-full resize-y rounded-lg border border-divider bg-background p-3 font-mono text-xs leading-relaxed"
            aria-label="SKILL.md 内容"
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setDraft(null)}>
              取消
            </Button>
            <Button disabled={writePkg.isPending} onClick={saveDraft}>
              {writePkg.isPending && <Loader2 className="size-4 animate-spin" />}
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  )
}

/** 从 frontmatter 提取 name（新建包时按内容识别） */
function extractNameFromFrontmatter(content: string): string {
  const m = content.match(/^---\s*\n([\s\S]*?)\n---/)
  if (!m) return ''
  const name = m[1].match(/^name:\s*["']?([^\s"']+)["']?\s*$/m)
  return name?.[1] ?? ''
}
