import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ArrowLeft, Link2, Pencil, X } from 'lucide-react'
import { ApiError } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  useEditKnowledgeContent,
  useKnowledgeContent,
  useKnowledgeDocument,
  useRelinkKnowledgeDocument,
} from '@/hooks/useKnowledge'
import { useProjectId } from '@/hooks/useProject'
import { getTitleBarHeight } from '@/lib/window-chrome'
import { StatusBadge } from '@/features/knowledge/KnowledgePage'

/**
 * 知识库文档抽屉（TF-052）：详情 + 真实路径 + 阅读/编辑原文 + relink。
 * - 阅读：GET /documents/:id/content（二进制仅路径）；
 * - 编辑：PUT content 直接写盘 → 触发重索引；
 * - relink：选新路径 + 拷贝选项（重置并重建索引）。
 */
export function KnowledgeDocumentDrawer({
  docId,
  open,
  onOpenChange,
}: {
  docId: string
  open: boolean
  onOpenChange: (o: boolean) => void
}) {
  const pid = useProjectId()
  const { data: doc, isLoading } = useKnowledgeDocument(docId || undefined, pid)
  const { data: content } = useKnowledgeContent(docId || undefined, pid)
  const editContent = useEditKnowledgeContent(pid)
  const relink = useRelinkKnowledgeDocument(pid)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const [relinkOpen, setRelinkOpen] = useState(false)
  const [newPath, setNewPath] = useState('')
  const [copy, setCopy] = useState('auto')
  const titleBarH = getTitleBarHeight()

  useEffect(() => {
    if (content?.content != null) setDraft(content.content)
  }, [content])

  const saveEdit = () => {
    editContent.mutate(
      { id: docId, content: draft },
      {
        onSuccess: () => {
          toast.success('已保存（触发重新索引）')
          setEditing(false)
        },
        onError: (e) => toast.error(e instanceof Error ? e.message : '保存失败'),
      },
    )
  }

  const doRelink = () => {
    if (!newPath.trim()) return
    relink.mutate(
      { id: docId, new_path: newPath.trim(), copy },
      {
        onSuccess: () => {
          toast.success('已重新链接（重建索引）')
          setRelinkOpen(false)
          setNewPath('')
        },
        onError: (e) => {
          const msg = e instanceof ApiError ? e.message : 'relink 失败'
          toast.error(msg)
        },
      },
    )
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        zIndex={50}
        style={{ paddingTop: titleBarH }}
        showCloseButton={false}
        className="flex w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-xl"
      >
        <SheetHeader className="border-b border-divider px-6 py-4">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => onOpenChange(false)}
              aria-label="返回"
              className="grid size-8 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-accent"
            >
              <ArrowLeft className="size-4" />
            </button>
            <SheetTitle className="text-base">{doc?.display_name ?? '文档'}</SheetTitle>
            <button
              type="button"
              onClick={() => onOpenChange(false)}
              aria-label="关闭"
              className="ml-auto grid size-8 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-accent"
            >
              <X className="size-4" />
            </button>
          </div>
          <SheetDescription>
            {doc && (
              <span className="flex flex-wrap items-center gap-1.5">
                <StatusBadge status={doc.status} embedded={doc.embedded} />
                {doc.type === 'binary' && <span className="text-xs">二进制</span>}
              </span>
            )}
          </SheetDescription>
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-4">
          {isLoading || !doc ? (
            <Skeleton className="h-40 w-full" />
          ) : (
            <div className="space-y-4 text-sm">
              <div className="rounded-xl border border-divider p-3">
                <div className="break-all font-mono text-xs text-muted-foreground">{doc.path}</div>
                <div className="mt-2 space-y-1 text-xs">
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">真实路径</span>
                    <span className="break-all text-right">{doc.abs_path}</span>
                  </div>
                  {doc.origin_path && (
                    <div className="flex justify-between gap-3">
                      <span className="text-muted-foreground">原始路径</span>
                      <span className="break-all text-right">{doc.origin_path}</span>
                    </div>
                  )}
                  {doc.embedding_model && (
                    <div className="flex justify-between gap-3">
                      <span className="text-muted-foreground">嵌入模型</span>
                      <span>{doc.embedding_model}</span>
                    </div>
                  )}
                  {doc.summary && (
                    <div className="mt-2">
                      <span className="text-muted-foreground">摘要</span>
                      <p className="mt-0.5 text-xs">{doc.summary}</p>
                    </div>
                  )}
                  {doc.history.length > 0 && (
                    <div className="mt-2">
                      <span className="text-muted-foreground">历史路径</span>
                      <div className="mt-0.5 space-y-0.5 font-mono">
                        {doc.history.map((h, i) => (
                          <div key={i} className="text-[11px]">
                            {h.path}
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              </div>

              {/* 内容区 */}
              {doc.type === 'binary' ? (
                <p className="rounded-xl border border-divider p-4 text-xs text-muted-foreground">
                  二进制文件不提供预览（仅注册引用）。
                </p>
              ) : editing ? (
                <textarea
                  value={draft}
                  onChange={(e) => setDraft(e.target.value)}
                  className="min-h-64 w-full rounded-xl border border-divider bg-background p-3 font-mono text-xs outline-none focus:border-primary"
                />
              ) : (
                <pre className="max-h-96 overflow-y-auto whitespace-pre-wrap rounded-xl border border-divider bg-muted/50 p-4 font-mono text-xs">
                  {content?.content ?? '（加载中…）'}
                </pre>
              )}
            </div>
          )}
        </div>

        <SheetFooter className="border-t border-divider bg-muted/60 px-6 py-3 sm:flex-row sm:items-center sm:justify-between">
          <Button variant="ghost" size="sm" onClick={() => setRelinkOpen(true)}>
            <Link2 className="size-3.5" /> 重新链接
          </Button>
          <div className="flex items-center gap-2">
            {doc?.type !== 'binary' && !editing && (
              <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
                <Pencil className="size-3.5" /> 编辑
              </Button>
            )}
            {editing && (
              <>
                <Button variant="ghost" size="sm" onClick={() => setEditing(false)}>
                  取消
                </Button>
                <Button size="sm" onClick={saveEdit} disabled={editContent.isPending}>
                  {editContent.isPending ? '保存中…' : '保存'}
                </Button>
              </>
            )}
          </div>
        </SheetFooter>

        {relinkOpen && (
          <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/60 backdrop-blur-sm">
            <div className="w-80 rounded-2xl border border-border bg-card p-4 shadow-lg">
              <div className="text-sm font-medium">重新链接文档</div>
              <p className="mt-1 text-xs text-muted-foreground">
                指定文件的新磁盘路径（须为文本文件）；将重置并重建索引，历史路径保留。
              </p>
              <Input
                value={newPath}
                onChange={(e) => setNewPath(e.target.value)}
                placeholder="新路径（绝对或相对 workdir）"
                className="mt-3 font-mono text-xs"
              />
              <select
                value={copy}
                onChange={(e) => setCopy(e.target.value)}
                className="mt-2 h-8 w-full rounded-lg border border-border bg-background px-2 text-xs"
              >
                <option value="auto">auto（项目外自动拷贝）</option>
                <option value="copy">copy（一律拷贝）</option>
                <option value="none">none（原样引用）</option>
              </select>
              <div className="mt-3 flex justify-end gap-2">
                <Button variant="ghost" size="sm" onClick={() => setRelinkOpen(false)}>
                  取消
                </Button>
                <Button size="sm" onClick={doRelink} disabled={!newPath.trim() || relink.isPending}>
                  {relink.isPending ? '处理中…' : '重新链接'}
                </Button>
              </div>
            </div>
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}
