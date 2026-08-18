import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import {
  Archive,
  FileText,
  Library,
  Loader2,
  Plus,
  RefreshCw,
  ScanSearch,
  Search,
  Trash2,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  useArchiveKnowledgeDocument,
  useCancelQueueTask,
  useCreateKnowledgeBase,
  useDeleteKnowledgeBase,
  useKnowledgeBases,
  useKnowledgeDocuments,
  useKnowledgeQueue,
  useKnowledgeScan,
  useKnowledgeSearch,
  useRegisterKnowledgeDocument,
  useRestoreKnowledgeDocument,
  useRetryQueueTask,
} from '@/hooks/useKnowledge'
import { useProjectId } from '@/hooks/useProject'
import { useEventInvalidator } from '@/hooks/useEvents'
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
  // TF-053 修复：接入 WS 事件失效（queue_updated / document_added 等无 knowledge. 前缀
  // 的事件），LLM/MCP 导入文档入队或嵌入状态变化时，文档列表与队列状态条实时刷新。
  useEventInvalidator(pid)
  const [selectedKB, setSelectedKB] = useState<number | null>(null)
  const [statusFilter, setStatusFilter] = useState('')
  const [q, setQ] = useState('')
  const [searchQ, setSearchQ] = useState('')
  const [docId, setDocId] = useState<string | null>(null)
  const [addOpen, setAddOpen] = useState(false)
  const [newKBName, setNewKBName] = useState('')
  // TF-052 归档视图：false = 默认列表（未归档）；true = 归档视图。
  const [showArchived, setShowArchived] = useState(false)
  const { data: bases, isLoading: basesLoading } = useKnowledgeBases(pid)
  // 选中库时用后端 filter[kb_id] 过滤（列表接口不返回 kb_ids，本地过滤会全空）。
  const { data: docList, isLoading: docsLoading } = useKnowledgeDocuments(
    { kb_id: selectedKB ?? undefined, archived: showArchived },
    pid,
  )
  const createBase = useCreateKnowledgeBase(pid)
  const deleteBase = useDeleteKnowledgeBase(pid)
  const scan = useKnowledgeScan(pid)
  const register = useRegisterKnowledgeDocument(pid)
  const archiveDoc = useArchiveKnowledgeDocument(pid)
  const restoreDoc = useRestoreKnowledgeDocument(pid)
  const queue = useKnowledgeQueue(pid)
  const cancelTask = useCancelQueueTask(pid)
  const retryTask = useRetryQueueTask(pid)
  const { data: searchRes, isFetching: searching } = useKnowledgeSearch(searchQ, { top_k: 10 }, pid)

  // TF-052：添加文件批处理状态（顶部进行中状态条；后端注册后入嵌入队列）。
  const [addBatch, setAddBatch] = useState<{
    total: number
    done: number
    failed: number
    current: string | null
  } | null>(null)

  const submitAdd = async (paths: string[], kbId: number, copy: string) => {
    const kbIds = kbId > 0 ? [kbId] : undefined
    setAddBatch({ total: paths.length, done: 0, failed: 0, current: paths[0] ?? null })
    // 顺序注册：同一 mutation 并发多次会覆盖 onSuccess（React Query 行为），
    // 改为逐个 await，每个完成后推进 current/进度。
    for (let i = 0; i < paths.length; i++) {
      const p = paths[i]
      setAddBatch((b) => (b ? { ...b, current: p } : b))
      try {
        await register.mutateAsync({ path: p, copy, kb_ids: kbIds })
        setAddBatch((b) => (b ? { ...b, done: b.done + 1 } : b))
      } catch (e) {
        setAddBatch((b) => (b ? { ...b, failed: b.failed + 1 } : b))
        toast.error(`「${p}」注册失败：${e instanceof Error ? e.message : '未知错误'}`)
      }
    }
    // 全部完成 → 批处理结束（嵌入由队列后台执行）。
    setAddBatch((b) => (b && b.done + b.failed >= b.total ? null : b))
  }

  // 队列活跃任务（全局嵌入状态展示）：排队中 + 进行中。
  const queuePending = queue.data?.pending ?? []
  const queueEmbedding = queue.data?.embedding ?? []
  const queueFailed = queue.data?.failed ?? []
  const queueHasActive = queuePending.length > 0 || queueEmbedding.length > 0
  const queueActiveCount = queuePending.length + queueEmbedding.length

  const docs = useMemo(() => {
    const list = docList?.items ?? []
    // kb 过滤由后端 filter[kb_id] 完成；此处仅做状态/关键词本地过滤。
    return list.filter(
      (d) =>
        (!statusFilter || d.status === statusFilter) &&
        (!q ||
          d.display_name.toLowerCase().includes(q.toLowerCase()) ||
          d.path.toLowerCase().includes(q.toLowerCase())),
    )
  }, [docList, statusFilter, q])

  // TF-052：持续无进展超时提示（默认 5 分钟）。
  const [stalled, setStalled] = useState(false)
  useEffect(() => {
    // 有进行中任务（注册批处理 或 队列活跃）时启动 5 分钟超时；任务完成/取消后重置。
    const working = (addBatch && addBatch.done + addBatch.failed < addBatch.total) || queueHasActive
    if (!working) {
      setStalled(false)
      return
    }
    const t = setTimeout(() => setStalled(true), 5 * 60 * 1000)
    return () => clearTimeout(t)
  }, [addBatch, queueHasActive])

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

      {/* TF-052：嵌入任务状态条（队列视图：排队/进行中/失败重试/取消） */}
      {(addBatch && addBatch.done + addBatch.failed < addBatch.total) ||
      queueHasActive ||
      queueFailed.length > 0 ? (
        <div
          className={`mt-3 shrink-0 rounded-xl border px-3 py-2 text-xs ${
            stalled
              ? 'border-warning-200 bg-warning-soft text-warning-ink'
              : 'border-divider bg-muted/40 text-muted-foreground'
          }`}
        >
          <div className="flex items-center gap-2">
            {queueHasActive || (addBatch && addBatch.done + addBatch.failed < addBatch.total) ? (
              <Loader2 className="size-3.5 shrink-0 animate-spin text-primary-700" />
            ) : (
              <span className="size-3.5 shrink-0 rounded-full bg-warning-soft" />
            )}
            <span>
              {addBatch && addBatch.done + addBatch.failed < addBatch.total
                ? `正在添加文件 ${addBatch.done + addBatch.failed}/${addBatch.total}${
                    addBatch.failed > 0 ? `（${addBatch.failed} 失败）` : ''
                  }`
                : ''}
              {addBatch && addBatch.done + addBatch.failed < addBatch.total && queueActiveCount > 0
                ? ' · '
                : ''}
              {queueActiveCount > 0
                ? `嵌入任务 ${queuePending.length} 排队 · ${queueEmbedding.length} 进行中`
                : ''}
              {stalled ? '（已超时，可能阻塞）' : ''}
            </span>
            {queueFailed.length > 0 && (
              <span className="text-destructive-ink">{queueFailed.length} 失败</span>
            )}
            <span className="ml-auto text-[10px]">状态自动刷新</span>
          </div>

          {/* 当前处理中的文件（进行中任务） */}
          {queueEmbedding.length > 0 && (
            <div
              className="mt-1 truncate pl-5 font-mono text-[11px]"
              title={queueEmbedding[0].path}
            >
              正在嵌入：{queueEmbedding[0].display_name}（{queueEmbedding[0].path}）
            </div>
          )}
          {/* 排队任务（前 3 个） */}
          {queuePending.length > 0 && (
            <div className="mt-1 truncate pl-5 font-mono text-[11px]">
              排队中：
              {queuePending
                .slice(0, 3)
                .map((t) => t.display_name)
                .join('、')}
              {queuePending.length > 3 ? ` 等 ${queuePending.length} 个` : ''}
            </div>
          )}
          {/* 失败任务：可重试/取消 */}
          {queueFailed.map((t) => (
            <div key={t.doc_id} className="mt-1 flex items-center gap-2 pl-5">
              <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-destructive-ink">
                {t.display_name}：{t.error ?? '失败'}
              </span>
              <button
                type="button"
                className="shrink-0 text-[11px] text-primary-700 hover:underline"
                onClick={() =>
                  retryTask.mutate(t.doc_id, {
                    onSuccess: () => toast.success(`已重新入队「${t.display_name}」`),
                    onError: (e) => toast.error(e instanceof Error ? e.message : '重试失败'),
                  })
                }
                disabled={retryTask.isPending}
              >
                重试
              </button>
              <button
                type="button"
                className="shrink-0 text-[11px] text-muted-foreground hover:underline"
                onClick={() =>
                  cancelTask.mutate(t.doc_id, {
                    onError: (e) => toast.error(e instanceof Error ? e.message : '取消失败'),
                  })
                }
              >
                清除
              </button>
            </div>
          ))}
          {/* 取消按钮（活跃任务） */}
          {queueActiveCount > 0 && (
            <div className="mt-1 pl-5">
              <button
                type="button"
                className="text-[11px] text-muted-foreground hover:underline"
                onClick={() => {
                  const target = queueEmbedding[0] ?? queuePending[0]
                  if (target)
                    cancelTask.mutate(target.doc_id, {
                      onSuccess: () => toast.success('已取消嵌入任务'),
                    })
                }}
                disabled={cancelTask.isPending}
              >
                取消全部嵌入
              </button>
            </div>
          )}
          {stalled && (
            <div className="mt-1 pl-5 text-[11px]">
              超过 5 分钟未完成，请检查守护进程 / embedding 配置（Ollama 是否运行、模型是否可用）。
            </div>
          )}
        </div>
      ) : null}

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
            <Button
              variant={showArchived ? 'default' : 'outline'}
              size="sm"
              className="h-8 gap-1.5 text-xs"
              onClick={() => setShowArchived((v) => !v)}
            >
              <Archive className="size-3.5" />
              {showArchived ? '返回文档' : '归档'}
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
                  {showArchived ? '暂无归档文档' : '暂无文档 — 在任务详情「添加资料」或下方注册'}
                </div>
              ) : (
                <div className="space-y-2">
                  {docs.map((d) => (
                    <DocCard
                      key={d.id}
                      doc={d}
                      showArchived={showArchived}
                      onClick={() => setDocId(d.id)}
                      onArchive={(id) =>
                        archiveDoc.mutate(id, {
                          onSuccess: () =>
                            toast.success('已归档（任务引用与文件保留，可在「归档」视图还原）'),
                          onError: (e) => toast.error(e instanceof Error ? e.message : '归档失败'),
                        })
                      }
                      onRestore={(id) =>
                        restoreDoc.mutate(id, {
                          onSuccess: () => toast.success('已还原'),
                          onError: (e) => toast.error(e instanceof Error ? e.message : '还原失败'),
                        })
                      }
                    />
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
        <KnowledgeAddDialog
          open={addOpen}
          onOpenChange={setAddOpen}
          project={pid ?? ''}
          onSubmit={submitAdd}
        />
      )}
    </div>
  )
}

