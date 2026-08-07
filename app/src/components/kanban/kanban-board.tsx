import { useState } from 'react'
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
  const effective = getEffectiveStatus ?? ((t: Task) => t.status)

  const byStatus = (key: string): Task[] => tasks.filter((t) => effective(t) === key)

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
      <DragOverlay>{activeTask ? <TaskCard task={activeTask} overlay /> : null}</DragOverlay>
    </DndContext>
  )
}
