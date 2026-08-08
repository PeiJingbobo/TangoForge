import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { ImportStep } from './ImportStep'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'

const WORKDIR = '/data/projects/tf'
const DRAFT = {
  id: 'd1',
  source_file: 'backlog.md',
  status: 'pending' as const,
  task_count: 2,
  created_at: '2026-08-08T08:00:00+08:00',
}
const DETAIL = {
  ...DRAFT,
  tasks: [
    {
      id: 'T1',
      title: '任务甲',
      description: '',
      status: 'todo',
      priority: 1,
      tags: [],
      assignee: '',
      depends_on: [],
      children: [],
    },
    {
      id: 'T2',
      title: '任务乙',
      description: '',
      status: 'doing',
      priority: 3,
      tags: [],
      assignee: '',
      depends_on: [],
      children: [],
    },
  ],
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/']}>{children}</MemoryRouter>
    </QueryClientProvider>
  )
}

function mockDialog() {
  const selectFiles = vi.fn().mockResolvedValue(['/a/01.md', '/a/02.md'])
  const selectDirectory = vi.fn().mockResolvedValue('/a')
  Object.defineProperty(window, 'tangoforge', {
    value: { dialog: { selectFiles, selectDirectory } },
    configurable: true,
  })
  return { selectFiles, selectDirectory }
}

describe('ImportStep（引导 Step 2：导入草稿）', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'WebSocket',
      class {
        close(): void {}
      },
    )
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/import`, () => HttpResponse.json({ code: 0, data: DRAFT })),
      http.get(`${DAEMON_BASE_URL}/api/import/drafts/d1`, () =>
        HttpResponse.json({ code: 0, data: DETAIL }),
      ),
      http.post(`${DAEMON_BASE_URL}/api/import/drafts/d1/confirm`, () =>
        HttpResponse.json({
          code: 0,
          data: { created: 2, archived: 0, dropped_deps: 0, source_file: 'backlog.md' },
        }),
      ),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('选择文件后展示已选列表（可移除）', async () => {
    mockDialog()
    const user = userEvent.setup()
    render(<ImportStep workdir={WORKDIR} onReady={() => {}} />, { wrapper })
    await user.click(screen.getByRole('button', { name: '选择 Markdown 文件' }))
    await waitFor(() => expect(screen.getByText('/a/01.md')).toBeInTheDocument())
    expect(screen.getByText('/a/02.md')).toBeInTheDocument()
    // 移除一个
    await user.click(screen.getByRole('button', { name: '移除 /a/01.md' }))
    await waitFor(() => expect(screen.queryByText('/a/01.md')).not.toBeInTheDocument())
    expect(screen.getByText('/a/02.md')).toBeInTheDocument()
  })

  it('选择目录后展示目录 chip（可清除）', async () => {
    mockDialog()
    const user = userEvent.setup()
    render(<ImportStep workdir={WORKDIR} onReady={() => {}} />, { wrapper })
    await user.click(screen.getByRole('button', { name: '选择目录' }))
    await waitFor(() => expect(screen.getByText('/a')).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: '清除目录选择' }))
    await waitFor(() => expect(screen.queryByText('/a')).not.toBeInTheDocument())
  })

  it('解析为草稿 → 自动打开预览 Dialog（复用 DraftReview）→ 确认导入 → onReady(true)', async () => {
    mockDialog()
    const onReady = vi.fn()
    const user = userEvent.setup()
    render(<ImportStep workdir={WORKDIR} onReady={onReady} />, { wrapper })
    await user.click(screen.getByRole('button', { name: '选择 Markdown 文件' }))
    await waitFor(() => expect(screen.getByText('/a/01.md')).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: '解析为草稿' }))
    // 解析成功自动打开预览 Dialog（DraftReview 三视图 + 确认导入）
    await waitFor(() => expect(screen.getByText('草稿审阅')).toBeInTheDocument())
    expect(screen.getByText('任务甲')).toBeInTheDocument()
    // 确认导入 → onReady(true) + Dialog 关闭 + 步骤显示导入完成
    await user.click(screen.getByRole('button', { name: '确认导入' }))
    await waitFor(() => expect(onReady).toHaveBeenCalledWith(true))
    await waitFor(() => expect(screen.getByText('导入完成')).toBeInTheDocument())
  })
})
