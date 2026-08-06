import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TreeNav } from './TreeNav'
import type { TaskTreeNode } from '@/types/task'

function mk(id: string, title: string, children: TaskTreeNode[] = []): TaskTreeNode {
  return {
    id,
    project_id: 1,
    parent_id: null,
    title,
    description: '',
    status: 'todo',
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

const TREE = [
  mk('a', '父任务 A', [mk('a1', '子任务 A1'), mk('a2', '子任务 A2')]),
  mk('b', '独立任务 B'),
]

describe('TreeNav（树形导航）', () => {
  it('渲染层级结构（与后端树一致）', () => {
    render(<TreeNav tree={TREE} onSelect={() => {}} />)
    expect(screen.getByRole('button', { name: '任务 父任务 A' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '任务 子任务 A1' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '任务 独立任务 B' })).toBeInTheDocument()
  })

  it('折叠后隐藏子任务，再次点击展开', async () => {
    const user = userEvent.setup()
    render(<TreeNav tree={TREE} onSelect={() => {}} />)
    await user.click(screen.getByRole('button', { name: '折叠' }))
    expect(screen.queryByRole('button', { name: '任务 子任务 A1' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '展开' }))
    expect(screen.getByRole('button', { name: '任务 子任务 A1' })).toBeInTheDocument()
  })

  it('搜索过滤：不匹配的任务隐藏', async () => {
    const user = userEvent.setup()
    render(<TreeNav tree={TREE} onSelect={() => {}} />)
    await user.type(screen.getByRole('textbox', { name: '搜索任务树' }), '独立')
    expect(screen.queryByRole('button', { name: '任务 父任务 A' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '任务 独立任务 B' })).toBeInTheDocument()
  })

  it('点击任务触发 onSelect', async () => {
    const onSelect = vi.fn()
    const user = userEvent.setup()
    render(<TreeNav tree={TREE} onSelect={onSelect} />)
    await user.click(screen.getByRole('button', { name: '任务 独立任务 B' }))
    expect(onSelect).toHaveBeenCalledWith('b')
  })
})
