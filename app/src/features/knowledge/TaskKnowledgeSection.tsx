import { useState } from 'react'
import { toast } from 'sonner'
import { BookOpen, FileText, Link2, Search, Trash2 } from 'lucide-react'
import { ApiError } from '@/api/client'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  useLinkTaskDocument,
  useTaskDocuments,
  useUnlinkTaskDocument,
  useKnowledgeSearch,
} from '@/hooks/useKnowledge'
import type { KnowledgeDocument } from '@/types/models'

/**
 * 任务详情「资料」区（TF-052，docs/KNOWLEDGE-BASE.md §10.1）：
 * 关联文档列表（点击阅读、移除关联）+「添加资料」（搜索既有文档或填路径）。
 * 依赖 /api/tasks/:id 内嵌 knowledge_documents 摘要（task.knowledge_documents）。
 */
export function TaskKnowledgeSection({
  taskId,
  documents,
  project,
}: {
  taskId: string
  /** 任务详情内嵌的文档摘要数组（GET /api/tasks/:id） */
  documents?: TaskKnowledgeDoc[]
  project: string
}) {
  const { data: docs } = useTaskDocuments(taskId, project)
  const list = docs ?? documents?.map((d) => d as unknown as KnowledgeDocument) ?? []
  const [addOpen, setAddOpen] = useState(false)

  return (
    <div className="rounded-xl border border-divider p-4">
      <div className="flex items-center gap-2">
        <BookOpen className="size-4 text-muted-foreground" />
        <span className="text-sm font-medium">资料</span>
        <span className="text-xs text-muted-foreground">({list.length})</span>
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto gap-1.5 text-xs"
          onClick={() => setAddOpen(true)}
        >
          <Link2 className="size-3.5" /> 添加资料
        </Button>
      </div>
      <div className="mt-2 space-y-1.5">
        {list.length === 0 ? (
          <p className="text-xs text-muted-foreground">暂无关联文档</p>
        ) : (
          list.map((d) => <DocRow key={d.id} doc={d} taskId={taskId} project={project} />)
        )}
      </div>
      {addOpen && (
        <AddKnowledgeDialog
          taskId={taskId}
          project={project}
          open={addOpen}
          onOpenChange={setAddOpen}
        />
      )}
    </div>
  )
}

/** 任务详情内嵌摘要类型（/api/tasks/:id knowledge_documents） */
export interface TaskKnowledgeDoc {
  id: string
  display_name: string
  path: string
  abs_path: string
  rel_path: string
  type: 'text' | 'binary'
  status: string
  summary: string
}

function DocRow({
  doc,
  taskId,
  project,
}: {
  doc: KnowledgeDocument
  taskId: string
  project: string
}) {
  const unlink = useUnlinkTaskDocument(project)

  const remove = () => {
    if (!window.confirm(`解除与「${doc.display_name}」的关联？（文档本身保留）`)) return
    unlink.mutate(
      { task_id: taskId, document_id: doc.id },
      {
        onSuccess: () => toast.success('已解除关联'),
        onError: (e) => toast.error(e instanceof Error ? e.message : '解除失败'),
      },
    )
  }

  return (
    <div className="flex items-center gap-2 rounded-lg border border-divider px-3 py-2">
      <FileText className="size-4 shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate text-sm font-medium" title={doc.path}>
            {doc.display_name}
          </span>
          {doc.type === 'binary' && (
            <span className="rounded bg-muted px-1 text-[10px] text-muted-foreground">二进制</span>
          )}
        </div>
        <div className="truncate font-mono text-[11px] text-muted-foreground">{doc.path}</div>
      </div>
      <Button
        variant="ghost"
        size="icon"
        className="size-7"
        onClick={remove}
        disabled={unlink.isPending}
        aria-label={`解除关联 ${doc.display_name}`}
      >
        <Trash2 className="size-3.5 text-destructive-ink" />
      </Button>
    </div>
  )
}

/** 添加资料对话框：向量搜索既有文档 + 填路径注册。 */
function AddKnowledgeDialog({
  taskId,
  project,
  open,
  onOpenChange,
}: {
  taskId: string
  project: string
  open: boolean
  onOpenChange: (o: boolean) => void
}) {
  const [query, setQuery] = useState('')
  const [path, setPath] = useState('')
  const { data: searchRes, isFetching } = useKnowledgeSearch(query, { top_k: 8 }, project)
  const link = useLinkTaskDocument(project)

  const linkDoc = (documentId: string) => {
    link.mutate(
      { task_id: taskId, document_id: documentId },
      {
        onSuccess: () => {
          toast.success('已关联')
          onOpenChange(false)
        },
        onError: (e) => toast.error(e instanceof Error ? e.message : '关联失败'),
      },
    )
  }

  const linkByPath = () => {
    if (!path.trim()) return
    link.mutate(
      { task_id: taskId, path: path.trim() },
      {
        onSuccess: () => {
          toast.success('已注册并关联')
          setPath('')
          onOpenChange(false)
        },
        onError: (e) => {
          const msg = e instanceof ApiError ? e.message : '关联失败'
          toast.error(msg)
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>添加资料</DialogTitle>
          <DialogDescription>搜索知识库既有文档，或填写磁盘路径直接注册关联。</DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="输入关键词检索知识库文档（向量搜索）"
              className="pl-9"
            />
          </div>
          <div className="max-h-56 space-y-1 overflow-y-auto">
            {isFetching ? (
              <Skeleton className="h-16 w-full" />
            ) : searchRes?.items.length ? (
              searchRes.items.map((hit) => (
                <button
                  key={hit.document.id}
                  type="button"
                  onClick={() => linkDoc(hit.document.id)}
                  className="flex w-full items-center gap-2 rounded-lg border border-divider px-3 py-2 text-left transition-colors hover:bg-accent"
                >
                  <FileText className="size-4 shrink-0 text-muted-foreground" />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium">
                        {hit.document.display_name}
                      </span>
                      <span className="rounded bg-primary-50 px-1 text-[10px] text-primary-700">
                        {(hit.score * 100).toFixed(0)}%
                      </span>
                      {hit.missing && (
                        <span className="rounded bg-warning-soft px-1 text-[10px] text-warning-ink">
                          missing
                        </span>
                      )}
                    </div>
                    <div className="truncate font-mono text-[11px] text-muted-foreground">
                      {hit.document.path}
                    </div>
                  </div>
                  <Link2 className="size-3.5 shrink-0 text-muted-foreground" />
                </button>
              ))
            ) : (
              <p className="py-2 text-center text-xs text-muted-foreground">
                {query.trim()
                  ? '无匹配结果（或未配置向量搜索）'
                  : '输入关键词检索；也可下方直接填写路径'}
              </p>
            )}
          </div>

          <div className="flex items-center gap-2 border-t border-divider pt-3">
            <Input
              value={path}
              onChange={(e) => setPath(e.target.value)}
              placeholder="磁盘路径（如 docs/spec.md，未注册自动入库）"
              className="flex-1 font-mono text-xs"
            />
            <Button size="sm" onClick={linkByPath} disabled={!path.trim() || link.isPending}>
              关联
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
