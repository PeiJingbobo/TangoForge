import { useState } from 'react'
import { BookOpen, ChevronRight } from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import { Badge } from '@/components/ui/badge'
import { useSkills, useSkillInfo } from '@/hooks/useSkills'
import { cn } from '@/lib/utils'

/** Skill 浏览（TF-028）：列表 + skill_info 详情。 */
export function SkillsPanel() {
  const { data: skills, isLoading } = useSkills()
  const [selected, setSelected] = useState<string | null>(null)
  const { data: detail } = useSkillInfo(selected ?? undefined)

  if (isLoading) return <Skeleton className="h-40 w-full" />

  return (
    <div className="grid gap-6 md:grid-cols-[280px_minmax(0,1fr)]">
      {/* 列表（行式） */}
      <div>
        <div className="mb-3 flex items-center gap-2">
          <BookOpen className="size-4 text-primary-600" />
          <h3 className="text-sm font-semibold">Skills（{skills?.length ?? 0}）</h3>
        </div>
        <Separator className="mb-3" />
        {!skills || skills.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            暂无 Skill。将 Markdown/YAML 文件放入{' '}
            <code className="rounded bg-muted px-1 font-mono text-xs">.taskboard/skills/</code>{' '}
            即可被扫描。
          </p>
        ) : (
          <ul className="space-y-0.5">
            {skills.map((s) => (
              <li key={s.name}>
                <button
                  type="button"
                  onClick={() => setSelected(s.name)}
                  className={cn(
                    'flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm transition-colors hover:bg-accent',
                    selected === s.name && 'bg-primary-50 font-semibold text-primary-700',
                  )}
                >
                  <ChevronRight
                    className={cn(
                      'size-3.5 text-muted-foreground transition-transform',
                      selected === s.name && 'rotate-90',
                    )}
                  />
                  <span className="truncate">{s.name}</span>
                  <span className="ml-auto text-caption text-muted-foreground">v{s.version}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* 详情 */}
      <div className="rounded-2xl border border-divider bg-card p-5">
        {detail ? (
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-h3 text-foreground">{detail.name}</h3>
              <Badge variant="outline">v{detail.version}</Badge>
            </div>
            {detail.description && (
              <p className="mt-2 text-sm text-muted-foreground">{detail.description}</p>
            )}
            <Separator className="my-4" />
            <pre className="max-h-[420px] overflow-auto font-mono text-xs leading-relaxed whitespace-pre-wrap text-foreground/90">
              {detail.instructions}
            </pre>
          </div>
        ) : (
          <p className="py-10 text-center text-sm text-muted-foreground">
            选择一个 Skill 查看 instructions 全文（Agent 接入指南）。
          </p>
        )}
      </div>
    </div>
  )
}
