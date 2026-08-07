import { useMemo } from 'react'
import { flattenTree } from '@/components/kanban/tree-utils'
import { TreeNav } from '@/components/common/TreeNav'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useTasks } from '@/hooks/useTasks'
import { useStateMachine } from '@/hooks/useStateMachine'
import { useEventInvalidator } from '@/hooks/useEvents'
import { useProjectId } from '@/hooks/useProject'
import { useTaskDrawerStore } from '@/stores/task-drawer'
import type { Task, TaskTreeNode } from '@/types/task'

function TaskRow({ task, onClick }: { task: Task; onClick: (id: string) => void }) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onClick(task.id)}
      onKeyDown={(e) => e.key === 'Enter' && onClick(task.id)}
      className="flex cursor-pointer items-center gap-2.5 rounded-lg border border-divider px-3.5 py-2.5 transition-colors hover:border-primary-300"
    >
      <span className="min-w-0 flex-1 truncate text-sm font-medium">{task.title}</span>
      {task.tags.slice(0, 2).map((t) => (
        <Badge key={t} variant="outline" className="hidden text-[10.5px] sm:inline-flex">
          {t}
        </Badge>
      ))}
      <span className="text-caption text-muted-foreground">
        {task.status}
        {task.priority > 0 ? ` · P${task.priority}` : ''}
      </span>
    </div>
  )
}

/** 时间线视图：按创建时间倒序（行式，非卡片） */
function TimelineView({ tasks, onOpen }: { tasks: Task[]; onOpen: (id: string) => void }) {
  const sorted = useMemo(
    () => [...tasks].sort((a, b) => b.created_at.localeCompare(a.created_at)),
    [tasks],
  )
  return (
    <div className="space-y-1.5">
      {sorted.map((t) => (
        <TaskRow key={t.id} task={t} onClick={onOpen} />
      ))}
      {sorted.length === 0 && (
        <p className="py-8 text-center text-sm text-muted-foreground">没有任务</p>
      )}
    </div>
  )
}

/** 状态分类视图：按状态机分组（背景色差分区） */
function StatusView({
  tasks,
  states,
  onOpen,
}: {
  tasks: Task[]
  states: { Key: string; Label: string }[]
  onOpen: (id: string) => void
}) {
  return (
    <div className="space-y-6">
      {states.map((s) => {
        const group = tasks.filter((t) => t.status === s.Key)
        if (group.length === 0) return null
        return (
          <div key={s.Key}>
            <div className="mb-2 flex items-center gap-2 px-1">
              <span className="size-2 rounded-full bg-primary-500" />
              <span className="text-sm font-semibold">{s.Label || s.Key}</span>
              <span className="text-caption text-muted-foreground">{group.length}</span>
            </div>
            <div className="space-y-1.5">
              {group.map((t) => (
                <TaskRow key={t.id} task={t} onClick={onOpen} />
              ))}
            </div>
          </div>
        )
      })}
    </div>
  )
}

/** 导航三视图页（TF-026）：树形 / 时间线 / 状态分类 */
export function NavPage() {
  const pid = useProjectId()
  const { data: taskData } = useTasks(undefined, pid)
  const { data: sm } = useStateMachine(pid)
  useEventInvalidator(pid)

  const flat = useMemo(() => flattenTree(taskData?.tree ?? []), [taskData])
  const states = sm?.States ?? []

  const openTaskDrawer = useTaskDrawerStore((st) => st.openDrawer)
  const openTask = (id: string) => openTaskDrawer({ taskId: id })

  const treeWithIds = (taskData?.tree ?? []) as TaskTreeNode[]

  return (
    <div>
      <h1 className="text-h2 text-foreground">任务导航</h1>
      <p className="mt-1 text-caption text-muted-foreground">
        共 {flat.length} 个任务 · 树形结构与后端一致
      </p>
      <Tabs defaultValue="tree" className="mt-6">
        <TabsList>
          <TabsTrigger value="tree">树形</TabsTrigger>
          <TabsTrigger value="timeline">时间线</TabsTrigger>
          <TabsTrigger value="status">状态分类</TabsTrigger>
        </TabsList>
        <TabsContent value="tree" className="pt-4">
          <TreeNav tree={treeWithIds} onSelect={openTask} />
        </TabsContent>
        <TabsContent value="timeline" className="pt-4">
          <TimelineView tasks={flat} onOpen={openTask} />
        </TabsContent>
        <TabsContent value="status" className="pt-4">
          <StatusView tasks={flat} states={states} onOpen={openTask} />
        </TabsContent>
      </Tabs>
    </div>
  )
}
