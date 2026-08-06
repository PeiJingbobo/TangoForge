import type { Task, TaskTreeNode } from '@/types/task'

/** 树形任务 → 扁平列表（看板/搜索用） */
export function flattenTree(tree: TaskTreeNode[]): Task[] {
  const out: Task[] = []
  const walk = (nodes: TaskTreeNode[]): void => {
    for (const n of nodes) {
      out.push(n)
      walk(n.children)
    }
  }
  walk(tree)
  return out
}

/** 树形任务中按 id 打补丁（乐观更新/回滚用；返回新树，不修改原树） */
export function patchTaskInTree(
  tree: TaskTreeNode[],
  id: string,
  patch: Partial<Task>,
): TaskTreeNode[] {
  return tree.map((n) => {
    if (n.id === id) return { ...n, ...patch }
    return { ...n, children: patchTaskInTree(n.children, id, patch) }
  })
}
