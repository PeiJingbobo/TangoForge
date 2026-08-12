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
    number: '',
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

  it('搜索：按任务编号匹配（TF-042）', async () => {
    const user = userEvent.setup()
    const tree = [{ ...mk('b', '独立任务 B'), number: 'T005' }]
    render(<TreeNav tree={tree} onSelect={() => {}} />)
    await user.type(screen.getByRole('textbox', { name: '搜索任务树' }), 'T005')
    expect(screen.getByRole('button', { name: '任务 独立任务 B' })).toBeInTheDocument()
  })

  it('搜索：按任务内容匹配（TF-042）', async () => {
    const user = userEvent.setup()
    const tree = [{ ...mk('b', '独立任务 B'), description: '包含鉴权流程说明' }]
    render(<TreeNav tree={tree} onSelect={() => {}} />)
    await user.type(screen.getByRole('textbox', { name: '搜索任务树' }), '鉴权')
    expect(screen.getByRole('button', { name: '任务 独立任务 B' })).toBeInTheDocument()
  })

  it('点击任务触发 onSelect', async () => {
    const onSelect = vi.fn()
    const user = userEvent.setup()
    render(<TreeNav tree={TREE} onSelect={onSelect} />)
    await user.click(screen.getByRole('button', { name: '任务 独立任务 B' }))
    expect(onSelect).toHaveBeenCalledWith('b')
  })

  it('传入 states 时任务行展示状态机彩色状态点（QA 2026-08-09）', () => {
    const tree = [{ ...mk('a', '任务甲'), status: 'doing' }]
    render(
      <TreeNav
        tree={tree}
        states={[
          { Key: 'todo', Label: '待办', Color: '#9aa0a6' },
          { Key: 'doing', Label: '进行中', Color: '#1a73e8' },
        ]}
        onSelect={() => {}}
      />,
    )
    const dot = document.querySelector('span[style*="background-color: rgb(26, 115, 232)"]')
    expect(dot).not.toBeNull()
  })

  it('任务行展示状态名称、优先级与标签（任务导航优化）', () => {
    const tree = [
      {
        ...mk('a', '任务甲'),
        status: 'doing',
        priority: 4,
        tags: ['前端', '交互', '多余标签'],
      },
    ]
    render(
      <TreeNav
        tree={tree}
        states={[
          { Key: 'todo', Label: '待办', Color: '#9aa0a6' },
          { Key: 'doing', Label: '进行中', Color: '#1a73e8' },
        ]}
        onSelect={() => {}}
      />,
    )
    // 状态名称（StateBadge label）
    expect(screen.getByText('进行中')).toBeInTheDocument()
    // 优先级
    expect(screen.getByText('P4')).toBeInTheDocument()
    // 标签（≤2 个展示）
    expect(screen.getByText('#前端')).toBeInTheDocument()
    expect(screen.getByText('#交互')).toBeInTheDocument()
    expect(screen.queryByText('#多余标签')).not.toBeInTheDocument()
  })
})
