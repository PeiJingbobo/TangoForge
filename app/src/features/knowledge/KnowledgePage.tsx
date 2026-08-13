import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { FileText, Library, Plus, RefreshCw, ScanSearch, Search, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  useCreateKnowledgeBase,
  useDeleteKnowledgeBase,
  useKnowledgeBases,
  useKnowledgeDocuments,
  useKnowledgeScan,
  useKnowledgeSearch,
} from '@/hooks/useKnowledge'
import { useProjectId } from '@/hooks/useProject'
import { cn } from '@/lib/utils'
import type { KnowledgeBase, KnowledgeDocument } from '@/types/models'
import { KnowledgeDocumentDrawer } from '@/features/knowledge/KnowledgeDocumentDrawer'
import { KnowledgeAddDialog } from '@/features/knowledge/KnowledgeAddDialog'

/**
 * 知识库页（/project/:id/knowledge，TF-052，docs/KNOWLEDGE-BASE.md §10.2）：
 * 左侧库列表（CRUD + 默认库徽标）+ 中间文档列表（路径/类型/状态/摘要/关联数，
 * 筛库/筛状态/搜索）+ 右上扫描按钮 + 检索视图 + 文档阅读/编辑抽屉。
 */
export function KnowledgePage() {
  const pid = useProjectId()
  const { data: bases, isLoading: basesLoading } = useKnowledgeBases(pid)
  const { data: docList, isLoading: docsLoading } = useKnowledgeDocuments(undefined, pid)
  const [selectedKB, setSelectedKB] = useState<number | null>(null)
  const [statusFilter, setStatusFilter] = useState('')
  const [q, setQ] = useState('')
  const [searchQ, setSearchQ] = useState('')
  const [docId, setDocId] = useState<string | null>(null)
  const [addOpen, setAddOpen] = useState(false)
  const [newKBName, setNewKBName] = useState('')
  const createBase = useCreateKnowledgeBase(pid)
  const deleteBase = useDeleteKnowledgeBase(pid)
  const scan = useKnowledgeScan(pid)
  const { data: searchRes, isFetching: searching } = useKnowledgeSearch(searchQ, { top_k: 10 }, pid)

  const docs = useMemo(() => {
    const list = docList?.items ?? []
    return list.filter(
      (d) =>
        (!selectedKB || d.kb_ids?.includes(selectedKB)) &&
        (!statusFilter || d.status === statusFilter) &&
        (!q ||
          d.display_name.toLowerCase().includes(q.toLowerCase()) ||
          d.path.toLowerCase().includes(q.toLowerCase())),
    )
  }, [docList, selectedKB, statusFilter, q])

  const createBaseSubmit = () => {
    const name = newKBName.trim()
    if (!name) return
    createBase.mutate(
      { name },
      {
        onSuccess: (kb) => {
          toast.success('已创建知识库', { description: kb.name })
          setNewKBName('')
        },
        onError: (e) => toast.error(e instanceof Error ? e.message : '创建失败'),
      },
    )
  }

  const removeBase = (kb: KnowledgeBase) => {
    if (kb.is_default) return
    if (!window.confirm(`删除知识库「${kb.name}」？（仅移除库内文档引用，文档与任务关联保留）`))
      return
    deleteBase.mutate(kb.id, {
      onSuccess: () => {
        toast.success('已删除知识库')
        if (selectedKB === kb.id) setSelectedKB(null)
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : '删除失败'),
    })
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="shrink-0">
        <h1 className="text-h2 text-foreground">知识库</h1>
        <p className="mt-1 text-caption text-muted-foreground">
          文档引用注册表 + 任务关联 + 语义索引（摘要/向量）；原文以磁盘为唯一真实源。
        </p>
      </div>

      <div className="mt-4 grid min-h-0 flex-1 gap-4 lg:grid-cols-[240px_1fr]">
        {/* 左：库列表 */}
        <aside className="flex min-h-0 flex-col overflow-y-auto rounded-2xl border border-border bg-card p-3">
          <div className="mb-2 flex items-center gap-1.5 text-sm font-medium">
            <Library className="size-4 text-muted-foreground" />
            知识库
          </div>
          {basesLoading ? (
            <Skeleton className="h-24 w-full" />
          ) : (
            <div className="space-y-1">
              {(bases ?? []).map((kb) => (
                <button
                  key={kb.id}
                  type="button"
                  onClick={() => setSelectedKB(selectedKB === kb.id ? null : kb.id)}
                  className={cn(
                    'flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm transition-colors',
                    selectedKB === kb.id ? 'bg-primary-50 text-primary-700' : 'hover:bg-accent',
                  )}
                >
                  <span className="min-w-0 flex-1 truncate">{kb.name}</span>
                  {kb.is_default && (
                    <span className="rounded bg-primary-50 px-1 text-[10px] text-primary-700">
                      默认
                    </span>
                  )}
                  <span className="text-[10px] text-muted-foreground">{kb.doc_count ?? 0}</span>
                  {!kb.is_default && (
                    <Trash2
                      className="size-3 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 hover:text-destructive-ink"
                      onClick={(e) => {
                        e.stopPropagation()
                        removeBase(kb)
                      }}
                    />
                  )}
                </button>
              ))}
            </div>
          )}
          <div className="mt-3 flex gap-1.5 border-t border-divider pt-3">
            <Input
              value={newKBName}
              onChange={(e) => setNewKBName(e.target.value)}
              placeholder="新库名"
              className="h-8 text-xs"
              onKeyDown={(e) => {
                if (e.key === 'Enter') createBaseSubmit()
              }}
            />
            <Button
              size="sm"
              className="h-8 shrink-0 px-2"
              onClick={createBaseSubmit}
              disabled={!newKBName.trim()}
            >
              <Plus className="size-3.5" />
            </Button>
          </div>
        </aside>

        {/* 右：文档列表 + 检索 */}
        <main className="flex min-h-0 flex-col gap-4">
          {/* 工具行：状态 → 筛选 →（空）→ 扫描 → 添加文件（右侧对齐） */}
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
              className="h-8 rounded-lg border border-border bg-background px-2 text-xs"
            >
              <option value="">全部状态</option>
              <option value="ok">正常</option>
              <option value="missing">缺失</option>
              <option value="failed">失败</option>
              <option value="indexing">索引中</option>
            </select>
            <div className="relative w-52">
              <Search className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder="筛选文档"
                className="h-8 pl-8 text-xs"
              />
            </div>
            {/* 弹性空位：将右侧操作推到最右 */}
            <div className="flex-1" />
            <Button
              variant="outline"
              size="sm"
              className="h-8 gap-1.5 text-xs"
              onClick={() =>
                scan.mutate(undefined, {
                  onSuccess: (s) =>
                    toast.success('扫描完成', {
                      description: `索引 ${s.indexed} · 跳过 ${s.skipped}`,
                    }),
                  onError: (e) => toast.error(e instanceof Error ? e.message : '扫描失败'),
                })
              }
              disabled={scan.isPending}
            >
              <RefreshCw className={cn('size-3.5', scan.isPending && 'animate-spin')} />
              扫描
            </Button>
            <Button size="sm" className="h-8 gap-1.5 text-xs" onClick={() => setAddOpen(true)}>
              <Plus className="size-3.5" />
              添加文件
            </Button>
          </div>

          {/* 检索框 */}
          <div className="relative shrink-0">
            <ScanSearch className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={searchQ}
              onChange={(e) => setSearchQ(e.target.value)}
              placeholder="向量语义检索（输入关键词查询；需配置 llm.embedding）"
              className="pl-9"
            />
          </div>

          {/* 检索结果（有查询时优先展示） */}
          {searchQ.trim() && (
            <div className="min-h-0 flex-1 overflow-y-auto rounded-2xl border border-border bg-card p-3">
              {searching ? (
                <Skeleton className="h-20 w-full" />
              ) : searchRes?.items.length ? (
                <div className="space-y-2">
                  {searchRes.items.map((hit) => (
                    <button
                      key={hit.document.id}
                      type="button"
                      onClick={() => setDocId(hit.document.id)}
                      className="flex w-full flex-col gap-1 rounded-xl border border-divider px-3 py-2 text-left transition-colors hover:bg-accent"
                    >
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium">{hit.document.display_name}</span>
                        <span className="rounded bg-primary-50 px-1 text-[10px] text-primary-700">
                          {(hit.score * 100).toFixed(0)}%
                        </span>
                        {hit.missing && (
                          <span className="rounded bg-warning-soft px-1 text-[10px] text-warning-ink">
                            缺失
                          </span>
                        )}
                      </div>
                      {hit.chunks.slice(0, 2).map((c, i) => (
                        <p key={i} className="line-clamp-2 text-xs text-muted-foreground">
                          {c.heading ? `[${c.heading}] ` : ''}
                          {c.text}
                        </p>
                      ))}
                    </button>
                  ))}
                </div>
              ) : (
                <p className="py-4 text-center text-xs text-muted-foreground">
                  {searchRes === undefined ? '检索中…' : '无匹配结果'}
                </p>
              )}
            </div>
          )}

          {/* 文档列表（无检索时） */}
          {!searchQ.trim() && (
            <div className="min-h-0 flex-1 overflow-y-auto">
              {docsLoading ? (
                <div className="space-y-2">
                  <Skeleton className="h-14 w-full" />
                  <Skeleton className="h-14 w-full" />
                </div>
              ) : docs.length === 0 ? (
                <div className="grid place-items-center py-16 text-sm text-muted-foreground">
                  暂无文档 — 在任务详情「添加资料」或下方注册
                </div>
              ) : (
                <div className="space-y-2">
                  {docs.map((d) => (
                    <DocCard key={d.id} doc={d} onClick={() => setDocId(d.id)} />
                  ))}
                </div>
              )}
            </div>
          )}
        </main>
      </div>

      {docId && (
        <KnowledgeDocumentDrawer
          docId={docId}
          open={!!docId}
          onOpenChange={(o) => {
            if (!o) setDocId(null)
          }}
        />
      )}

      {addOpen && (
        <KnowledgeAddDialog open={addOpen} onOpenChange={setAddOpen} project={pid ?? ''} />
      )}
    </div>
  )
}

