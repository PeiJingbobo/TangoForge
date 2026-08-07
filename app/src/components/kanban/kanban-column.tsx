import { useMemo, useRef } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { useDroppable } from '@dnd-kit/core'
import { TaskCard } from './task-card'
import type { Task } from '@/types/task'
import type { StateMachineState } from '@/types/models'
import { cn } from '@/lib/utils'

interface KanbanColumnProps {
  col: StateMachineState
  tasks: Task[]
  /** 拖拽目标插入位置（该列为目标容器时）：在移除 active 后的列内 index 处渲染占位卡片 */
  placeholderIndex?: number
  onOpenTask: (id: string) => void
}

const COLUMN_DOT: Record<string, string> = {
  todo: 'bg-muted-foreground',
  doing: 'bg-primary-500',
  done: 'bg-success',
}

/** 卡片间距（TaskCard mb-2 = 8px）：动态测量须计入，保证任意高度卡片间距恒等 */
const CARD_GAP = 8

/** 估算卡片高度（首帧/无布局环境的初始值） */
const ESTIMATED_CARD_HEIGHT = 108

/** 占位卡片高度（与常规卡片一致） */
const PLACEHOLDER_HEIGHT = 104

/** 占位卡片虚拟项 key（同一时刻仅一个目标容器，可复用固定 key） */
const PLACEHOLDER_ID = '__kanban-placeholder__'

/** 虚拟项：真实任务或占位符 */
type ListItem = Task | { __placeholder: true }

/**
 * 看板列：背景色差分组（UI-VISION §2.2 区域级色差），任务条目为卡片。
 * 每列独立虚拟滚动（≥1000 任务不卡，docs/TECHNICAL.md §4.3）。
 * 动态测量：标题 1/2 行、带标签/负责人等卡片高度不同，固定估算会导致长卡片
 * 挤压相邻卡片、间距被压扁——measureElement 按实际高度 + 固定间距布局。
 * 拖拽体验：active 卡片由 DragOverlay 浮起（本列已移除），目标插入位置渲染占位卡片。
 */
export function KanbanColumn({ col, tasks, placeholderIndex, onOpenTask }: KanbanColumnProps) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const { setNodeRef, isOver } = useDroppable({ id: col.Key })

  const items = useMemo<ListItem[]>(() => {
    if (placeholderIndex === undefined || placeholderIndex < 0 || placeholderIndex > tasks.length) {
      return tasks
    }
    return [
      ...tasks.slice(0, placeholderIndex),
      { __placeholder: true },
      ...tasks.slice(placeholderIndex),
    ]
  }, [tasks, placeholderIndex])

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ESTIMATED_CARD_HEIGHT,
    getItemKey: (i) => ('__placeholder' in items[i] ? PLACEHOLDER_ID : (items[i] as Task).id),
    overscan: 6,
    // 首帧初始视口（测试环境无 ResizeObserver 布局时保证渲染；浏览器由 RO 实时更新）
    initialRect: { width: 300, height: 800 },
    // 动态测量：jsdom 无布局（height=0）时回退估算值，保证测试可预测
    measureElement: (el) => {
      const h = el.getBoundingClientRect().height
      return (h > 0 ? h : ESTIMATED_CARD_HEIGHT) + CARD_GAP
    },
  })

  return (
    <div
      ref={setNodeRef}
      className={cn(
        'flex flex-col rounded-[14px] bg-muted p-2.5',
        isOver && 'ring-2 ring-primary-300',
      )}
    >
      <div className="flex items-center justify-between px-2 py-2.5">
        <div className="flex items-center gap-2">
          <span
            aria-hidden
            className={cn('size-2 rounded-full', COLUMN_DOT[col.Key] ?? 'bg-primary-500')}
          />
          <span className="text-[13px] font-semibold">{col.Label || col.Key}</span>
          <span className="rounded-full bg-card px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
            {tasks.length}
          </span>
        </div>
      </div>

      <div
        ref={scrollRef}
        className="max-h-[calc(100vh-260px)] min-h-[180px] flex-1 overflow-auto pr-0.5"
      >
        <SortableContext items={tasks.map((t) => t.id)} strategy={verticalListSortingStrategy}>
          <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
            {virtualizer.getVirtualItems().map((vi) => {
              const item = items[vi.index]
              return (
                <div
                  key={vi.key}
                  data-index={vi.index}
                  ref={virtualizer.measureElement}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    transform: `translateY(${vi.start}px)`,
                  }}
                >
                  {'__placeholder' in item ? (
                    <PlaceholderCard />
                  ) : (
                    <TaskCard task={item} onOpen={onOpenTask} />
                  )}
                </div>
              )
            })}
          </div>
        </SortableContext>
        {tasks.length === 0 && placeholderIndex === undefined && (
          <div className="mt-2 rounded-[14px] border border-dashed border-border px-3 py-6 text-center text-xs text-muted-foreground">
            拖拽任务到此处
          </div>
        )}
      </div>
    </div>
  )
}

/** 拖拽占位卡片：虚线框指示目标插入位置 */
function PlaceholderCard() {
  return (
    <div
      aria-hidden
      style={{ height: PLACEHOLDER_HEIGHT }}
      className="mb-2 flex items-center justify-center rounded-[14px] border-2 border-dashed border-primary-300 bg-primary-50/50 text-xs font-medium text-primary-600"
    >
      松开放置于此
    </div>
  )
}
