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
    useTaskDrawerStore.setState({ stack: [] })
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
      http.get(`${DAEMON_BASE_URL}/api/tasks/task-2`, () =>
        HttpResponse.json({
          code: 0,
          data: { ...TASK_A, id: 'task-2', title: '关联任务乙' },
        }),
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
    useTaskDrawerStore.setState({ stack: [] })
  })

  it('openDrawer({taskId}) → 渲染任务抽屉', async () => {
    render(<GlobalTaskDrawer />, { wrapper })
    expect(screen.queryByText('前端 token 接入')).not.toBeInTheDocument()
    useTaskDrawerStore.getState().openDrawer({ taskId: 'task-1' })
    await waitFor(() => expect(screen.getByText('前端 token 接入')).toBeInTheDocument())
  })

  it('详情内 pushTask 压入新层 Dialog：返回（pop）后还原上一个任务', async () => {
    const user = userEvent.setup()
    render(<GlobalTaskDrawer />, { wrapper })
    useTaskDrawerStore.getState().openDrawer({ taskId: 'task-1' })
    await waitFor(() => expect(screen.getByText('前端 token 接入')).toBeInTheDocument())
    // 详情内打开关联任务 → 压入新层（新 Dialog 展示关联任务）
    useTaskDrawerStore.getState().pushTask({ taskId: 'task-2' })
    await waitFor(() => expect(screen.getByText('关联任务乙')).toBeInTheDocument())
    // 点击顶层返回箭头 → 弹出新层，还原上一个任务
    const backButtons = screen.getAllByRole('button', { name: '返回关闭详情' })
    await user.click(backButtons[backButtons.length - 1])
    await waitFor(() => expect(screen.getByText('前端 token 接入')).toBeInTheDocument())
    expect(screen.queryByText('关联任务乙')).not.toBeInTheDocument()
  })

  it('堆栈层级 z-index 递增：新层遮罩/内容高于旧层，内容高于自身遮罩', async () => {
    render(<GlobalTaskDrawer />, { wrapper })
    useTaskDrawerStore.getState().openDrawer({ taskId: 'task-1' })
    await waitFor(() => expect(screen.getByText('前端 token 接入')).toBeInTheDocument())
    const rootOverlay = document.querySelector('[data-slot="sheet-overlay"]')
    const rootContent = document.querySelector('[data-slot="sheet-content"]')
    // 根层：遮罩 50，内容 51（内容高于自身遮罩）
    expect(rootOverlay?.getAttribute('style')).toContain('z-index: 50')
    expect(rootContent?.getAttribute('style')).toContain('z-index: 51')

    useTaskDrawerStore.getState().pushTask({ taskId: 'task-2' })
    await waitFor(() => expect(screen.getByText('关联任务乙')).toBeInTheDocument())
    const overlays = document.querySelectorAll('[data-slot="sheet-overlay"]')
    const contents = document.querySelectorAll('[data-slot="sheet-content"]')
    // 新层（stack 末尾）：遮罩 60、内容 61，均高于根层
    expect(overlays[overlays.length - 1].getAttribute('style')).toContain('z-index: 60')
    expect(contents[contents.length - 1].getAttribute('style')).toContain('z-index: 61')
  })

  it('popTask 关闭动画：顶层先标记 open=false 保持挂载，动画结束后移除', async () => {
    // 真实计时器完成数据加载与首层渲染
    render(<GlobalTaskDrawer />, { wrapper })
    useTaskDrawerStore.getState().openDrawer({ taskId: 'task-1' })
    useTaskDrawerStore.getState().pushTask({ taskId: 'task-2' })
    await waitFor(() => expect(screen.getByText('关联任务乙')).toBeInTheDocument())

    // 切假计时器：验证关闭层延迟移除
    vi.useFakeTimers()
    useTaskDrawerStore.getState().popTask()
    let stack = useTaskDrawerStore.getState().stack
    expect(stack.length).toBe(2)
    expect(stack[0].open).toBe(true)
    expect(stack[1].open).toBe(false)

    vi.advanceTimersByTime(400)
    stack = useTaskDrawerStore.getState().stack
    expect(stack.length).toBe(1)
    expect(stack[0].taskId).toBe('task-1')
    expect(stack[0].open).toBe(true)
    vi.useRealTimers()
  })
})
