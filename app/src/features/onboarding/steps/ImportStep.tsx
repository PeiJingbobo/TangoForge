import { useState } from 'react'
import { toast } from 'sonner'
import { CheckCircle2, FileUp, FolderOpen, Loader2, Rocket, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useImportTasks } from '@/hooks/useImports'
import { DraftReview } from '@/features/imports/DraftReview'
import type { ImportDraft } from '@/types/models'

/**
 * 引导 Step 2：导入 Markdown 草稿（TF-041，可选）。
 * 选择文件/目录 → 展示已选列表由用户确认 → LLM 解析草稿 → Dialog 内预览/修改
 * （复用 DraftReview）→ 确认入库后同步 onReady(true) 进入下一步；或跳过。
 * 文件列表交互参考「导入导出」ImportDialog（多选 Badge 可移除 + 目录 chip）。
 */
export function ImportStep({
  workdir,
  onReady,
}: {
  workdir: string
  onReady: (ok: boolean) => void
}) {
  const importTasks = useImportTasks(workdir)
  const [dir, setDir] = useState<string | null>(null)
  const [paths, setPaths] = useState<string[]>([])
  const [draft, setDraft] = useState<ImportDraft | null>(null)
  const [done, setDone] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)

  const pickFiles = async () => {
    const shell = window.tangoforge?.dialog
    if (!shell) {
      toast.error('文件选择仅桌面版可用（Web 预览请粘贴内容）')
      return
    }
    const files = await shell.selectFiles()
    if (!files || files.length === 0) return
    setPaths((prev) => [...new Set([...prev, ...files])])
  }

  const pickDirectory = async () => {
    const shell = window.tangoforge?.dialog
    if (!shell) {
      toast.error('目录选择仅桌面版可用')
      return
    }
    const d = await shell.selectDirectory()
    if (!d) return
    setDir(d)
  }

  const submit = () => {
    const input = dir !== null ? { directory: dir } : { file_paths: paths }
    importTasks.mutate(input, {
      onSuccess: (d) => {
        setDraft(d)
        setPreviewOpen(true)
        toast.success(`解析完成：${d.task_count} 个任务`)
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : '导入解析失败'),
    })
  }

  const resetSelection = () => {
    setDir(null)
    setPaths([])
    setDraft(null)
    setDone(false)
  }

  const canSubmit = importTasks.isPending || (dir === null && paths.length === 0)

  return (
    <div className="space-y-4">
      {!draft && !done && (
        <>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => void pickFiles()}
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
              onClick={() => void pickDirectory()}
              disabled={importTasks.isPending}
            >
              <FolderOpen className="size-3.5" />
              选择目录
            </Button>
          </div>

          {/* 已选文件列表（供用户确认，参考 ImportDialog 交互） */}
          {dir && (
            <div className="flex items-center gap-2 rounded-lg border border-primary-200 bg-primary-50 px-3 py-2">
              <FolderOpen className="size-4 shrink-0 text-primary-600" />
              <span className="min-w-0 flex-1 truncate font-mono text-xs">{dir}</span>
              <button
                type="button"
                aria-label="清除目录选择"
                onClick={() => setDir(null)}
                className="rounded p-0.5 text-muted-foreground hover:text-foreground"
              >
                <X className="size-3.5" />
              </button>
            </div>
          )}
          {paths.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {paths.map((p) => (
                <Badge
                  key={p}
                  variant="outline"
                  className="max-w-64 gap-1 py-1 pr-1 pl-2.5 font-mono text-xs"
                >
                  <span className="truncate">{p}</span>
                  <button
                    type="button"
                    aria-label={`移除 ${p}`}
                    onClick={() => setPaths((prev) => prev.filter((x) => x !== p))}
                    className="rounded-full p-0.5 text-muted-foreground hover:text-destructive-ink"
                  >
                    <X className="size-3" />
                  </button>
                </Badge>
              ))}
            </div>
          )}

          {(dir !== null || paths.length > 0) && (
            <div className="flex items-center gap-2">
              <Button size="sm" onClick={submit} disabled={canSubmit}>
                {importTasks.isPending ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : (
                  <Rocket className="size-3.5" />
                )}
                解析为草稿
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={resetSelection}
                disabled={importTasks.isPending}
              >
                清除选择
              </Button>
            </div>
          )}

          <p className="text-xs text-muted-foreground">
            支持多选 Markdown
            文件，或选择目录递归扫描；解析结果先进入草稿，可在弹窗中预览与修改后确认入库（按文件全量覆盖）。可选步骤，可跳过。
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
            <Button size="sm" onClick={() => setPreviewOpen(true)}>
              预览并修改草稿
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

      {/* 草稿预览/修改 Dialog（复用 DraftReview） */}
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className="flex max-h-[92vh] w-[min(94vw,1080px)] flex-col gap-0 overflow-hidden p-0">
          <DialogHeader className="sr-only">
            <DialogTitle>草稿预览</DialogTitle>
            <DialogDescription>预览并修改解析出的任务草稿，确认后入库。</DialogDescription>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto p-6">
            {draft && (
              <DraftReview
                draftId={draft.id}
                project={workdir}
                onConfirmed={() => {
                  setDone(true)
                  setPreviewOpen(false)
                  onReady(true)
                }}
                onExit={() => setPreviewOpen(false)}
              />
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
