import { useState } from 'react'
import { toast } from 'sonner'
import { Download, Loader2, Sparkles } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Separator } from '@/components/ui/separator'
import { useExportMarkdown, useGenerateTemplate } from '@/hooks/useExports'
import type { RenderResult } from '@/types/models'

/**
 * 导出对话框（TF-027）：
 * 模板模式（default/llm）+ 目标（overwrite/copy）+ 渲染预览 + 执行（已写盘）。
 * LLM 生成模板入口（示例文档 → 生成 .tmpl 并更新项目配置）。
 */
export interface ExportDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ExportDialog({ open, onOpenChange }: ExportDialogProps) {
  const exportMarkdown = useExportMarkdown()
  const generateTemplate = useGenerateTemplate()
  const [templateMode, setTemplateMode] = useState<'default' | 'llm'>('default')
  const [target, setTarget] = useState<'overwrite' | 'copy'>('copy')
  const [path, setPath] = useState('')
  const [result, setResult] = useState<RenderResult | null>(null)
  const [showTemplateGen, setShowTemplateGen] = useState(false)
  const [example, setExample] = useState('')

  const busy = exportMarkdown.isPending || generateTemplate.isPending

  const run = () => {
    if (target === 'overwrite' && !path.trim()) {
      toast.info('overwrite 模式需要填写目标路径')
      return
    }
    exportMarkdown.mutate(
      {
        template_mode: templateMode,
        target,
        ...(target === 'overwrite' ? { path: path.trim() } : {}),
      },
      {
        onSuccess: (r) => {
          setResult(r)
          toast.success('导出完成', { description: r.path })
        },
        onError: (e) => toast.error(e instanceof Error ? e.message : '导出失败'),
      },
    )
  }

  const genTemplate = () => {
    if (!example.trim()) {
      toast.info('请粘贴示例文档')
      return
    }
    generateTemplate.mutate(example, {
      onSuccess: (r) => {
        toast.success('模板已生成并应用', { description: r.path })
        setShowTemplateGen(false)
        setTemplateMode('llm')
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : '模板生成失败'),
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>导出 Markdown</DialogTitle>
          <DialogDescription>按项目导出模板渲染，支持 default / LLM 生成模板。</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="flex flex-wrap gap-2">
            <Badge
              variant={templateMode === 'default' ? 'default' : 'outline'}
              role="button"
              className="cursor-pointer px-3 py-1.5"
              onClick={() => setTemplateMode('default')}
            >
              默认模板
            </Badge>
            <Badge
              variant={templateMode === 'llm' ? 'default' : 'outline'}
              role="button"
              className="cursor-pointer px-3 py-1.5"
              onClick={() => setTemplateMode('llm')}
            >
              LLM 模板
            </Badge>
            <Badge
              variant={target === 'copy' ? 'default' : 'outline'}
              role="button"
              className="cursor-pointer px-3 py-1.5"
              onClick={() => setTarget('copy')}
            >
              另存（.taskboard/export.md）
            </Badge>
            <Badge
              variant={target === 'overwrite' ? 'default' : 'outline'}
              role="button"
              className="cursor-pointer px-3 py-1.5"
              onClick={() => setTarget('overwrite')}
            >
              覆盖指定文件
            </Badge>
          </div>

          {target === 'overwrite' && (
            <div>
              <Label htmlFor="export-path">目标路径</Label>
              <Input
                id="export-path"
                value={path}
                onChange={(e) => setPath(e.target.value)}
                placeholder="/Users/you/projects/backlog/backlog.md"
                className="mt-1.5"
              />
            </div>
          )}

          {result && (
            <>
              <Separator />
              <div className="rounded-xl border border-divider bg-muted/60 p-3">
                <div className="mb-2 flex items-center justify-between">
                  <span className="text-sm font-semibold">预览</span>
                  <span className="truncate font-mono text-caption text-muted-foreground">
                    {result.path}
                  </span>
                </div>
                <pre className="max-h-56 overflow-auto rounded-lg bg-card p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap">
                  {result.content}
                </pre>
              </div>
            </>
          )}

          {showTemplateGen ? (
            <div className="space-y-2 rounded-xl border border-primary-100 bg-primary-50/60 p-4">
              <Label htmlFor="template-example">示例文档（LLM 将生成贴近此风格的模板）</Label>
              <textarea
                id="template-example"
                value={example}
                onChange={(e) => setExample(e.target.value)}
                rows={5}
                placeholder="# 项目标题&#10;…"
                className="w-full rounded-lg border border-input bg-card px-3 py-2 text-sm outline-none focus:border-primary-400"
              />
              <div className="flex gap-2">
                <Button size="sm" onClick={genTemplate} disabled={busy}>
                  {busy ? (
                    <Loader2 className="size-3.5 animate-spin" />
                  ) : (
                    <Sparkles className="size-3.5" />
                  )}
                  生成模板
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setShowTemplateGen(false)}>
                  取消
                </Button>
              </div>
            </div>
          ) : (
            <Button
              variant="ghost"
              size="sm"
              className="text-primary-600"
              onClick={() => setShowTemplateGen(true)}
            >
              <Sparkles className="size-3.5" /> 用 LLM 生成模板
            </Button>
          )}
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={busy}>
            关闭
          </Button>
          <Button onClick={run} disabled={busy}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : <Download className="size-4" />}
            导出并写盘
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
