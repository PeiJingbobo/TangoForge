import { useRef } from 'react'
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
  /** 拖拽目标插入位置（该列为目标容器时）：在目标 index 卡片顶部渲染插入指示线 */
  placeholderIndex?: number
  /**
   * 拖拽目标容器归属（TF-036 拖拽高亮优化）：
   * 由 board 的 dragState.overContainer 传入（over 命中列 id 或列内卡片均归因到列），
   * 保证「悬停在卡片之间」与「列空白区」同样触发列高亮边框；
   * 与 useDroppable.isOver（仅指针直接命中列 droppable 区域）取并集。
   */
  isDropTarget?: boolean
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

/**
 * 看板列：背景色差分组（UI-VISION §2.2 区域级色差），任务条目为卡片。
 * 每列独立虚拟滚动（≥1000 任务不卡，docs/TECHNICAL.md §4.3）。
 *
 * 拖拽方案（稳定版，修复 Maximum update depth exceeded 反馈环）：
 * - active 卡片保留 DOM 隐形（opacity-0，dnd-kit 测量依赖），DragOverlay 浮起视觉；
 * - 目标占位为**绝对定位指示线**（不插入 items、不改变卡片布局）——
 *   若占位符插入虚拟滚动 items，卡片 translateY 每帧变化 → dnd-kit 提交后测量
 *   rect 变化 → layout effect 同步 setState → 循环白屏。指示线布局恒定，测量稳定。
 * - 动态测量：标题 1/2 行、带标签等卡片高度不同，measureElement 按实际高度 + 固定间距布局。
 */
export function KanbanColumn({
  col,
  tasks,
  placeholderIndex,
  isDropTarget = false,
  onOpenTask,
}: KanbanColumnProps) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const { setNodeRef, isOver } = useDroppable({ id: col.Key })

  const virtualizer = useVirtualizer({
    count: tasks.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ESTIMATED_CARD_HEIGHT,
    getItemKey: (i) => tasks[i].id,
    overscan: 6,
    // 首帧初始视口（测试环境无 ResizeObserver 布局时保证渲染；浏览器由 RO 实时更新）
    initialRect: { width: 300, height: 800 },
    // 动态测量：jsdom 无布局（height=0）时回退估算值，保证测试可预测
    measureElement: (el) => {
      const h = el.getBoundingClientRect().height
      return (h > 0 ? h : ESTIMATED_CARD_HEIGHT) + CARD_GAP
    },
  })

  // 指示线位置：目标 index 卡片顶部（虚拟项 start）；末尾 → 最后卡片底部。
  // 目标总在拖拽视口内，getVirtualItems 可命中；视口外 → 不渲染（边缘情况）。
  const placeholderTop =
    placeholderIndex === undefined || placeholderIndex < 0
      ? undefined
      : (() => {
          const vitems = virtualizer.getVirtualItems()
          if (vitems.length === 0) return undefined
          if (placeholderIndex >= tasks.length) {
            const last = vitems[vitems.length - 1]
            return last.start + last.size - CARD_GAP
          }
          return vitems.find((v) => v.index === placeholderIndex)?.start
        })()

  return (
    <div
      ref={setNodeRef}
      className={cn(
        'flex h-full min-h-0 min-w-[280px] flex-1 flex-col rounded-[14px] bg-muted p-2.5',
        // 拖拽目标高亮：over 归属列（卡片间/空白区）或 isOver（指针直接命中列）→ 高亮边框
        (isOver || isDropTarget) && 'ring-2 ring-primary-300',
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

      <div ref={scrollRef} className="min-h-0 flex-1 overflow-auto pr-0.5">
        <SortableContext items={tasks.map((t) => t.id)} strategy={verticalListSortingStrategy}>
          <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
            {virtualizer.getVirtualItems().map((vi) => {
              const task = tasks[vi.index]
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
                  <TaskCard task={task} onOpen={onOpenTask} />
                </div>
              )
            })}
            {/* 拖拽目标指示线：绝对定位、不参与布局（避免测量反馈环） */}
            {placeholderTop !== undefined && (
              <div
                aria-hidden
                data-testid="drop-indicator"
                className="pointer-events-none absolute left-1 right-1 z-10"
                style={{ top: placeholderTop }}
              >
                <div className="h-[3px] rounded-full bg-primary-500 shadow-[0_0_0_2px_rgba(255,255,255,0.6)]" />
              </div>
            )}
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
