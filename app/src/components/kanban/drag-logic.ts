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
 * 传入目标列完整任务数组（**含 active**，active 卡片保留 DOM 隐形）+ over 命中项 + activeId，
 * 返回「含 active 列表」中的插入 index（占位符渲染位置）。
 * - over 命中卡片：同列保持相对顺序（从后拖到前 → 插 over 前；从前拖到后 → 插 over 后）；
 *   跨列 → 插 over 前。
 * - over 未命中（列空白/列头）→ 末尾。
 */
export function resolveOverIndex(colTasks: Task[], overId: string, activeId: string): number {
  const overIdx = colTasks.findIndex((t) => t.id === overId)
  if (overIdx === -1) return colTasks.length
  const activeIdx = colTasks.findIndex((t) => t.id === activeId)
  if (activeIdx === -1) return overIdx
  return overIdx < activeIdx ? overIdx : overIdx + 1
}

/**
 * 拖拽结束后的列内新顺序（纯函数，可单测）：
 * 传入目标列完整任务数组（含 active）+ over 命中项 + activeId，
 * 返回移除 active 后按目标插入位置重排的 id 数组
 * （本地保持列内顺序，避免数据刷新回跳造成的闪烁）。
 */
export function resolveColOrder(colTasks: Task[], overId: string, activeId: string): string[] {
  const target = resolveOverIndex(colTasks, overId, activeId) // 含 active 的插入点
  const rest = colTasks.filter((t) => t.id !== activeId).map((t) => t.id)
  const activeIdx = colTasks.findIndex((t) => t.id === activeId)
  // active 在目标列表中且位于插入点之前 → 移除后坐标前移一位
  const insert = activeIdx !== -1 && target > activeIdx ? target - 1 : target
  return [...rest.slice(0, insert), activeId, ...rest.slice(insert)]
}

/**
 * 按本地列内顺序覆盖重排任务列表（拖拽结束会话内保持顺序；未记录列保持原序）。
 * 纯函数，可单测。
 */
export function applyColumnOrder(
  tasks: Task[],
  order: Record<string, string[]>,
  statusOf: (t: Task) => string,
): Task[] {
  const groups = new Map<string, Task[]>()
  for (const t of tasks) {
    const s = statusOf(t)
    if (!groups.has(s)) groups.set(s, [])
    groups.get(s)!.push(t)
  }
  const out: Task[] = []
  for (const [status, group] of groups) {
    const ordered = order[status]
    if (!ordered || ordered.length === 0) {
      out.push(...group)
      continue
    }
    const byId = new Map(group.map((t) => [t.id, t]))
    const seen = new Set<string>()
    for (const id of ordered) {
      const t = byId.get(id)
      if (t && !seen.has(id)) {
        out.push(t)
        seen.add(id)
      }
    }
    for (const t of group) if (!seen.has(t.id)) out.push(t)
  }
  return out
}

/**
 * 树序稳定化（QA 2026-08-09）：同列任务按树 DFS 相对序重排——
 * 子任务与父任务同列时始终紧跟在父任务下方（「一直排列在父任务下方」）。
 * 应用在 applyColumnOrder 之后：拖拽仅流转状态，列内顺序由树层级决定。
 * 纯函数，可单测。
 */
export function stabilizeTreeOrder(
  tasks: Task[],
  treeIndex: Map<string, number>,
  statusOf: (t: Task) => string,
): Task[] {
  const groups = new Map<string, Task[]>()
  for (const t of tasks) {
    const s = statusOf(t)
    if (!groups.has(s)) groups.set(s, [])
    groups.get(s)!.push(t)
  }
  const out: Task[] = []
  for (const group of groups.values()) {
    const sorted = [...group].sort(
      (a, b) =>
        (treeIndex.get(a.id) ?? Number.MAX_SAFE_INTEGER) -
        (treeIndex.get(b.id) ?? Number.MAX_SAFE_INTEGER),
    )
    out.push(...sorted)
  }
  return out
}