function DocCard({
  doc,
  showArchived,
  onArchive,
  onRestore,
  onClick,
}: {
  doc: KnowledgeDocument
  showArchived: boolean
  onArchive: (id: string) => void
  onRestore: (id: string) => void
  onClick: () => void
}) {
  return (
    <div className="flex w-full items-center gap-3 rounded-xl border border-divider px-3 py-2.5 text-left transition-colors hover:bg-accent">
      <button
        type="button"
        onClick={onClick}
        className="flex min-w-0 flex-1 items-center gap-3 text-left"
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
      </button>
      {doc.task_count ? (
        <span className="shrink-0 text-[10px] text-muted-foreground">{doc.task_count} 任务</span>
      ) : null}
      {/* TF-052 归档/还原操作 */}
      {showArchived ? (
        <button
          type="button"
          onClick={() => onRestore(doc.id)}
          className="shrink-0 rounded-lg border border-divider px-2 py-1 text-[11px] text-primary-700 transition-colors hover:bg-primary-50"
          aria-label={`还原文档 ${doc.display_name}`}
        >
          还原
        </button>
      ) : (
        <button
          type="button"
          onClick={() => onArchive(doc.id)}
          className="shrink-0 rounded-lg border border-divider px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-accent"
          title="归档（从列表/检索隐藏，任务引用与文件保留）"
          aria-label={`归档文档 ${doc.display_name}`}
        >
          归档
        </button>
      )}
    </div>
  )
}

