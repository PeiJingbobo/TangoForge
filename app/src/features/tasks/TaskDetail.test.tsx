import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { TaskDetailDrawer, GlobalTaskDrawer } from './TaskDetail'
import { useProjectStore } from '@/stores/project'
import { useTaskDrawerStore } from '@/stores/task-drawer'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { toast } from 'sonner'
import type { Task } from '@/types/task'

const TASK_A: Task = {
  id: 'task-1',
  project_id: 1,
  parent_id: null,
  title: '前端 token 接入',
  number: 'T01',
  description: '描述内容',
  status: 'todo',
  priority: 4,
  tags: ['前端'],
  assignee: 'PB',
  depends_on: [],
  archived_from: '',
  source_file: '',
  source_section: '',
  created_at: '2026-08-07T08:00:00+08:00',
  updated_at: '2026-08-07T08:00:00+08:00',
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/project/p/kanban']}>{children}</MemoryRouter>
    </QueryClientProvider>
  )
}

describe('TaskDetailDrawer（全局抽屉）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tf' })
    vi.stubGlobal(
      'WebSocket',
      class {
        close(): void {}
      },
    )
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/tasks/task-1`, () =>
        HttpResponse.json({ code: 0, data: TASK_A }),
      ),
      http.get(`${DAEMON_BASE_URL}/api/tasks`, () =>
        HttpResponse.json({
          code: 0,
          data: { tree: [{ ...TASK_A, children: [] }], total: 1, page: 0, size: 0 },
        }),
      ),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('taskId 模式：加载详情 + 编辑保存（内部 PATCH → onSaved 回调最新详情）', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    const onSaved = vi.fn()
    server.use(
      http.patch(`${DAEMON_BASE_URL}/api/tasks/task-1`, () =>
        HttpResponse.json({ code: 0, data: { ...TASK_A, title: '新标题' } }),
      ),
    )
    const user = userEvent.setup()
    render(<TaskDetailDrawer open onOpenChange={() => {}} taskId="task-1" onSaved={onSaved} />, {
      wrapper,
    })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '编辑标题' })).toHaveTextContent('前端 token 接入'),
    )
    await user.click(screen.getByRole('button', { name: '编辑标题' }))
    await user.clear(screen.getByRole('textbox', { name: '任务标题编辑' }))
    await user.type(screen.getByRole('textbox', { name: '任务标题编辑' }), '新标题')
    await user.keyboard('{Enter}')
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(onSaved).toBeCalled())
    expect((onSaved.mock.calls[0][0] as Task).title).toBe('新标题')
    toastSpy.mockRestore()
  })

  it('task 对象模式：编辑保存经 onSaved 回调最新详情（不内部发请求）', async () => {
    const onSaved = vi.fn()
    let patchCalled = false
    server.use(
      http.patch(`${DAEMON_BASE_URL}/api/tasks/task-1`, () => {
        patchCalled = true
        return HttpResponse.json({ code: 0, data: TASK_A })
      }),
    )
    const user = userEvent.setup()
    render(<TaskDetailDrawer open onOpenChange={() => {}} task={TASK_A} onSaved={onSaved} />, {
      wrapper,
    })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '编辑标题' })).toHaveTextContent('前端 token 接入'),
    )
    await user.click(screen.getByRole('button', { name: '编辑标题' }))
    await user.clear(screen.getByRole('textbox', { name: '任务标题编辑' }))
    await user.type(screen.getByRole('textbox', { name: '任务标题编辑' }), '对象模式标题')
    await user.keyboard('{Enter}')
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(onSaved).toBeCalled())
    expect((onSaved.mock.calls[0][0] as Task).title).toBe('对象模式标题')
    expect(patchCalled).toBe(false) // 未发请求
  })

  it('只读模式：无编辑入口与保存按钮', async () => {
    render(<TaskDetailDrawer open onOpenChange={() => {}} task={TASK_A} mode="read" />, { wrapper })
    await waitFor(() => expect(screen.getByText('前端 token 接入')).toBeInTheDocument())
    expect(screen.queryByRole('button', { name: '编辑标题' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '保存修改' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '归档' })).not.toBeInTheDocument()
  })

  it('依赖环：taskId 模式保存失败 → toast 无环提示', async () => {
    const toastSpy = vi.spyOn(toast, 'error').mockImplementation(() => '')
    server.use(
      http.patch(`${DAEMON_BASE_URL}/api/tasks/task-1`, () =>
        HttpResponse.json(
          { code: 'CIRCULAR_DEPENDENCY', message: '循环依赖检测：a → b → a', detail: '' },
          { status: 422 },
        ),
      ),
    )
    const user = userEvent.setup()
    render(<TaskDetailDrawer open onOpenChange={() => {}} taskId="task-1" />, { wrapper })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '编辑标题' })).toBeInTheDocument(),
    )
    await user.click(screen.getByRole('button', { name: '编辑标题' }))
    await user.clear(screen.getByRole('textbox', { name: '任务标题编辑' }))
    await user.type(screen.getByRole('textbox', { name: '任务标题编辑' }), '触发依赖环')
    await user.keyboard('{Enter}')
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
  })
})

describe('GlobalTaskDrawer（store 桥接）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tf' })
    useTaskDrawerStore.setState({ open: false, taskId: undefined, task: undefined })
    vi.stubGlobal(
      'WebSocket',
      class {
        close(): void {}
      },
    )
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/tasks/task-1`, () =>
        HttpResponse.json({ code: 0, data: TASK_A }),
      ),
      http.get(`${DAEMON_BASE_URL}/api/tasks`, () =>
        HttpResponse.json({
          code: 0,
          data: { tree: [{ ...TASK_A, children: [] }], total: 1, page: 0, size: 0 },
        }),
      ),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    useTaskDrawerStore.setState({ open: false, taskId: undefined, task: undefined })
  })

  it('openDrawer({taskId}) → 渲染任务抽屉', async () => {
    render(<GlobalTaskDrawer />, { wrapper })
    expect(screen.queryByText('前端 token 接入')).not.toBeInTheDocument()
    useTaskDrawerStore.getState().openDrawer({ taskId: 'task-1' })
    await waitFor(() => expect(screen.getByText('前端 token 接入')).toBeInTheDocument())
  })
})
