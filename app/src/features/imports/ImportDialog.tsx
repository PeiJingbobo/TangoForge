import { useState } from 'react'
import { toast } from 'sonner'
import { FileUp, FolderOpen, Loader2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useImportTasks } from '@/hooks/useImports'

/**
 * 导入对话框（TF-027）：Markdown 解析入口。
 * 四形态（ParseInput）：单文件 / 多文件（逗号分隔）/ 目录 / 原始内容粘贴。
 * Electron 环境：系统文件/目录选择器（dialog.selectFiles/selectDirectory）；
 * Web 环境：手动输入路径兜底。成功写入草稿（pending），由 DraftsPanel 确认/丢弃。
 */
export interface ImportDialogProps {
  onOpenChange: (open: boolean) => void
}

export function ImportDialog({ onOpenChange }: ImportDialogProps) {
  const importTasks = useImportTasks()
  const [mode, setMode] = useState<'path' | 'content'>('path')
  const [paths, setPaths] = useState<string[]>([])
  const [dir, setDir] = useState<string | null>(null)
  const [content, setContent] = useState('')
  const [sourceFile, setSourceFile] = useState('')

  const busy = importTasks.isPending
  const hasDialog = Boolean(window.tangoforge?.dialog)

  const pickFiles = async () => {
    if (!window.tangoforge?.dialog) {
      toast.info('当前为非桌面环境，请手动输入路径')
      return
    }
    const files = await window.tangoforge.dialog.selectFiles()
    if (files && files.length > 0) setPaths((prev) => [...new Set([...prev, ...files])])
  }

  const pickDirectory = async () => {
    if (!window.tangoforge?.dialog) {
      toast.info('当前为非桌面环境，请手动输入路径')
      return
    }
    const d = await window.tangoforge.dialog.selectDirectory()
    if (d) setDir(d)
  }

  const submit = () => {
    if (dir) {
      importTasks.mutate(
        { directory: dir },
        {
          onSuccess: (d) => {
            toast.success('解析完成，草稿已就绪', {
              description: `${d.source_file} · ${d.task_count} 个任务`,
            })
            onOpenChange(false)
          },
          onError: (e) => toast.error(e instanceof Error ? e.message : '解析失败'),
        },
      )
      return
    }
    if (paths.length > 0) {
      importTasks.mutate(
        { file_paths: paths },
        {
          onSuccess: (d) => {
            toast.success('解析完成，草稿已就绪', {
              description: `${d.source_file} · ${d.task_count} 个任务`,
            })
            onOpenChange(false)
          },
          onError: (e) => toast.error(e instanceof Error ? e.message : '解析失败'),
        },
      )
      return
    }
    // 内容粘贴
    if (!content.trim() || !sourceFile.trim()) {
      toast.info('请选择文件/目录，或输入 Markdown 内容与源文件名')
      return
    }
    importTasks.mutate(
      { content, source_file: sourceFile.trim() },
      {
        onSuccess: (d) => {
          toast.success('解析完成，草稿已就绪', {
            description: `${d.source_file} · ${d.task_count} 个任务`,
          })
          onOpenChange(false)
        },
        onError: (e) => toast.error(e instanceof Error ? e.message : '解析失败'),
      },
    )
  }

  const canSubmit = busy || (dir === null && paths.length === 0 && !content.trim())

  return (
    <div>
      <Tabs value={mode} onValueChange={(v) => setMode(v as 'path' | 'content')}>
        <TabsList>
          <TabsTrigger value="path">文件 / 目录</TabsTrigger>
          <TabsTrigger value="content">粘贴内容</TabsTrigger>
        </TabsList>
        <TabsContent value="path" className="space-y-3 pt-4">
          {hasDialog ? (
            <>
              <div className="flex flex-wrap gap-2">
                <Button
                  variant="outline"
                  onClick={() => void pickFiles()}
                  disabled={busy}
                  aria-label="选择 Markdown 文件"
                >
                  <FileUp className="size-4" />
                  选择文件
                </Button>
                <Button
                  variant="outline"
                  onClick={() => void pickDirectory()}
                  disabled={busy}
                  aria-label="选择目录"
                >
                  <FolderOpen className="size-4" />
                  选择目录
                </Button>
              </div>
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
              <p className="text-caption text-muted-foreground">
                支持多选 Markdown 文件，或选择目录递归扫描；解析结果先进入草稿，确认后入库。
              </p>
            </>
          ) : (
            <>
              <Label htmlFor="import-path">路径（逗号分隔多文件；以 / 结尾视为目录递归扫描）</Label>
              <Input
                id="import-path"
                value={paths.join(',')}
                onChange={(e) =>
                  setPaths(
                    e.target.value
                      .split(',')
                      .map((p) => p.trim())
                      .filter(Boolean),
                  )
                }
                placeholder="/Users/you/projects/backlog/01-tasks.md, /Users/you/projects/backlog/02.md"
                className="mt-1.5"
              />
              <p className="text-caption text-muted-foreground">
                桌面环境下可通过「选择文件 / 选择目录」直接挑选。
              </p>
            </>
          )}
        </TabsContent>
        <TabsContent value="content" className="space-y-3 pt-4">
          <div>
            <Label htmlFor="import-source">源文件名</Label>
            <Input
              id="import-source"
              value={sourceFile}
              onChange={(e) => setSourceFile(e.target.value)}
              placeholder="backlog.md"
              className="mt-1.5"
            />
          </div>
          <div>
            <Label htmlFor="import-content">Markdown 内容</Label>
            <textarea
              id="import-content"
              value={content}
              onChange={(e) => setContent(e.target.value)}
              rows={8}
              placeholder="# 任务标题\n\n- [ ] 子任务…"
              className="mt-1.5 w-full rounded-[14px] border border-input bg-card px-3.5 py-2.5 text-sm outline-none placeholder:text-muted-foreground focus:border-primary-400 focus:ring-[3px] focus:ring-primary-100"
            />
          </div>
        </TabsContent>
      </Tabs>

      <div className="mt-5 flex justify-end gap-2">
        <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={busy}>
          取消
        </Button>
        <Button onClick={submit} disabled={canSubmit}>
          {busy ? <Loader2 className="size-4 animate-spin" /> : <FileUp className="size-4" />}
          {busy ? '解析中（LLM）…' : '开始解析'}
        </Button>
      </div>
    </div>
  )
}
