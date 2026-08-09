import { describe, it, expect } from 'vitest'
import { flattenTree, patchTaskInTree, treeOrderIndex } from './tree-utils'
import type { Task, TaskTreeNode } from '@/types/task'

function mk(id: string, status: string, children: TaskTreeNode[] = []): TaskTreeNode {
  return {
    id,
    project_id: 1,
    parent_id: null,
    title: id,
    number: '',
    description: '',
    status,
    priority: 0,
    tags: [],
    assignee: '',
    depends_on: [],
    archived_from: '',
    source_file: '',
    source_section: '',
    created_at: '2026-08-06T10:00:00+08:00',
    updated_at: '2026-08-06T10:00:00+08:00',
    children,
  }
}

describe('tree-utils', () => {
  it('flattenTree 展平全部层级', () => {
    const tree = [
      mk('a', 'todo', [mk('a1', 'todo'), mk('a2', 'doing', [mk('a2x', 'done')])]),
      mk('b', 'done'),
    ]
    const flat = flattenTree(tree)
    expect(flat.map((t) => t.id)).toEqual(['a', 'a1', 'a2', 'a2x', 'b'])
  })

  it('flattenTree 注入 level（QA 2026-08-09：看板层级缩进）', () => {
    const tree = [
      mk('a', 'todo', [mk('a1', 'todo'), mk('a2', 'doing', [mk('a2x', 'done')])]),
      mk('b', 'done'),
    ]
    const flat = flattenTree(tree)
    expect(flat.map((t) => [t.id, t.level])).toEqual([
      ['a', 0],
      ['a1', 1],
      ['a2', 1],
      ['a2x', 2],
      ['b', 0],
    ])
  })

  it('treeOrderIndex 返回树 DFS 序索引', () => {
    const tree = [
      mk('a', 'todo', [mk('a1', 'todo'), mk('a2', 'doing', [mk('a2x', 'done')])]),
      mk('b', 'done'),
    ]
    const idx = treeOrderIndex(tree)
    expect(idx.get('a')).toBe(0)
    expect(idx.get('a2')).toBe(2)
    expect(idx.get('a2x')).toBe(3)
    expect(idx.get('b')).toBe(4)
  })

  it('patchTaskInTree 深层打补丁且不修改原树', () => {
    const tree = [mk('a', 'todo', [mk('a1', 'todo')])]
    const next = patchTaskInTree(tree, 'a1', { status: 'done' })
    expect(next[0].children[0].status).toBe('done')
    expect(tree[0].children[0].status).toBe('todo') // 原树不变
  })

  it('patch 未命中 id 返回原树结构', () => {
    const tree = [mk('a', 'todo')]
    const next = patchTaskInTree(tree, 'nope', { status: 'done' } as Partial<Task>)
    expect(next).toEqual(tree)
  })
})
