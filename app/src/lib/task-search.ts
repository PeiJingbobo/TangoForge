import type { Task } from '@/types/task'

/**
 * 任务搜索匹配（TF-042）：同时匹配 **任务编号 number / 标题 title / 内容 description**，
 * 大小写不敏感；空 query 恒匹配。看板与任务导航（树形/时间线/状态分类）共用。
 */
export function matchesTaskQuery(
  t: Pick<Task, 'number' | 'title' | 'description'>,
  query: string,
): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  return (
    t.title.toLowerCase().includes(q) ||
    t.number.toLowerCase().includes(q) ||
    t.description.toLowerCase().includes(q)
  )
}
