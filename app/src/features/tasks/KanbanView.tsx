import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { Search, Plus } from 'lucide-react'
import type { DragEndEvent } from '@dnd-kit/core'
import { KanbanBoard } from '@/components/kanban/kanban-board'
import {
  resolveDragTarget,
  applyColumnOrder,
  resolveColOrder,
  stabilizeTreeOrder,
} from '@/components/kanban/drag-logic'
import { flattenTree, treeOrderIndex } from '@/components/kanban/tree-utils'
import { TagFilter } from '@/components/common/TagFilter'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { useTasks, useCreateTask } from '@/hooks/useTasks'
import { useStateMachine } from '@/hooks/useStateMachine'
import { useEventInvalidator } from '@/hooks/useEvents'
import { useKanbanMutations } from '@/hooks/useKanban'
import { useProjectId } from '@/hooks/useProject'
import { useTaskDrawerStore } from '@/stores/task-drawer'
import { matchesTaskQuery } from '@/lib/task-search'
import type { Task } from '@/types/task'

/** 看板视图（TF-025）：状态机动态列 + dnd-kit 拖拽流转 + 虚拟滚动 + 过滤搜索 */
export function KanbanView() {
  const { data: taskData } = useTasks()
  const { data: sm } = useStateMachine()
  const pid = useProjectId()

  const [query, setQuery] = useState('')
  // 标签多选筛选（空集合 = 不过滤）
  const [tagFilter, setTagFilter] = useState<Set<string>>(new Set())
  const [createOpen, setCreateOpen] = useState(false)
  // 本地列内顺序（拖拽结束保持，避免数据刷新回跳闪烁；仅会话内，未记录列用默认序）
  const [colOrder, setColOrder] = useState<Record<string, string[]>>({})
  const { getEffectiveStatus, moveTask } = useKanbanMutations(pid)
  const openTaskDrawer = useTaskDrawerStore((st) => st.openDrawer)

  // WS 实时失效（多端等价：其他通道的写操作实时反映）
  useEventInvalidator(pid)

  const allTasks = useMemo(() => flattenTree(taskData?.tree ?? []), [taskData])
  // 树 DFS 序索引（QA 2026-08-09：列内子任务始终跟随父任务）
  const treeIdx = useMemo(() => treeOrderIndex(taskData?.tree ?? []), [taskData])
  const columns = useMemo(() => (sm?.States ?? []).filter((s) => s.Key !== 'archived'), [sm])
  const allTags = useMemo(() => [...new Set(allTasks.flatMap((t) => t.tags))].sort(), [allTasks])

  const filtered = useMemo(
    () =>
      allTasks.filter((t) => {
        if (t.status === 'archived') return false
        // 同时匹配编号 / 标题 / 内容（TF-042）
        if (!matchesTaskQuery(t, query)) return false
        // 标签多选：命中任一选中标签即保留
        if (tagFilter.size > 0 && !t.tags.some((tag) => tagFilter.has(tag))) return false
        return true
      }),
    [allTasks, query, tagFilter],
  )

  const tasksById = useMemo(() => new Map(allTasks.map((t) => [t.id, t])), [allTasks])

  // 看板展示顺序：本地列内顺序覆盖（拖拽结束保持；未记录列原序）
  // → 树序稳定化：同列父子始终相邻（子任务紧跟父任务），拖拽仅流转状态
  const displayTasks = useMemo(
    () =>
      stabilizeTreeOrder(
        applyColumnOrder(filtered, colOrder, getEffectiveStatus),
        treeIdx,
        getEffectiveStatus,
      ),
    [filtered, colOrder, getEffectiveStatus, treeIdx],
  )

  const createTask = useCreateTask(pid)

  // 拖拽结束：状态流转 + 本地列内顺序保持（resolveColOrder 计算目标列新顺序）
  const handleDragEnd = (e: DragEndEvent) => {
    const { active, over } = e
    if (!over) return
    const target = resolveDragTarget(
      String(active.id),
      String(over.id),
      tasksById,
      columns.map((c) => c.Key),
    )
    if (!target) return
    moveTask(target.taskId, target.to)
    const activeTask = tasksById.get(String(active.id))
    if (!activeTask) return
    const activeId = String(active.id)
    const fromCol = getEffectiveStatus(activeTask)
    setColOrder((prev) => {
      const next = { ...prev }
      // 目标列新顺序（当前 id 列表：colOrder 或默认，过滤已删除/失效任务）
      const targetDefault = filtered
        .filter((t) => getEffectiveStatus(t) === target.to)
        .map((t) => t.id)
      const targetIds = (prev[target.to] ?? targetDefault).filter((id) => tasksById.has(id))
      const targetTasks = targetIds
        .map((id) => tasksById.get(id))
        .filter((t): t is Task => Boolean(t))
      next[target.to] = resolveColOrder(targetTasks, String(over.id), activeId)
      // 源列移除（跨列时）
      if (fromCol !== target.to) {
        const srcDefault = filtered
          .filter((t) => getEffectiveStatus(t) === fromCol)
          .map((t) => t.id)
        next[fromCol] = (prev[fromCol] ?? srcDefault).filter(
          (id) => id !== activeId && tasksById.has(id),
        )
      }
      return next
    })
  }

  return (
    // h-full flex-col：工具栏固定，看板区 flex-1 撑满剩余高度（列内滚动，页面不滚动）
    <div className="flex h-full min-h-0 flex-col">
      {/* 工具栏：标题一行；搜索 + 标签多选筛选 + 新建 一行（新建右侧对齐） */}
      <div className="mb-5 shrink-0">
        <div>
          <h1 className="text-h2 text-foreground">任务看板</h1>
          <p className="mt-1 text-caption text-muted-foreground">
            基于项目状态机动态生成列 · {filtered.length} 个任务
          </p>
        </div>
        <div className="mt-4 flex min-w-0 items-center gap-3">
          <div className="relative shrink-0">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="搜索编号 / 标题 / 内容…"
              aria-label="搜索任务"
              className="h-9 w-52 rounded-full pl-9 text-sm"
            />
          </div>
          <TagFilter
            tags={allTags}
            selected={tagFilter}
            onChange={setTagFilter}
            className="min-w-0 flex-1"
          />
          <Button size="sm" className="ml-auto shrink-0" onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            新建任务
          </Button>
        </div>
      </div>

      {columns.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-border p-14 text-center text-body text-muted-foreground">
          该项目状态机为空或全部为 archived 状态。
        </div>
      ) : (
        // 看板区：flex-1 撑满剩余高度（列内滚动，页面不滚动）
        <div className="min-h-0 flex-1">
          <KanbanBoard
            states={columns}
            tasks={displayTasks}
            getEffectiveStatus={getEffectiveStatus}
            onOpenTask={(id) => openTaskDrawer({ taskId: id })}
            onDragEnd={handleDragEnd}
          />
        </div>
      )}

      {/* 快速新建（完整表单在 TF-026 TaskForm） */}
      <QuickCreateDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        statuses={columns.map((c) => ({ key: c.Key, label: c.Label }))}
        onCreate={async (title, status) => {
          try {
            const t = await createTask.mutateAsync({ title, status })
            toast.success(`任务已创建：${t.title}`)
            openTaskDrawer({ taskId: t.id })
          } catch (err) {
            toast.error(err instanceof Error ? err.message : '创建失败')
          }
        }}
      />
    </div>
  )
}

interface QuickCreateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  statuses: { key: string; label: string }[]
  onCreate: (title: string, status: string) => Promise<void>
}

function QuickCreateDialog({ open, onOpenChange, statuses, onCreate }: QuickCreateDialogProps) {
  const [title, setTitle] = useState('')
  const [status, setStatus] = useState(statuses[0]?.key ?? 'todo')

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>新建任务</DialogTitle>
          <DialogDescription>任务将进入草稿列对应的状态。</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div>
            <Label htmlFor="qct-title">标题</Label>
            <Input
              id="qct-title"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="任务标题"
              autoFocus
            />
          </div>
          <div>
            <Label>状态</Label>
            <div className="mt-1.5 flex flex-wrap gap-1.5">
              {statuses.map((s) => (
                <Badge
                  key={s.key}
                  variant={status === s.key ? 'default' : 'outline'}
                  className="cursor-pointer px-3 py-1.5"
                  onClick={() => setStatus(s.key)}
                >
                  {s.label || s.key}
                </Badge>
              ))}
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button
            disabled={!title.trim()}
            onClick={() => {
              void onCreate(title.trim(), status)
              setTitle('')
              onOpenChange(false)
            }}
          >
            创建
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
