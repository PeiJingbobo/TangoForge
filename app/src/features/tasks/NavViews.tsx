import { useMemo, useState } from 'react'
import { flattenTree } from '@/components/kanban/tree-utils'
import { TreeNav } from '@/components/common/TreeNav'
import { TaskNumberBadge } from '@/components/common/TaskNumberBadge'
import { StateBadge } from '@/components/common/StateBadge'
import { TagFilter } from '@/components/common/TagFilter'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useTasks } from '@/hooks/useTasks'
import { useStateMachine } from '@/hooks/useStateMachine'
import { useEventInvalidator } from '@/hooks/useEvents'
import { useProjectId } from '@/hooks/useProject'
import { useTaskDrawerStore } from '@/stores/task-drawer'
import { matchesTaskQuery } from '@/lib/task-search'
import type { StateMachineState } from '@/types/models'
import type { Task, TaskTreeNode } from '@/types/task'

/** 按选中标签过滤树（保留层级：节点自身命中或子节点命中时保留）。 */
function filterTreeByTags(nodes: TaskTreeNode[], selected: Set<string>): TaskTreeNode[] {
  if (selected.size === 0) return nodes
  const matches = (t: Task) => t.tags.some((tag) => selected.has(tag))
  return nodes
    .map((n) => ({ ...n, children: filterTreeByTags(n.children, selected) }))
    .filter((n) => matches(n) || n.children.length > 0)
}

/** 按选中标签过滤平铺任务列表。 */
function matchesTagFilter(t: Task, selected: Set<string>): boolean {
  return selected.size === 0 || t.tags.some((tag) => selected.has(tag))
}

function TaskRow({
  task,
  states,
  onClick,
}: {
  task: Task
  states: StateMachineState[]
  onClick: (id: string) => void
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onClick(task.id)}
      onKeyDown={(e) => e.key === 'Enter' && onClick(task.id)}
      className="flex cursor-pointer items-center gap-2.5 rounded-lg border border-divider px-3.5 py-2.5 transition-colors hover:border-primary-300"
    >
      <TaskNumberBadge number={task.number} />
      <span className="min-w-0 flex-1 truncate text-sm font-medium">{task.title}</span>
      {task.tags.slice(0, 2).map((t) => (
        <Badge key={t} variant="outline" className="hidden text-[10.5px] sm:inline-flex">
          {t}
        </Badge>
      ))}
      <StateBadge status={task.status} states={states} />
      <span className="text-caption text-muted-foreground">
        {task.priority > 0 ? `P${task.priority}` : ''}
      </span>
    </div>
  )
}

/** 视图搜索框（TF-042：固定位置，仅列表内部滚动）。 */
function ViewSearch({
  value,
  onChange,
  placeholder,
  label,
}: {
  value: string
  onChange: (v: string) => void
  placeholder: string
  label: string
}) {
  return (
    <input
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      aria-label={label}
      className="mb-3 w-full shrink-0 rounded-full border border-input bg-card px-3.5 py-1.5 text-sm outline-none placeholder:text-muted-foreground focus:border-primary-400 focus:ring-[3px] focus:ring-primary-100"
    />
  )
}

/** 时间线视图：按创建时间倒序（行式，非卡片）；搜索框固定 + 列表内部滚动。 */
export function TimelineView({
  tasks,
  states,
  onOpen,
}: {
  tasks: Task[]
  states: StateMachineState[]
  onOpen: (id: string) => void
}) {
  const [query, setQuery] = useState('')
  const sorted = useMemo(
    () => [...tasks].sort((a, b) => b.created_at.localeCompare(a.created_at)),
    [tasks],
  )
  // 同时匹配编号 / 标题 / 内容（TF-042）
  const visible = useMemo(() => sorted.filter((t) => matchesTaskQuery(t, query)), [sorted, query])
  return (
    <div className="flex h-full min-h-0 flex-col">
      <ViewSearch
        value={query}
        onChange={setQuery}
        placeholder="搜索编号 / 标题 / 内容…"
        label="搜索时间线任务"
      />
      <div className="min-h-0 flex-1 space-y-1.5 overflow-y-auto pr-1">
        {visible.map((t) => (
          <TaskRow key={t.id} task={t} states={states} onClick={onOpen} />
        ))}
        {visible.length === 0 && (
          <p className="py-8 text-center text-sm text-muted-foreground">
            {query.trim() ? '无匹配任务' : '没有任务'}
          </p>
        )}
      </div>
    </div>
  )
}

