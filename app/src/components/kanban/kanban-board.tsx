import { useMemo, useState } from 'react'
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import { sortableKeyboardCoordinates } from '@dnd-kit/sortable'
import { KanbanColumn } from './kanban-column'
import { TaskCard } from './task-card'
import { resolveOverIndex } from './drag-logic'
import type { Task } from '@/types/task'
import type { StateMachineState } from '@/types/models'

export interface KanbanBoardProps {
  states: StateMachineState[]
  tasks: Task[]
  /** 任务当前展示状态（乐观拖拽：本地覆盖优先） */
  getEffectiveStatus?: (t: Task) => string
  onOpenTask: (id: string) => void
  onDragEnd: (e: DragEndEvent) => void
}

interface DragState {
  activeId: string
  /** 目标容器（当前悬停列 key） */
  overContainer: string
  /** 目标插入 index（含 active 的列内位置）；-1 = 未移动，不显示占位符 */
  overIndex: number
}

/** 看板主体：DndContext + 状态机动态列（UI-VISION 场景 B） */
export function KanbanBoard({
  states,
  tasks,
  getEffectiveStatus,
  onOpenTask,
  onDragEnd,
}: KanbanBoardProps) {
  const [activeId, setActiveId] = useState<string | null>(null)
  const [dragState, setDragState] = useState<DragState | null>(null)
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const activeTask = activeId ? tasks.find((t) => t.id === activeId) : null
  // 引用稳定（useMemo）：effective 参与 byStatusMap 的 useMemo 依赖，不能每帧新建
  const effective = useMemo(
    () => getEffectiveStatus ?? ((t: Task) => t.status),
    [getEffectiveStatus],
  )

  // 按状态分组结果 memo 化（引用稳定）：拖拽中 DndContext 高频触发消费组件重渲染，
  // 若每次 filter 新建数组 → 列 items 重算 → 虚拟滚动测量抖动 ↔ dnd-kit ResizeObserver 测量
  // 互相触发 setState → Maximum update depth exceeded 白屏。稳定引用可打破该反馈环。
  const byStatusMap = useMemo(() => {
    const map = new Map<string, Task[]>()
    for (const s of states) map.set(s.Key, [])
    for (const t of tasks) {
      const key = effective(t)
      const bucket = map.get(key)
      if (bucket) bucket.push(t)
    }
    return map
  }, [tasks, states, effective])

  const byStatus = (key: string): Task[] => byStatusMap.get(key) ?? []

  const handleDragStart = (e: DragStartEvent) => {
    const id = String(e.active.id)
    const task = tasks.find((t) => t.id === id)
    if (!task) return
    setActiveId(id)
    // 初始不显示占位符（over 仍为自身，避免与隐藏的 active 卡片形成双空位）；首次 dragOver 后显示
    setDragState({
      activeId: id,
      overContainer: effective(task),
      overIndex: -1,
    })
  }

  const handleDragOver = (e: DragOverEvent) => {
    if (!dragState || !e.over) return
    const overId = String(e.over.id)
    // over 为 active 自身（拖起未移动/回到原位）：不更新占位，避免原位置下方出现占位符
    if (overId === dragState.activeId) return
    const overTask = tasks.find((t) => t.id === overId)
    const container = overTask ? effective(overTask) : overId
    setDragState({
      activeId: dragState.activeId,
      overContainer: container,
      overIndex: resolveOverIndex(byStatus(container), overId, dragState.activeId),
    })
  }

  const handleDragCancel = () => {
    setActiveId(null)
    setDragState(null)
  }

  return (
    <DndContext
      sensors={sensors}
      onDragStart={handleDragStart}
      onDragOver={handleDragOver}
      onDragCancel={handleDragCancel}
      onDragEnd={(e) => {
        setActiveId(null)
        setDragState(null)
        onDragEnd(e)
      }}
    >
      <div className="grid grid-cols-[repeat(auto-fit,minmax(260px,1fr))] items-start gap-3.5">
        {states.map((col) => (
          <KanbanColumn
            key={col.Key}
            col={col}
            // active 卡片保留在列内（dnd-kit 测量依赖），由 TaskCard 隐形；占位符指示目标
            tasks={byStatus(col.Key)}
            placeholderIndex={
              dragState?.overContainer === col.Key && dragState.overIndex >= 0
                ? dragState.overIndex
                : undefined
            }
            onOpenTask={onOpenTask}
          />
        ))}
      </div>
      {/* dropAnimation=null：松手即消失（不飞回原位置，避免与乐观落位视觉冲突产生闪烁） */}
      <DragOverlay dropAnimation={null}>
        {activeTask ? <TaskCard task={activeTask} overlay /> : null}
      </DragOverlay>
    </DndContext>
  )
}
