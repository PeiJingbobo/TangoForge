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
