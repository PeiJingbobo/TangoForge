import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { TaskForm } from './TaskForm'
import type { StateMachineState } from '@/types/models'
import type { Task } from '@/types/task'

const STATES: StateMachineState[] = [
  { Key: 'todo', Label: '待办', Color: '#999' },
  { Key: 'doing', Label: '进行中', Color: '#2292d8' },
  { Key: 'done', Label: '已完成', Color: '#22c55e' },
]

function mk(id: string, over: Partial<Task> = {}): Task {
  return {
    id,
    project_id: 1,
    parent_id: null,
    title: `任务 ${id}`,
    description: '描述内容',
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
    ...over,
  }
}

const TASK_A = mk('a', { tags: ['前端'], depends_on: ['b'] })
const ALL = [TASK_A, mk('b')]

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient()
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('TaskForm（行内编辑）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('渲染阅读流：标题/描述/标签/依赖', () => {
    render(<TaskForm task={TASK_A} states={STATES} allTasks={ALL} onSubmit={() => {}} />, {
      wrapper,
    })
    expect(screen.getByRole('button', { name: '编辑标题' })).toHaveTextContent('任务 a')
    expect(screen.getByRole('button', { name: '编辑描述' })).toHaveTextContent('描述内容')
    expect(screen.getByText('前端')).toBeInTheDocument()
    expect(screen.getByText('任务 b')).toBeInTheDocument() // 依赖 chip
  })

  it('标题行内编辑：修改后保存按钮提交 title', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<TaskForm task={TASK_A} states={STATES} allTasks={ALL} onSubmit={onSubmit} />, {
      wrapper,
    })
    await user.click(screen.getByRole('button', { name: '编辑标题' }))
    const input = screen.getByRole('textbox', { name: '任务标题编辑' })
    await user.clear(input)
    await user.type(input, '新标题')
    await user.keyboard('{Enter}')
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ title: '新标题' }))
  })

  it('无改动时保存按钮禁用', () => {
    render(<TaskForm task={TASK_A} states={STATES} allTasks={ALL} onSubmit={() => {}} />, {
      wrapper,
    })
    expect(screen.getByRole('button', { name: '已保存' })).toBeDisabled()
  })

  it('标签添加/移除', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<TaskForm task={TASK_A} states={STATES} allTasks={ALL} onSubmit={onSubmit} />, {
      wrapper,
    })
    await user.click(screen.getByRole('button', { name: '添加标签' }))
    await user.type(screen.getByRole('textbox', { name: '新标签' }), '交互{Enter}')
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ tags: ['前端', '交互'] }))
  })

  it('依赖添加：选择任务后提交 depends_on', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    const taskC = mk('c')
    render(
      <TaskForm task={taskC} states={STATES} allTasks={[taskC, TASK_A]} onSubmit={onSubmit} />,
      { wrapper },
    )
    await user.click(screen.getByRole('combobox', { name: /添加依赖/ }))
    await user.click(await screen.findByText('任务 a'))
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ depends_on: ['a'] }))
  })
})
