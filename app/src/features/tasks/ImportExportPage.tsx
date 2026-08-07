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
import { ExportPanel } from '@/features/tasks/ExportPanel'
import { ImportDialog } from '@/features/imports/ImportDialog'
import { DraftsPanel } from '@/features/imports/DraftsPanel'
import { DraftReview } from '@/features/imports/DraftReview'
import { cn } from '@/lib/utils'

/**
 * 导入导出（TF-029 / TF-039 分区）：
 * Tab 分区「导出」（在前，更常用）/「导入」。
 * - 导出：ExportDialog（模板预览/生成）+ 导出记录列表 + 完成提示（打开目录/文件）；
 * - 导入：Markdown 导入（文件/目录/内容 → 草稿）+ 草稿审阅。
 */
type Section = 'export' | 'import'

export function ImportExportPage() {
  const [section, setSection] = useState<Section>('export')
  const [importOpen, setImportOpen] = useState(false)
  const [reviewing, setReviewing] = useState<string | null>(null)

  // 草稿审阅界面（全屏视图，覆盖列表）
  if (reviewing) {
    return <DraftReview draftId={reviewing} onExit={() => setReviewing(null)} />
  }

  const tabs: { key: Section; label: string }[] = [
    { key: 'export', label: '导出' },
    { key: 'import', label: '导入' },
  ]

  return (
    <div>
      <div className="mb-5">
        <h1 className="text-h2 text-foreground">导入导出</h1>
        <p className="mt-1 text-caption text-muted-foreground">
          导出按模板渲染回写 Markdown，或从 Markdown 导入任务（草稿审阅后确认）。
        </p>
      </div>

      {/* Tab 分区（导出在前） */}
      <div className="mb-5 flex items-center gap-1 border-b border-divider">
        {tabs.map((t) => (
          <button
            key={t.key}
            type="button"
            onClick={() => setSection(t.key)}
            aria-pressed={section === t.key}
            className={cn(
              'relative -mb-px flex items-center gap-1.5 border-b-2 px-4 py-2 text-sm transition-colors',
              section === t.key
                ? 'border-primary-500 font-semibold text-primary-700'
                : 'border-transparent text-muted-foreground hover:text-foreground',
            )}
          >
            {t.key === 'export' ? <Download className="size-4" /> : <FileUp className="size-4" />}
            {t.label}
          </button>
        ))}
      </div>

      {section === 'export' ? (
        <ExportPanel />
      ) : (
        <div>
          <div className="mb-4">
            <Button variant="outline" onClick={() => setImportOpen(true)} aria-label="打开导入">
              <FileUp className="size-4" />
              导入 Markdown
            </Button>
          </div>
          <DraftsPanel onReview={setReviewing} />

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
        </div>
      )}
    </div>
  )
}
