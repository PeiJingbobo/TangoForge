import { useState } from 'react'
import { toast } from 'sonner'
import { Check, Eye, FileText, Loader2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { useDrafts, useConfirmDraft, useDiscardDraft } from '@/hooks/useImports'

/**
 * 导入草稿面板（TF-027 + 审阅入口）：pending 草稿列表 + 审阅（虚拟任务体系预览/编辑）+
 * 确认（文件级覆盖）/ 丢弃。确认成功后反馈 created/archived 数量。
 */
export interface DraftsPanelProps {
  /** 打开草稿审阅界面 */
  onReview?: (draftId: string) => void
}

export function DraftsPanel({ onReview }: DraftsPanelProps) {
  const { data: drafts, isLoading } = useDrafts()
  const confirmDraft = useConfirmDraft()
  const discardDraft = useDiscardDraft()
  const [busyId, setBusyId] = useState<string | null>(null)

  if (isLoading) return null
  if (!drafts || drafts.length === 0) return null

  const handleConfirm = (id: string) => {
    setBusyId(id)
    confirmDraft.mutate(id, {
      onSuccess: (r) => {
        toast.success('导入完成', {
          description: `${r.source_file}：创建 ${r.created} 个，覆盖归档 ${r.archived} 个`,
        })
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : '确认失败'),
      onSettled: () => setBusyId(null),
    })
  }

  const handleDiscard = (id: string) => {
    setBusyId(id)
    discardDraft.mutate(id, {
      onSuccess: () => toast.success('草稿已丢弃'),
      onError: (e) => toast.error(e instanceof Error ? e.message : '丢弃失败'),
      onSettled: () => setBusyId(null),
    })
  }

  return (
    <div className="rounded-2xl border border-border bg-card">
      <div className="flex items-center justify-between px-5 py-3.5">
        <div className="flex items-center gap-2">
          <FileText className="size-4 text-primary-600" />
          <h3 className="text-sm font-semibold">待确认的导入草稿</h3>
          <Badge>{drafts.length}</Badge>
        </div>
      </div>
      <Separator />
      <ul className="divide-y divide-divider">
        {drafts.map((d) => (
          <li key={d.id} className="flex items-center gap-3 px-5 py-3">
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-medium">{d.source_file}</div>
              <div className="text-caption text-muted-foreground">
                {d.task_count} 个任务 · {new Date(d.created_at).toLocaleString()}
              </div>
            </div>
            {onReview && (
              <Button
                variant="outline"
                size="sm"
                disabled={busyId === d.id}
                onClick={() => onReview(d.id)}
                aria-label={`审阅草稿 ${d.source_file}`}
              >
                <Eye className="size-3.5" />
                审阅
              </Button>
            )}
            <Button
              size="sm"
              disabled={busyId === d.id}
              onClick={() => handleConfirm(d.id)}
              aria-label={`确认导入 ${d.source_file}`}
            >
              {busyId === d.id ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Check className="size-3.5" />
              )}
              确认导入
            </Button>
            <Button
              variant="ghost"
              size="sm"
              disabled={busyId === d.id}
              onClick={() => handleDiscard(d.id)}
              aria-label={`丢弃草稿 ${d.source_file}`}
              className="text-muted-foreground hover:text-destructive-ink"
            >
              <X className="size-3.5" />
              丢弃
            </Button>
          </li>
        ))}
      </ul>
    </div>
  )
}
