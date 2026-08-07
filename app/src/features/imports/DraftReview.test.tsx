import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { DraftReview } from './DraftReview'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { toast } from 'sonner'

const DETAIL = {
  id: 'd1',
  source_file: 'backlog.md',
  status: 'pending',
  task_count: 3,
  created_at: '2026-08-07T08:00:00+08:00',
  tasks: [
    {
      id: 'T1',
      title: '顶层任务 A',
      description: '描述A',
      status: 'todo',
      priority: 2,
      tags: [],
      assignee: '',
      depends_on: [],
      children: [],
    },
    {
      id: 'T2',
      title: '顶层任务 B',
      description: '',
      status: 'doing',
      priority: 4,
      tags: ['前端'],
      assignee: 'PB',
      depends_on: ['T1'],
      children: [
        {
          id: 'T3',
          title: '子任务 C',
          description: '',
          status: 'todo',
          priority: 0,
          tags: [],
          assignee: '',
          depends_on: [],
          children: [],
        },
      ],
    },
  ],
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/project/p/io']}>{children}</MemoryRouter>
    </QueryClientProvider>
  )
}

describe('DraftReview（草稿审阅）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tf' })
    vi.stubGlobal(
      'WebSocket',
      class {
        close(): void {}
      },
    )
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/import/drafts/d1`, () =>
        HttpResponse.json({ code: 0, data: DETAIL }),
      ),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('渲染审阅界面：返回/关闭/丢弃/确认导入 + 树形视图', async () => {
    render(<DraftReview draftId="d1" onExit={() => {}} />, { wrapper })
    await waitFor(() => expect(screen.getByText('草稿审阅')).toBeInTheDocument())
    expect(screen.getByText('backlog.md')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '返回草稿列表' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '关闭' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '丢弃' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '确认导入' })).toBeInTheDocument()
    // 树形视图：顶层 + 子任务
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '任务 顶层任务 A' })).toBeInTheDocument(),
    )
    expect(screen.getByRole('button', { name: '任务 顶层任务 B' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '任务 子任务 C' })).toBeInTheDocument()
  })

  it('时间线/状态分类视图可切换（依赖标题引用展示）', async () => {
    const user = userEvent.setup()
    render(<DraftReview draftId="d1" onExit={() => {}} />, { wrapper })
    await waitFor(() => expect(screen.getByText('顶层任务 A')).toBeInTheDocument())
    await user.click(screen.getByRole('tab', { name: '状态分类' }))
    await waitFor(() => expect(screen.getByText('待办')).toBeInTheDocument())
  })

  it('点击任务打开抽屉 → 编辑保存 → PUT 草稿任务树（含编辑后标题）', async () => {
    let putBody: unknown = null
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/import/drafts/d1/tasks`, async ({ request }) => {
        putBody = await request.json()
        return HttpResponse.json({ code: 0, data: { ok: true } })
      }),
    )
    const user = userEvent.setup()
    render(<DraftReview draftId="d1" onExit={() => {}} />, { wrapper })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '任务 顶层任务 A' })).toBeInTheDocument(),
    )
    await user.click(screen.getByRole('button', { name: '任务 顶层任务 A' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '编辑标题' })).toBeInTheDocument(),
    )
    await user.click(screen.getByRole('button', { name: '编辑标题' }))
    await user.clear(screen.getByRole('textbox', { name: '任务标题编辑' }))
    await user.type(screen.getByRole('textbox', { name: '任务标题编辑' }), '顶层任务A-改')
    await user.keyboard('{Enter}')
    await user.click(screen.getByRole('button', { name: '保存草稿任务' }))
    await waitFor(() => expect(putBody).not.toBeNull())
    expect(putBody).toEqual(
      expect.objectContaining({
        tasks: expect.arrayContaining([
          expect.objectContaining({ title: '顶层任务A-改', status: 'todo', priority: 2 }),
        ]),
      }),
    )
  })

  it('依赖经临时 id 引用：打开任务抽屉展示被依赖任务标题（与标题解耦）', async () => {
    const user = userEvent.setup()
    render(<DraftReview draftId="d1" onExit={() => {}} />, { wrapper })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '任务 顶层任务 B' })).toBeInTheDocument(),
    )
    await user.click(screen.getByRole('button', { name: '任务 顶层任务 B' }))
    // 依赖区展示被依赖任务标题（T1 → 顶层任务 A）
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '编辑标题' })).toBeInTheDocument(),
    )
    expect(screen.getByRole('button', { name: '移除依赖 顶层任务 A' })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: /添加依赖/ })).toBeInTheDocument()
  })

  it('确认导入 → toast + onExit', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    const onExit = vi.fn()
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/import/drafts/d1/confirm`, () =>
        HttpResponse.json({
          code: 0,
          data: { draft_id: 'd1', source_file: 'backlog.md', created: 3, archived: 0 },
        }),
      ),
    )
    const user = userEvent.setup()
    render(<DraftReview draftId="d1" onExit={onExit} />, { wrapper })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '确认导入' })).toBeInTheDocument(),
    )
    await user.click(screen.getByRole('button', { name: '确认导入' }))
    await waitFor(() => expect(onExit).toBeCalled())
    toastSpy.mockRestore()
  })

  it('丢弃草稿 → onExit', async () => {
    const onExit = vi.fn()
    const confirmSpy = vi.spyOn(window, 'confirm').mockImplementation(() => true)
    server.use(
      http.delete(`${DAEMON_BASE_URL}/api/import/drafts/d1`, () =>
        HttpResponse.json({ code: 0, data: { ok: true } }),
      ),
    )
    const user = userEvent.setup()
    render(<DraftReview draftId="d1" onExit={onExit} />, { wrapper })
    await waitFor(() => expect(screen.getByRole('button', { name: '丢弃' })).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: '丢弃' }))
    await waitFor(() => expect(onExit).toBeCalled())
    confirmSpy.mockRestore()
  })
})
