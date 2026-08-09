import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useRef, useState, type ReactNode } from 'react'
import { TaskForm, type TaskFormHandle } from './TaskForm'
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
    number: '',
    description: '描述内容',
    status: 'todo',
    priority: 0,
    tags: [],
    assignee: '',
    depends_on: [],
    archived_from: '',
    source_file: '',
    source_section: '',
    created_at: '2026-08-07T10:00:00+08:00',
    updated_at: '2026-08-07T10:00:00+08:00',
    ...over,
  }
}

const TASK_A = mk('a', { tags: ['前端'], depends_on: ['b'] })
const ALL = [TASK_A, mk('b')]

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient()
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

/** 宿主模拟（抽屉 footer 语义）：ref 提交 + dirty 状态 */
function Harness({
  task = TASK_A,
  readOnly = false,
  onSubmit,
}: {
  task?: Task
  readOnly?: boolean
  onSubmit?: (body: unknown) => void
}) {
  const ref = useRef<TaskFormHandle>(null)
  const [dirty, setDirty] = useState(false)
  return (
    <>
      <TaskForm
        ref={ref}
        task={task}
        states={STATES}
        allTasks={ALL}
        readOnly={readOnly}
        onSubmit={onSubmit ?? (() => {})}
        onDirtyChange={setDirty}
      />
      <button type="button" aria-label="测试提交" onClick={() => ref.current?.submit()}>
        提交
      </button>
      <span data-testid="dirty">{String(dirty)}</span>
    </>
  )
}

describe('TaskForm（内容表单：footer 由宿主渲染）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('渲染阅读流：标题/描述/标签/依赖', () => {
    render(<Harness />, { wrapper })
    expect(screen.getByRole('button', { name: '编辑标题' })).toHaveTextContent('任务 a')
    expect(screen.getByRole('button', { name: '编辑描述' })).toHaveTextContent('描述内容')
    expect(screen.getByText('前端')).toBeInTheDocument()
    expect(screen.getByText('任务 b')).toBeInTheDocument() // 依赖 chip
    // footer 不在内容区（由宿主渲染）
    expect(screen.queryByRole('button', { name: /保存/ })).not.toBeInTheDocument()
  })

  it('标题行内编辑：修改后 dirty=true，ref.submit 提交差异', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<Harness onSubmit={onSubmit} />, { wrapper })
    await user.click(screen.getByRole('button', { name: '编辑标题' }))
    const input = screen.getByRole('textbox', { name: '任务标题编辑' })
    await user.clear(input)
    await user.type(input, '新标题')
    await user.keyboard('{Enter}')
    expect(screen.getByTestId('dirty')).toHaveTextContent('true')
    await user.click(screen.getByRole('button', { name: '测试提交' }))
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ title: '新标题' }))
  })

  it('子任务展示编号与状态徽标（QA 2026-08-09）', () => {
    const child = mk('c', { parent_id: 'a', title: '子任务甲', number: 'T006', status: 'doing' })
    render(
      <TaskForm
        task={TASK_A}
        states={STATES}
        allTasks={[TASK_A, child]}
        onSubmit={() => {}}
        onDirtyChange={() => {}}
      />,
      { wrapper },
    )
    expect(screen.getByText('子任务（1）')).toBeInTheDocument()
    expect(screen.getByText('子任务甲')).toBeInTheDocument()
    expect(screen.getByText('T006')).toBeInTheDocument() // 编号徽标
    expect(screen.getAllByText('进行中').length).toBeGreaterThan(0) // 状态徽标
  })

  it('无改动时 dirty=false', () => {
    render(<Harness />, { wrapper })
    expect(screen.getByTestId('dirty')).toHaveTextContent('false')
  })

  it('标签添加：dirty=true 且提交包含 tags', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<Harness onSubmit={onSubmit} />, { wrapper })
    await user.click(screen.getByRole('button', { name: '添加标签' }))
    await user.type(screen.getByRole('textbox', { name: '新标签' }), '交互{Enter}')
    expect(screen.getByTestId('dirty')).toHaveTextContent('true')
    await user.click(screen.getByRole('button', { name: '测试提交' }))
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ tags: ['前端', '交互'] }))
  })

  it('依赖添加：选择任务后提交 depends_on', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    const taskC = mk('c')
    const ALL3 = [taskC, TASK_A, mk('b')]
    const Harness3 = () => {
      const ref = useRef<TaskFormHandle>(null)
      const [dirty, setDirty] = useState(false)
      return (
        <>
          <TaskForm
            ref={ref}
            task={taskC}
            states={STATES}
            allTasks={ALL3}
            onSubmit={onSubmit}
            onDirtyChange={setDirty}
          />
          <button type="button" aria-label="测试提交" onClick={() => ref.current?.submit()}>
            提交
          </button>
          <span data-testid="dirty">{String(dirty)}</span>
        </>
      )
    }
    render(<Harness3 />, { wrapper })
    await user.click(screen.getByRole('combobox', { name: /添加依赖/ }))
    await user.click(await screen.findByText('任务 a'))
    await user.click(screen.getByRole('button', { name: '测试提交' }))
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ depends_on: ['a'] }))
  })

  it('只读模式：无编辑入口', () => {
    render(<Harness readOnly />, { wrapper })
    expect(screen.queryByRole('button', { name: '编辑标题' })).not.toBeInTheDocument()
    expect(screen.getByText('任务 a')).toBeInTheDocument()
  })
})
