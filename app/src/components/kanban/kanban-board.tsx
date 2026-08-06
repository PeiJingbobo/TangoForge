import { useState } from 'react'
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import { sortableKeyboardCoordinates } from '@dnd-kit/sortable'
import { KanbanColumn } from './kanban-column'
import { TaskCard } from './task-card'
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

/** 看板主体：DndContext + 状态机动态列（UI-VISION 场景 B） */
export function KanbanBoard({
  states,
  tasks,
  getEffectiveStatus,
  onOpenTask,
  onDragEnd,
}: KanbanBoardProps) {
  const [activeId, setActiveId] = useState<string | null>(null)
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const activeTask = activeId ? tasks.find((t) => t.id === activeId) : null
  const effective = getEffectiveStatus ?? ((t: Task) => t.status)

  const byStatus = (key: string): Task[] => tasks.filter((t) => effective(t) === key)

  const handleDragStart = (e: DragStartEvent) => setActiveId(String(e.active.id))
  const handleDragCancel = () => setActiveId(null)

  return (
    <DndContext
      sensors={sensors}
      onDragStart={handleDragStart}
      onDragCancel={handleDragCancel}
      onDragEnd={(e) => {
        setActiveId(null)
        onDragEnd(e)
      }}
    >
      <div className="grid grid-cols-[repeat(auto-fit,minmax(260px,1fr))] items-start gap-3.5">
        {states.map((col) => (
          <KanbanColumn key={col.Key} col={col} tasks={byStatus(col.Key)} onOpenTask={onOpenTask} />
        ))}
      </div>
      <DragOverlay>{activeTask ? <TaskCard task={activeTask} /> : null}</DragOverlay>
    </DndContext>
  )
}
