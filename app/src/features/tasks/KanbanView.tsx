import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { Search, Plus } from 'lucide-react'
import type { DragEndEvent } from '@dnd-kit/core'
import { KanbanBoard } from '@/components/kanban/kanban-board'
import { resolveDragTarget } from '@/components/kanban/drag-logic'
import { flattenTree } from '@/components/kanban/tree-utils'
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
import { cn } from '@/lib/utils'

/** 看板视图（TF-025）：状态机动态列 + dnd-kit 拖拽流转 + 虚拟滚动 + 过滤搜索 */
export function KanbanView() {
  const { data: taskData } = useTasks()
  const { data: sm } = useStateMachine()
  const pid = useProjectId()
  const navigate = useNavigate()

  const [query, setQuery] = useState('')
  const [tagFilter, setTagFilter] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const { getEffectiveStatus, moveTask } = useKanbanMutations(pid)

  // WS 实时失效（多端等价：其他通道的写操作实时反映）
  useEventInvalidator(pid)

  const allTasks = useMemo(() => flattenTree(taskData?.tree ?? []), [taskData])
  const columns = useMemo(() => (sm?.States ?? []).filter((s) => s.Key !== 'archived'), [sm])
  const allTags = useMemo(() => [...new Set(allTasks.flatMap((t) => t.tags))].sort(), [allTasks])

  const filtered = useMemo(
    () =>
      allTasks.filter((t) => {
        if (t.status === 'archived') return false
        if (query) {
          const q = query.toLowerCase()
          if (!t.title.toLowerCase().includes(q) && !t.description?.toLowerCase().includes(q)) {
            return false
          }
        }
        if (tagFilter && !t.tags.includes(tagFilter)) return false
        return true
      }),
    [allTasks, query, tagFilter],
  )

  const tasksById = useMemo(() => new Map(allTasks.map((t) => [t.id, t])), [allTasks])

  const createTask = useCreateTask(pid)

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
  }

  return (
    <div>
      {/* 工具栏：标题 + 搜索 + 标签过滤 + 新建 */}
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-h2 text-foreground">任务看板</h1>
          <p className="mt-1 text-caption text-muted-foreground">
            基于项目状态机动态生成列 · {filtered.length} 个任务
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="搜索任务…"
              aria-label="搜索任务"
              className="h-9 w-52 rounded-full pl-9 text-sm"
            />
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setTagFilter(null)}
            className={cn(!tagFilter && 'text-muted-foreground')}
          >
            全部
          </Button>
          {allTags.slice(0, 5).map((tag) => (
            <Button
              key={tag}
              variant="ghost"
              size="sm"
              onClick={() => setTagFilter(tagFilter === tag ? null : tag)}
              className={cn(tagFilter === tag && 'bg-primary-50 text-primary-700')}
            >
              #{tag}
            </Button>
          ))}
          <Button size="sm" onClick={() => setCreateOpen(true)}>
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
        <KanbanBoard
          states={columns}
          tasks={filtered}
          getEffectiveStatus={getEffectiveStatus}
          onOpenTask={(id) => navigate(`/project/${encodeURIComponent(pid ?? '')}/tasks/${id}`)}
          onDragEnd={handleDragEnd}
        />
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
            navigate(`/project/${encodeURIComponent(pid ?? '')}/tasks/${t.id}`)
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
