import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { TaskDetail } from './TaskDetail'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { toast } from 'sonner'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/project/p/tasks/a']}>{children}</MemoryRouter>
    </QueryClientProvider>
  )
}

const TASK_A = {
  id: 'task-1',
  project_id: 1,
  parent_id: null,
  title: '前端 token 接入',
  description: '描述内容',
  status: 'todo',
  priority: 4,
  tags: ['前端'],
  assignee: 'PB',
  depends_on: ['task-2'],
  archived_from: '',
  source_file: '',
  source_section: '',
  created_at: '2026-08-06T10:00:00+08:00',
  updated_at: '2026-08-06T10:00:00+08:00',
}

describe('TaskDetail', () => {
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
  })

  it('渲染阅读流：标题/描述/meta', async () => {
    render(<TaskDetail taskId="task-1" />, { wrapper })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '编辑标题' })).toHaveTextContent('前端 token 接入'),
    )
    expect(screen.getByRole('button', { name: '编辑描述' })).toHaveTextContent('描述内容')
    expect(screen.getByRole('combobox', { name: '状态' })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: '优先级' })).toBeInTheDocument()
  })

  it('修改字段保存 → PATCH 成功 toast', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    server.use(
      http.patch(`${DAEMON_BASE_URL}/api/tasks/task-1`, () =>
        HttpResponse.json({ code: 0, data: { ...TASK_A, title: '新标题' } }),
      ),
    )
    const user = userEvent.setup()
    render(<TaskDetail taskId="task-1" />, { wrapper })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '编辑标题' })).toBeInTheDocument(),
    )
    await user.click(screen.getByRole('button', { name: '编辑标题' }))
    await user.clear(screen.getByRole('textbox', { name: '任务标题编辑' }))
    await user.type(screen.getByRole('textbox', { name: '任务标题编辑' }), '新标题')
    await user.keyboard('{Enter}')
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
  })

  it('依赖环：CIRCULAR_DEPENDENCY → toast 展示无环提示', async () => {
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
    render(<TaskDetail taskId="task-1" />, { wrapper })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '编辑标题' })).toBeInTheDocument(),
    )
    // 修改标题触发提交（PATCH 恒返回 CIRCULAR_DEPENDENCY）
    await user.click(screen.getByRole('button', { name: '编辑标题' }))
    await user.clear(screen.getByRole('textbox', { name: '任务标题编辑' }))
    await user.type(screen.getByRole('textbox', { name: '任务标题编辑' }), '触发依赖环')
    await user.keyboard('{Enter}')
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    const message = toastSpy.mock.calls[0]?.[0]
    expect(String(message)).toContain('循环依赖')
    const description = toastSpy.mock.calls[0]?.[1] as { description?: string }
    expect(description?.description).toContain('循环依赖')
    toastSpy.mockRestore()
  })
})