/** 状态分类视图：按状态机分组（背景色差分区）；搜索框固定 + 列表内部滚动。 */
export function StatusView({
  tasks,
  states,
  onOpen,
}: {
  tasks: Task[]
  states: StateMachineState[]
  onOpen: (id: string) => void
}) {
  const [query, setQuery] = useState('')
  // 同时匹配编号 / 标题 / 内容（TF-042）
  const filtered = useMemo(() => tasks.filter((t) => matchesTaskQuery(t, query)), [tasks, query])
  return (
    <div className="flex h-full min-h-0 flex-col">
      <ViewSearch
        value={query}
        onChange={setQuery}
        placeholder="搜索编号 / 标题 / 内容…"
        label="搜索状态分类任务"
      />
      <div className="min-h-0 flex-1 overflow-y-auto pr-1">
        <div className="space-y-6">
          {states.map((s) => {
            const group = filtered.filter((t) => t.status === s.Key)
            if (group.length === 0) return null
            return (
              <div key={s.Key}>
                <div className="mb-2 flex items-center gap-2 px-1">
                  <span
                    className="size-2 rounded-full"
                    style={{ backgroundColor: s.Color || '#9aa0a6' }}
                  />
                  <span className="text-sm font-semibold">{s.Label || s.Key}</span>
                  <span className="text-caption text-muted-foreground">{group.length}</span>
                </div>
                <div className="space-y-1.5">
                  {group.map((t) => (
                    <TaskRow key={t.id} task={t} states={states} onClick={onOpen} />
                  ))}
                </div>
              </div>
            )
          })}
          {filtered.length === 0 && (
            <p className="py-8 text-center text-sm text-muted-foreground">
              {query.trim() ? '无匹配任务' : '没有任务'}
            </p>
          )}
        </div>
      </div>
    </div>
  )
}

/** 导航三视图页（TF-026 / TF-042 滚动布局）：标题与 Tab 固定，仅视图列表内部滚动。 */
export function NavPage() {
  const pid = useProjectId()
  const { data: taskData } = useTasks(undefined, pid)
  const { data: sm } = useStateMachine(pid)
  useEventInvalidator(pid)

  const flat = useMemo(() => flattenTree(taskData?.tree ?? []), [taskData])
  const states = sm?.States ?? []
  // 标签多选筛选（导航三视图共用）
  const [selectedTags, setSelectedTags] = useState<Set<string>>(new Set())
  const allTags = useMemo(() => [...new Set(flat.flatMap((t) => t.tags))].sort(), [flat])

  const openTaskDrawer = useTaskDrawerStore((st) => st.openDrawer)
  const openTask = (id: string) => openTaskDrawer({ taskId: id })

  const filteredFlat = useMemo(
    () => flat.filter((t) => matchesTagFilter(t, selectedTags)),
    [flat, selectedTags],
  )
  const filteredTree = useMemo(
    () => filterTreeByTags((taskData?.tree ?? []) as TaskTreeNode[], selectedTags),
    [taskData, selectedTags],
  )

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* 顶部标题（固定） */}
      <div className="shrink-0">
        <h1 className="text-h2 text-foreground">任务导航</h1>
        <p className="mt-1 text-caption text-muted-foreground">
          共 {flat.length} 个任务 · 树形结构与后端一致
        </p>
      </div>

      {/* Tab + 标签筛选同一行（Tab 左侧，标签筛选右侧，高度对齐） */}
      <Tabs defaultValue="tree" className="mt-5 flex min-h-0 flex-1 flex-col">
        <div className="flex shrink-0 items-center gap-3">
          <TabsList className="w-fit shrink-0">
            <TabsTrigger value="tree">树形</TabsTrigger>
            <TabsTrigger value="timeline">时间线</TabsTrigger>
            <TabsTrigger value="status">状态分类</TabsTrigger>
          </TabsList>
          <TagFilter
            tags={allTags}
            selected={selectedTags}
            onChange={setSelectedTags}
            className="min-w-0 flex-1"
          />
        </div>
        <TabsContent value="tree" className="mt-4 min-h-0 flex-1">
          <TreeNav tree={filteredTree} states={states} onSelect={openTask} />
        </TabsContent>
        <TabsContent value="timeline" className="mt-4 min-h-0 flex-1">
          <TimelineView tasks={filteredFlat} states={states} onOpen={openTask} />
        </TabsContent>
        <TabsContent value="status" className="mt-4 min-h-0 flex-1">
          <StatusView tasks={filteredFlat} states={states} onOpen={openTask} />
        </TabsContent>
      </Tabs>
    </div>
  )
}
