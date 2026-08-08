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

  const [selectedHost, setSelectedHost] = useState<string>('')
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

  const togglePkg = (name: string) => {
    setSelectedPkgs((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  const doInstall = (host: string, pkgs: string[]) => {
    install.mutate(
      { host, packages: pkgs },
      {
        onSuccess: (results) => {
          const failed = results.filter((r) => !r.ok)
          const ok = results.filter((r) => r.ok)
          if (ok.length > 0) toast.success(`已安装到 ${host}：${ok.map((r) => r.name).join('、')}`)
          for (const f of failed) toast.error(`${f.name} 安装失败：${f.error}`)
          setSelectedPkgs(new Set())
        },
        onError: (e) => toast.error(e instanceof Error ? e.message : '安装失败'),
      },
    )
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
    <div className="space-y-6">
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
                  variant={selectedHost === h.key ? 'default' : 'outline'}
                  className="cursor-pointer gap-1 px-3 py-1.5"
                  onClick={() => setSelectedHost(h.key)}
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
                  const installed = selectedHost
                    ? stateMap.get(selectedHost)?.get(p.name)
                    : undefined
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
              disabled={!selectedHost || selectedPkgs.size === 0 || install.isPending}
              onClick={() => doInstall(selectedHost, [...selectedPkgs])}
            >
              {install.isPending && <Loader2 className="size-4 animate-spin" />}
              <Download className="size-4" />
              {install.isPending ? '安装中…' : `安装到 ${selectedHost || '…'}`}
            </Button>
            {selectedHost && selectedPkgs.size > 0 && (
              <span className="text-caption text-muted-foreground">
                {selectedPkgs.size} 个包批量安装
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
                            onClick={() => doInstall(hs.key, stalePkgs)}
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

const AGENTS_PROMPT_ZH = `> TangoForge 是本项目使用的任务管理中间件（本地守护进程，端口 19810）。
> 当你需要管理本项目任务（创建/查询/更新/流转状态/导入导出 Markdown）时，
> 应通过 TangoForge 完成，而不是自己臆造数据。
>
> 用法：先读说明书 \`curl http://127.0.0.1:19810/api/guide\`（或 MCP guide 工具），
> 按其中 HTTP/MCP/CLI 方式调用；所有请求必须带 X-Project 项目目录头。
> 项目下已安装的 Skill 位于 .claude/skills / .cursor/skills / .github/skills 等宿主目录，
> 命中任务场景时优先读取对应 SKILL.md 按其流程执行。`

const AGENTS_PROMPT_EN = `> TangoForge is the task management middleware used by this project
> (local daemon, port 19810). Whenever you need to manage project tasks
> (create/query/update/transition status/import-export Markdown), use TangoForge
> instead of inventing data.
>
> Usage: first read the guide at \`curl http://127.0.0.1:19810/api/guide\`
> (or the MCP "guide" tool), then call via HTTP/MCP/CLI per its instructions;
> every request must carry the X-Project header with the project workdir.
> Installed Skills live in .claude/skills / .cursor/skills / .github/skills etc.,
> read the matching SKILL.md and follow it when a task scenario hits.`

function SkillLibrary() {
  const { data: packages, isLoading } = useSkillPackages()
  const writePkg = useSkillPackageWrite()
  const [expanded, setExpanded] = useState<string | null>(null)
  const [draft, setDraft] = useState<string | null>(null) // 新建/编辑草稿
  const [editingName, setEditingName] = useState<string | null>(null)
  const [lang, setLang] = useState<'zh' | 'en'>('zh')
  const [copied, setCopied] = useState(false)

  if (isLoading) return <Skeleton className="h-40 w-full" />

  const copyPrompt = async () => {
    try {
      await navigator.clipboard.writeText(lang === 'zh' ? AGENTS_PROMPT_ZH : AGENTS_PROMPT_EN)
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
          {lang === 'zh' ? AGENTS_PROMPT_ZH : AGENTS_PROMPT_EN}
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
