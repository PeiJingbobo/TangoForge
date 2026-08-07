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
  /** 目标插入 index（不含 active 的列内位置） */
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
    const container = effective(task)
    setDragState({
      activeId: id,
      overContainer: container,
      overIndex: resolveOverIndex(byStatus(container), id, id),
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
            tasks={byStatus(col.Key).filter((t) => t.id !== activeId)}
            placeholderIndex={
              dragState?.overContainer === col.Key ? dragState.overIndex : undefined
            }
            onOpenTask={onOpenTask}
          />
        ))}
      </div>
      <DragOverlay>{activeTask ? <TaskCard task={activeTask} /> : null}</DragOverlay>
    </DndContext>
  )
}
