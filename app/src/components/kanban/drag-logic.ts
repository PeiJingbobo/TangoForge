import type { Task } from '@/types/task'

/**
 * 拖拽目标解析（纯函数，可单测）：
 * - over 命中列 id → 目标为该列；
 * - over 命中任务卡片 → 目标为该卡片所在列；
 * - 目标与来源同列 → null（无操作）。
 */
export function resolveDragTarget(
  activeId: string,
  overId: string,
  tasksById: Map<string, Task>,
  columnKeys: string[],
): { taskId: string; to: string } | null {
  const task = tasksById.get(activeId)
  if (!task) return null

  let to: string
  if (columnKeys.includes(overId)) {
    to = overId
  } else {
    const overTask = tasksById.get(overId)
    if (!overTask) return null
    to = overTask.status
  }

  if (to === task.status) return null
  return { taskId: task.id, to }
}

/**
 * 拖拽目标插入位置（纯函数，可单测）：
 * 传入目标列完整任务数组（**含 active**）+ over 命中项 + activeId，
 * 返回「移除 active 后」的列内插入 index（占位符渲染位置）。
 * - over 命中卡片：同列保持相对顺序（从后拖到前 → 插 over 前；从前拖到后 → 插 over 后）；
 *   跨列 → 插 over 前。
 * - over 未命中（列空白/列头）→ 末尾。
 */
export function resolveOverIndex(colTasks: Task[], overId: string, activeId: string): number {
  const overIdx = colTasks.findIndex((t) => t.id === overId)
  if (overIdx === -1) return colTasks.filter((t) => t.id !== activeId).length
  const activeIdx = colTasks.findIndex((t) => t.id === activeId)
  let target = overIdx
  if (activeIdx !== -1) {
    target = overIdx < activeIdx ? overIdx : overIdx + 1
  }
  const list = colTasks.filter((t) => t.id !== activeId)
  const idx = activeIdx !== -1 && target > activeIdx ? target - 1 : target
  return Math.max(0, Math.min(idx, list.length))
}