export function StatusBadge({ status, embedded }: { status: string; embedded: number }) {
  // 嵌入状态四态（TF-052）：未嵌入 / 正在嵌入 / 已嵌入 / 嵌入失败。
  if (status === 'ok' || status === 'indexing') {
    if (status === 'indexing' || (status === 'ok' && embedded === 0)) {
      // 正在嵌入（indexing）→ 蓝色；未嵌入（ok + embedded=0）→ 灰色。
      return status === 'indexing' ? (
        <span className="rounded bg-primary-50 px-1 text-[10px] text-primary-700">正在嵌入</span>
      ) : (
        <span className="rounded bg-muted px-1 text-[10px] text-muted-foreground">未嵌入</span>
      )
    }
    if (embedded === 2) {
      return (
        <span className="rounded bg-destructive-soft px-1 text-[10px] text-destructive-ink">
          嵌入失败
        </span>
      )
    }
    return <span className="rounded bg-success-soft px-1 text-[10px] text-success-ink">已嵌入</span>
  }
  if (status === 'missing') {
    return <span className="rounded bg-warning-soft px-1 text-[10px] text-warning-ink">缺失</span>
  }
  if (status === 'failed') {
    return (
      <span className="rounded bg-destructive-soft px-1 text-[10px] text-destructive-ink">
        失败
      </span>
    )
  }
  return null
}
