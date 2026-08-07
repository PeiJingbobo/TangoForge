import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { CheckCircle2, Download, FolderOpen, History, FileText } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ExportDialog } from '@/features/tasks/ExportDialog'
import { useProjectId } from '@/hooks/useProject'
import {
  addExportRecord,
  getExportRecords,
  type ExportRecord,
} from '@/features/tasks/export-records'
import type { RenderResult } from '@/types/models'

/**
 * 导出面板（TF-039）：
 * - 「导出 Markdown」按钮 → ExportDialog（模板预览/生成 + 渲染）；
 * - 导出成功 → 窗口内完成提示（路径 + 打开目录/打开文件按钮）+
 *   localStorage 追加导出记录（时间/路径/模式/任务数）；
 * - 导出记录列表：每条提供打开目录/打开文件操作。
 */
export function ExportPanel() {
  const project = useProjectId()
  const [exportOpen, setExportOpen] = useState(false)
  const [lastResult, setLastResult] = useState<RenderResult | null>(null)
  const [records, setRecords] = useState<ExportRecord[]>([])

  const refresh = useCallback(() => {
    setRecords(project ? getExportRecords(project) : [])
  }, [project])

  // 项目切换/挂载时刷新记录。
  useEffect(() => {
    refresh()
  }, [refresh])

  const onExported = (r: RenderResult) => {
    setLastResult(r)
    if (project) {
      addExportRecord(project, { path: r.path, mode: 'default', taskCount: 0 })
    }
    refresh()
  }

  const reveal = (path: string) => {
    const shell = window.tangoforge?.shell
    if (!shell) {
      toast.error('「打开目录」仅桌面版可用（Web 预览不支持）')
      return
    }
    void shell.revealPath(path).then((ok) => {
      if (!ok) toast.error('打开目录失败')
    })
  }

  const open = (path: string) => {
    const shell = window.tangoforge?.shell
    if (!shell) {
      toast.error('「打开文件」仅桌面版可用（Web 预览不支持）')
      return
    }
    void shell.openPath(path).then((ok) => {
      if (!ok) toast.error('打开文件失败')
    })
  }

  const fmtTime = (iso: string) => {
    const d = new Date(iso)
    return d.toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  }

  return (
    <div className="space-y-5">
      {/* 操作区 */}
      <div className="flex items-center gap-2">
        <Button variant="outline" onClick={() => setExportOpen(true)} aria-label="打开导出">
          <Download className="size-4" />
          导出 Markdown
        </Button>
      </div>

      {/* 导出完成提示（窗口内，非仅 toast） */}
      {lastResult && (
        <div className="flex items-start gap-3 rounded-xl border border-success-soft bg-success-soft/60 p-4">
          <CheckCircle2 className="mt-0.5 size-5 shrink-0 text-success" />
          <div className="min-w-0 flex-1">
            <div className="text-sm font-semibold">导出完成</div>
            <div
              className="mt-1 truncate font-mono text-xs text-muted-foreground"
              title={lastResult.path}
            >
              {lastResult.path}
            </div>
            <div className="mt-2 flex gap-2">
              <Button size="sm" variant="outline" onClick={() => reveal(lastResult.path)}>
                <FolderOpen className="size-3.5" />
                打开目录
              </Button>
              <Button size="sm" variant="outline" onClick={() => open(lastResult.path)}>
                <FileText className="size-3.5" />
                打开文件
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* 导出记录 */}
      <div>
        <div className="mb-2 flex items-center gap-1.5 text-label uppercase tracking-wider text-muted-foreground">
          <History className="size-3.5" />
          导出记录
        </div>
        {records.length === 0 ? (
          <p className="rounded-xl border border-dashed border-border px-4 py-6 text-center text-xs text-muted-foreground">
            暂无导出记录，完成一次导出后显示在此处。
          </p>
        ) : (
          <ul className="flex flex-col gap-2">
            {records.map((rec) => (
              <li
                key={rec.id}
                className="flex items-center gap-3 rounded-xl border border-divider px-3 py-2.5"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-medium text-muted-foreground">
                      {fmtTime(rec.exportedAt)}
                    </span>
                    <span className="rounded-full bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                      {rec.mode === 'llm' ? 'LLM 模板' : '默认模板'}
                    </span>
                  </div>
                  <div className="mt-1 truncate font-mono text-xs" title={rec.path}>
                    {rec.path}
                  </div>
                </div>
                <div className="flex shrink-0 gap-1">
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={`打开目录 ${rec.path}`}
                    onClick={() => reveal(rec.path)}
                  >
                    <FolderOpen className="size-3.5" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={`打开文件 ${rec.path}`}
                    onClick={() => open(rec.path)}
                  >
                    <FileText className="size-3.5" />
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>

      <ExportDialog open={exportOpen} onOpenChange={setExportOpen} onExported={onExported} />
    </div>
  )
}
