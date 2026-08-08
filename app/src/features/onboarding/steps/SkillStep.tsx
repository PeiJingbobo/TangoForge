import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { CheckCircle2, Loader2, Wrench } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useSkillPackages, useSkillInstall } from '@/hooks/useSkills'
import { SKILL_HOSTS, DEFAULT_SKILL_HOST } from '@/features/skills/hosts'

/**
 * 引导 Step 4：为工作目录安装 Skill（TF-041 简化版）。
 * 宿主选择与项目 Skills 配置页共用 SKILL_HOSTS（TF-042：全部目录型 .xxx/skills，
 * 无单文件 .md 宿主），按项目级/用户级分组展示全部目标宿主；默认 .claude/skills。
 */
export function SkillStep({
  workdir,
  onReady,
}: {
  workdir: string
  onReady: (ok: boolean) => void
}) {
  const { data: packages, isLoading } = useSkillPackages(workdir)
  const install = useSkillInstall(workdir)
  const [host, setHost] = useState(DEFAULT_SKILL_HOST)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [done, setDone] = useState(false)

  const projectHosts = SKILL_HOSTS.filter((h) => h.scope === 'project')
  const userHosts = SKILL_HOSTS.filter((h) => h.scope === 'user')

  // 默认全选内置包（source=builtin）。
  useEffect(() => {
    if (packages && packages.length > 0) {
      setSelected(new Set(packages.filter((p) => p.source === 'builtin').map((p) => p.name)))
    }
  }, [packages])

  const toggle = (name: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
    setDone(false)
  }

  const doInstall = () => {
    if (selected.size === 0) {
      onReady(true)
      return
    }
    install.mutate(
      { host, packages: [...selected] },
      {
        onSuccess: (results) => {
          const ok = results.filter((r) => r.ok).length
          const fail = results.filter((r) => !r.ok)
          if (fail.length > 0) {
            toast.error(`${fail.length} 个包安装失败: ${fail[0].error ?? ''}`)
          } else {
            toast.success(`已安装 ${ok} 个技能包到 ${host}`)
          }
          setDone(true)
          onReady(true)
        },
        onError: (e) => toast.error(e instanceof Error ? e.message : '安装失败'),
      },
    )
  }

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" /> 加载技能包…
      </div>
    )
  }

  const HostGroup = ({ title, hosts }: { title: string; hosts: typeof SKILL_HOSTS }) => (
    <div>
      <div className="mb-1.5 text-label text-muted-foreground">{title}</div>
      <div className="flex flex-wrap gap-2">
        {hosts.map((h) => (
          <button
            key={h.key}
            type="button"
            onClick={() => setHost(h.key)}
            className={`rounded-full border px-3 py-1 text-xs font-medium transition-colors ${
              host === h.key
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
      <HostGroup title="目标宿主 · 项目级" hosts={projectHosts} />
      <HostGroup title="目标宿主 · 用户级（全局）" hosts={userHosts} />

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

      <div className="flex items-center justify-between border-t border-divider pt-4">
        <Button variant="ghost" size="sm" onClick={() => onReady(true)}>
          跳过此步
        </Button>
        <Button size="sm" onClick={doInstall} disabled={install.isPending || done}>
          {install.isPending ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : (
            <CheckCircle2 className="size-3.5" />
          )}
          {done ? '已安装' : selected.size === 0 ? '不安装并继续' : `安装到 ${host}`}
        </Button>
      </div>
    </div>
  )
}
