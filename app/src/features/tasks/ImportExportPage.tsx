import { useState } from 'react'
import { Download, FileUp } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ImportDialog } from '@/features/imports/ImportDialog'
import { DraftsPanel } from '@/features/imports/DraftsPanel'
import { DraftReview } from '@/features/imports/DraftReview'
import { ExportDialog } from '@/features/tasks/ExportDialog'

/**
 * 导入导出（TF-029 项目二级 tab）：
 * Markdown 导入（文件/目录/内容 → 草稿）+ 草稿审阅（虚拟任务体系预览/编辑/确认/丢弃）
 * + 导出（模板/目标/预览）。
 */
export function ImportExportPage() {
  const [importOpen, setImportOpen] = useState(false)
  const [exportOpen, setExportOpen] = useState(false)
  const [reviewing, setReviewing] = useState<string | null>(null)

  // 草稿审阅界面（全屏视图，覆盖列表）
  if (reviewing) {
    return <DraftReview draftId={reviewing} onExit={() => setReviewing(null)} />
  }

  return (
    <div>
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-h2 text-foreground">导入导出</h1>
          <p className="mt-1 text-caption text-muted-foreground">
            从 Markdown 导入任务（草稿审阅后确认）或按导出模板渲染回写。
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => setImportOpen(true)} aria-label="打开导入">
            <FileUp className="size-4" />
            导入
          </Button>
          <Button variant="outline" onClick={() => setExportOpen(true)} aria-label="打开导出">
            <Download className="size-4" />
            导出
          </Button>
        </div>
      </div>

      <div className="mb-6">
        <DraftsPanel onReview={setReviewing} />
      </div>

      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>从 Markdown 导入</DialogTitle>
            <DialogDescription>
              解析结果先进入草稿，审阅确认后按文件全量覆盖入库。
            </DialogDescription>
          </DialogHeader>
          <ImportDialog onOpenChange={setImportOpen} />
        </DialogContent>
      </Dialog>

      <ExportDialog open={exportOpen} onOpenChange={setExportOpen} />
    </div>
  )
}
