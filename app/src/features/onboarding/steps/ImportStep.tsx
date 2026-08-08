import { useState } from 'react'
import { toast } from 'sonner'
import { CheckCircle2, FileUp, FolderOpen, Loader2, Rocket } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useImportTasks, useConfirmDraft } from '@/hooks/useImports'
import type { ImportDraft } from '@/types/models'

/**
 * 引导 Step 2：导入 Markdown 草稿（TF-041，可选）。
 * 选择文件/目录 → LLM 解析草稿 → 展示任务数 → 确认入库（文件级覆盖）；
 * 或跳过（不导入，后续可在「导入导出」页补充）。
 */
export function ImportStep({
  workdir,
  onReady,
}: {
  workdir: string
  onReady: (ok: boolean) => void
}) {
  const importTasks = useImportTasks(workdir)
  const confirmDraft = useConfirmDraft(workdir)
  const [draft, setDraft] = useState<ImportDraft | null>(null)
  const [done, setDone] = useState(false)

  const pickFiles = async () => {
    const shell = window.tangoforge?.dialog
    if (!shell) {
      toast.error('文件选择仅桌面版可用（Web 预览请粘贴内容）')
      return
    }
    const paths = await shell.selectFiles()
    if (!paths || paths.length === 0) return
    submit({ file_paths: paths })
  }

  const pickDirectory = async () => {
    const shell = window.tangoforge?.dialog
    if (!shell) {
      toast.error('目录选择仅桌面版可用')
      return
    }
    const dir = await shell.selectDirectory()
    if (!dir) return
    submit({ directory: dir })
  }

  const submit = (input: Parameters<typeof importTasks.mutate>[0]) => {
    importTasks.mutate(input, {
      onSuccess: (d) => {
        setDraft(d)
        toast.success(`解析完成：${d.task_count} 个任务`)
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : '导入解析失败'),
    })
  }

  const confirm = () => {
    if (!draft) return
    confirmDraft.mutate(draft.id, {
      onSuccess: (r) => {
        toast.success(`已导入 ${r.created} 个任务`)
        setDone(true)
        onReady(true)
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : '确认导入失败'),
    })
  }

  return (
    <div className="space-y-4">
      {!draft && !done && (
        <>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={pickFiles}
              disabled={importTasks.isPending}
            >
              {importTasks.isPending ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <FileUp className="size-3.5" />
              )}
              选择 Markdown 文件
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={pickDirectory}
              disabled={importTasks.isPending}
            >
              <FolderOpen className="size-3.5" />
              选择目录
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            解析结果先进入草稿，预览任务数后确认入库（按文件全量覆盖）。可选步骤，可跳过。
          </p>
        </>
      )}

      {draft && !done && (
        <div className="rounded-xl border border-divider bg-muted/50 p-4">
          <div className="flex items-center gap-2 text-sm font-semibold">
            <CheckCircle2 className="size-4 text-success" />
            草稿已就绪
          </div>
          <div className="mt-2 space-y-1 text-xs text-muted-foreground">
            <div className="truncate font-mono" title={draft.source_file}>
              来源: {draft.source_file}
            </div>
            <div>任务数: {draft.task_count}</div>
          </div>
          <div className="mt-3 flex gap-2">
            <Button size="sm" onClick={confirm} disabled={confirmDraft.isPending}>
              {confirmDraft.isPending ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Rocket className="size-3.5" />
              )}
              确认导入
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setDraft(null)
                onReady(true)
              }}
            >
              跳过（不导入）
            </Button>
          </div>
        </div>
      )}

      {done && (
        <div className="flex items-start gap-2 rounded-xl border border-success-soft bg-success-soft/60 p-4">
          <CheckCircle2 className="mt-0.5 size-5 shrink-0 text-success" />
          <div className="text-sm">
            <div className="font-semibold">导入完成</div>
            <div className="mt-0.5 text-muted-foreground">
              草稿已确认入库（{draft?.source_file}）。可继续下一步。
            </div>
          </div>
        </div>
      )}

      {!draft && !done && (
        <div className="flex justify-end border-t border-divider pt-3">
          <Button variant="ghost" size="sm" onClick={() => onReady(true)}>
            跳过此步
          </Button>
        </div>
      )}
    </div>
  )
}
