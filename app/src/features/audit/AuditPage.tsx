import { useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { useAudit } from '@/hooks/useAudit'
import type { AuditEntry } from '@/types/models'

/**
 * 审计日志（TF-029 项目二级 tab）：ts 倒序，action/actor 精确过滤。
 */
export function AuditPage() {
  const [action, setAction] = useState<string | null>(null)
  const { data, isLoading, refetch, isFetching } = useAudit({ action: action ?? undefined })

  return (
    <div>
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-h2 text-foreground">审计日志</h1>
          <p className="mt-1 text-caption text-muted-foreground">
            当前项目的全部写操作记录（只追加，不可修改）。
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void refetch()}
          disabled={isFetching}
          aria-label="刷新审计"
        >
          <RefreshCw className={isFetching ? 'size-3.5 animate-spin' : 'size-3.5'} />
          刷新
        </Button>
      </div>

      <div className="mb-4 flex flex-wrap gap-1.5">
        <Badge
          variant={action === null ? 'default' : 'outline'}
          className="cursor-pointer px-3 py-1"
          onClick={() => setAction(null)}
        >
          全部
        </Badge>
        {[
          'task.created',
          'task.updated',
          'task.archived',
          'task.restored',
          'import.confirmed',
          'config.updated',
        ].map((a) => (
          <Badge
            key={a}
            variant={action === a ? 'default' : 'outline'}
            className="cursor-pointer px-3 py-1 font-mono"
            onClick={() => setAction(action === a ? null : a)}
          >
            {a}
          </Badge>
        ))}
      </div>

      {isLoading ? (
        <div className="space-y-2">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-12 rounded-xl" />
          ))}
        </div>
      ) : !data?.items || data.items.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-border p-14 text-center text-body text-muted-foreground">
          暂无审计记录。
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-divider">
          <table className="w-full text-sm">
            <thead className="bg-muted/60 text-left text-xs text-muted-foreground">
              <tr>
                <th className="px-4 py-2.5 font-medium">时间</th>
                <th className="px-4 py-2.5 font-medium">动作</th>
                <th className="px-4 py-2.5 font-medium">来源</th>
                <th className="px-4 py-2.5 font-medium">结果</th>
                <th className="px-4 py-2.5 font-medium">目标</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-divider">
              {data.items.map((e: AuditEntry, i: number) => (
                <tr key={`${e.ts}-${i}`} className="hover:bg-accent/40">
                  <td className="whitespace-nowrap px-4 py-2.5 font-mono text-xs">
                    {new Date(e.ts).toLocaleString()}
                  </td>
                  <td className="px-4 py-2.5">
                    <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
                      {e.action}
                    </code>
                  </td>
                  <td className="px-4 py-2.5 text-muted-foreground">{e.actor}</td>
                  <td className="px-4 py-2.5">
                    <Badge
                      variant="outline"
                      className={
                        e.result === 'ok'
                          ? 'bg-success-soft text-success-ink'
                          : 'bg-destructive-soft text-destructive-ink'
                      }
                    >
                      {e.result}
                    </Badge>
                  </td>
                  <td className="max-w-48 truncate px-4 py-2.5 font-mono text-xs text-muted-foreground">
                    {e.target}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
