import { memo } from 'react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import type { Task } from '@/types/task'
import { cn } from '@/lib/utils'

/**
 * 优先级色带（docs/UI-VISION 语义色；红高灰低）：
 * 0 灰 / 1 绿 / 2 天蓝 / 3 琥珀 / 4-5 红
 */
export const PRIORITY_BARS: Record<number, string> = {
  0: 'bg-muted-foreground/30',
  1: 'bg-success-ink',
  2: 'bg-primary-400',
  3: 'bg-warning-ink',
  4: 'bg-destructive-ink',
  5: 'bg-destructive',
}

/** 标签哈希着色：稳定映射到 5 组 token 软色（禁止硬编码色值） */
const TAG_PALETTE = [
  'bg-primary-50 text-primary-700',
  'bg-success-soft text-success-ink',
  'bg-warning-soft text-warning-ink',
  'bg-destructive-soft text-destructive-ink',
  'bg-secondary text-secondary-foreground',
] as const

function hashTag(tag: string): number {
  let h = 0
  for (let i = 0; i < tag.length; i++) h = (h * 31 + tag.charCodeAt(i)) >>> 0
  return h
}

export const TaskTag = memo(function TaskTag({ tag }: { tag: string }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-[10.5px] font-semibold leading-none',
        TAG_PALETTE[hashTag(tag) % TAG_PALETTE.length],
      )}
    >
      {tag}
    </span>
  )
})

interface TaskCardProps {
  task: Task
  onOpen?: (id: string) => void
  /**
   * DragOverlay 浮层模式：正常显示（不隐藏、不加拖拽半透明）。
   * 列表中的 active 卡片保留 DOM（dnd-kit 测量依赖）但完全隐形（opacity-0）。
   */
  overlay?: boolean
}

/**
 * 看板任务卡片（UI-VISION：唯一允许的卡片形态——可点击/可拖拽/可对比）。
 * 左侧优先级色带 + 标题 + 标签 + 负责人。
 * 拖拽中：列表内的 active 卡片 opacity-0 隐形但保留 DOM（供 dnd-kit 测量 rect，
 * 移除会导致 DragOverlay 测量循环 → Maximum update depth exceeded 白屏）；
 * 视觉由 DragOverlay 的 overlay 副本接管。
 */
export const TaskCard = memo(function TaskCard({ task, onOpen, overlay = false }: TaskCardProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: task.id,
  })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      onClick={() => onOpen?.(task.id)}
      role="button"
      tabIndex={0}
      aria-label={`任务 ${task.title}`}
      className={cn(
        'group relative mb-2 cursor-grab rounded-[14px] border border-border bg-card px-3.5 py-3 pl-4 transition-shadow hover:border-primary-300 hover:shadow-[var(--shadow-card)] active:cursor-grabbing',
        // 列表内被拖卡片：完全隐形（保留占位与测量）；overlay 副本正常显示 + 浮起阴影
        isDragging && !overlay && 'z-10 opacity-0',
        overlay && 'shadow-[var(--shadow-card)]',
      )}
    >
      {/* 优先级色带 */}
      <span
        aria-hidden
        className={cn(
          'absolute left-0 top-3 bottom-3 w-[3px] rounded-full',
          PRIORITY_BARS[Math.min(5, Math.max(0, task.priority))],
        )}
      />
      <div className="line-clamp-2 text-sm font-semibold leading-snug tracking-[-0.005em]">
        {task.title}
      </div>
      {task.tags.length > 0 && (
        <div className="mt-2 flex flex-wrap items-center gap-1">
          {task.tags.slice(0, 4).map((t) => (
            <TaskTag key={t} tag={t} />
          ))}
        </div>
      )}
      <div className="mt-2 flex items-center justify-between">
        <span className="text-[11px] text-muted-foreground">
          {task.priority > 0 && `P${task.priority} · `}
          {task.status}
        </span>
        {task.assignee && (
          <span className="grid size-5 place-items-center rounded-full bg-primary-200 text-[9px] font-bold text-primary-800">
            {task.assignee.slice(0, 1).toUpperCase()}
          </span>
        )}
      </div>
    </div>
  )
})
