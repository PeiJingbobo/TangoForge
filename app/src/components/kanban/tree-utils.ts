import type { Task, TaskTreeNode } from '@/types/task'

/**
 * 树形任务 → 扁平列表（看板/搜索用）。
 * QA 2026-08-09：注入 level（DFS 深度，0=顶层）供看板卡片层级缩进。
 */
export function flattenTree(tree: TaskTreeNode[]): Task[] {
  const out: Task[] = []
  const walk = (nodes: TaskTreeNode[], depth: number): void => {
    for (const n of nodes) {
      out.push({ ...n, level: depth })
      walk(n.children, depth + 1)
    }
  }
  walk(tree, 0)
  return out
}

/**
 * 树 DFS 相对序索引（id → 全局 index，flattenTree 顺序）。
 * 供看板列内「子任务始终排列在父任务下方」稳定排序（QA 2026-08-09）。
 */
export function treeOrderIndex(tree: TaskTreeNode[]): Map<string, number> {
  const idx = new Map<string, number>()
  let i = 0
  const walk = (nodes: TaskTreeNode[]): void => {
    for (const n of nodes) {
      idx.set(n.id, i++)
      walk(n.children)
    }
  }
  walk(tree)
  return idx
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
