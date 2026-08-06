import { useState } from 'react'
import { toast } from 'sonner'
import { FileUp, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useImportTasks } from '@/hooks/useImports'

/**
 * 导入对话框（TF-027）：Markdown 解析入口。
 * 四形态（ParseInput）：单文件 / 多文件（逗号分隔）/ 目录 / 原始内容粘贴。
 * 成功写入草稿（pending），由 DraftsPanel 确认/丢弃。
 */
export interface ImportDialogProps {
  onOpenChange: (open: boolean) => void
}

export function ImportDialog({ onOpenChange }: ImportDialogProps) {
  const importTasks = useImportTasks()
  const [mode, setMode] = useState<'path' | 'content'>('path')
  const [pathValue, setPathValue] = useState('')
  const [content, setContent] = useState('')
  const [sourceFile, setSourceFile] = useState('')

  const busy = importTasks.isPending

  const submit = () => {
    if (mode === 'path') {
      const paths = pathValue
        .split(',')
        .map((p) => p.trim())
        .filter(Boolean)
      if (paths.length === 0) return
      if (paths.length === 1 && paths[0].endsWith('/')) {
        // 目录
        importTasks.mutate(
          { directory: paths[0] },
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
      toast.info('请输入 Markdown 内容与源文件名')
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

  return (
    <div>
      <Tabs value={mode} onValueChange={(v) => setMode(v as 'path' | 'content')}>
        <TabsList>
          <TabsTrigger value="path">文件 / 目录</TabsTrigger>
          <TabsTrigger value="content">粘贴内容</TabsTrigger>
        </TabsList>
        <TabsContent value="path" className="pt-4">
          <Label htmlFor="import-path">路径（支持逗号分隔多文件；以 / 结尾视为目录递归扫描）</Label>
          <Input
            id="import-path"
            value={pathValue}
            onChange={(e) => setPathValue(e.target.value)}
            placeholder="/Users/you/projects/backlog/01-tasks.md, /Users/you/projects/backlog/02.md"
            className="mt-1.5"
          />
          <p className="mt-2 text-caption text-muted-foreground">
            解析结果先进入草稿，确认后才入库（文件级全量覆盖）。
          </p>
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
        <Button
          onClick={submit}
          disabled={busy || (mode === 'path' ? !pathValue.trim() : !content.trim())}
        >
          {busy ? <Loader2 className="size-4 animate-spin" /> : <FileUp className="size-4" />}
          {busy ? '解析中（LLM）…' : '开始解析'}
        </Button>
      </div>
    </div>
  )
}