function DocCard({ doc, onClick }: { doc: KnowledgeDocument; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-center gap-3 rounded-xl border border-divider px-3 py-2.5 text-left transition-colors hover:bg-accent"
    >
      <FileText className="size-4 shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate text-sm font-medium">{doc.display_name}</span>
          <StatusBadge status={doc.status} embedded={doc.embedded} />
        </div>
        <div className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground">
          {doc.path}
        </div>
        {doc.summary && (
          <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{doc.summary}</p>
        )}
      </div>
      {doc.task_count ? (
        <span className="shrink-0 text-[10px] text-muted-foreground">{doc.task_count} 任务</span>
      ) : null}
    </button>
  )
}

export function StatusBadge({ status, embedded }: { status: string; embedded: number }) {
  return (
    <>
      {status === 'missing' && (
        <span className="rounded bg-warning-soft px-1 text-[10px] text-warning-ink">缺失</span>
      )}
      {status === 'failed' && (
        <span className="rounded bg-destructive-soft px-1 text-[10px] text-destructive-ink">
          失败
        </span>
      )}
      {status === 'indexing' && (
        <span className="rounded bg-muted px-1 text-[10px] text-muted-foreground">索引中</span>
      )}
      {status === 'ok' && embedded === 1 && (
        <span className="rounded bg-success-soft px-1 text-[10px] text-success-ink">已嵌入</span>
      )}
      {status === 'ok' && embedded === 0 && (
        <span className="rounded bg-muted px-1 text-[10px] text-muted-foreground">未嵌入</span>
      )}
      {status === 'ok' && embedded === 2 && (
        <span className="rounded bg-destructive-soft px-1 text-[10px] text-destructive-ink">
          嵌入失败
        </span>
      )}
    </>
  )
}
