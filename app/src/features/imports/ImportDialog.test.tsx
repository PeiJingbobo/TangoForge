import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { ImportDialog } from './ImportDialog'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { toast } from 'sonner'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

const IMPORT_RESULT = {
  draft_id: 'd1',
  source_file: 'backlog.md',
  task_count: 3,
  status: 'pending',
  created_at: '2026-08-07T08:00:00+08:00',
}

function mockDialog() {
  const selectFiles = vi.fn(async () => ['/data/a.md', '/data/b.md'])
  const selectDirectory = vi.fn(async () => '/data/projects/backlog')
  Object.defineProperty(window, 'tangoforge', {
    value: {
      dialog: { selectFiles, selectDirectory },
    },
    configurable: true,
  })
  return { selectFiles, selectDirectory }
}

describe('ImportDialog（文件/目录选择器）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tf' })
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/import`, () =>
        HttpResponse.json({ code: 0, data: IMPORT_RESULT }),
      ),
    )
  })
  afterEach(() => {
    Object.defineProperty(window, 'tangoforge', { value: undefined, configurable: true })
    vi.restoreAllMocks()
  })

  it('桌面模式：选择文件 → 路径 chips → 提交 file_paths', async () => {
    mockDialog()
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    let body: unknown = null
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/import`, async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({ code: 0, data: IMPORT_RESULT })
      }),
    )
    const user = userEvent.setup()
    render(<ImportDialog onOpenChange={() => {}} />, { wrapper })
    await user.click(screen.getByRole('button', { name: '选择 Markdown 文件' }))
    await waitFor(() => expect(screen.getByText('/data/a.md')).toBeInTheDocument())
    expect(screen.getByText('/data/b.md')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '开始解析' }))
    await waitFor(() => expect(body).not.toBeNull())
    expect(body).toEqual({ file_paths: ['/data/a.md', '/data/b.md'] })
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
  })

  it('桌面模式：选择目录 → 提交 directory', async () => {
    mockDialog()
    let body: unknown = null
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/import`, async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({ code: 0, data: IMPORT_RESULT })
      }),
    )
    const user = userEvent.setup()
    render(<ImportDialog onOpenChange={() => {}} />, { wrapper })
    await user.click(screen.getByRole('button', { name: '选择目录' }))
    await waitFor(() => expect(screen.getByText('/data/projects/backlog')).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: '开始解析' }))
    await waitFor(() => expect(body).not.toBeNull())
    expect(body).toEqual({ directory: '/data/projects/backlog' })
  })

  it('非桌面模式：显示手动输入路径兜底', () => {
    render(<ImportDialog onOpenChange={() => {}} />, { wrapper })
    expect(
      screen.getByPlaceholderText(
        '/Users/you/projects/backlog/01-tasks.md, /Users/you/projects/backlog/02.md',
      ),
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '选择 Markdown 文件' })).not.toBeInTheDocument()
  })
})

describe('ImportDialog 默认路径（TF-053 体验优化：默认打开当前项目根目录）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tf' })
  })
  afterEach(() => {
    Object.defineProperty(window, 'tangoforge', { value: undefined, configurable: true })
    vi.restoreAllMocks()
  })

  it('选择文件/目录时默认路径 = 当前项目根目录', async () => {
    const { selectFiles, selectDirectory } = mockDialog()
    const user = userEvent.setup()
    render(<ImportDialog onOpenChange={() => {}} />, { wrapper })
    await user.click(screen.getByRole('button', { name: '选择 Markdown 文件' }))
    expect(selectFiles).toHaveBeenCalledWith('/data/projects/tf')
    await user.click(screen.getByRole('button', { name: '选择目录' }))
    expect(selectDirectory).toHaveBeenCalledWith('/data/projects/tf')
  })
})
