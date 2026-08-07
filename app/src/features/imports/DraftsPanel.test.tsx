import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { DraftsPanel } from './DraftsPanel'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { toast } from 'sonner'

const DRAFT = {
  id: 'd1',
  source_file: 'backlog.md',
  status: 'pending',
  task_count: 12,
  created_at: '2026-08-06T10:00:00+08:00',
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('DraftsPanel（草稿确认流）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tf' })
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/import/drafts`, () =>
        HttpResponse.json({ code: 0, data: [DRAFT] }),
      ),
    )
  })

  it('渲染 pending 草稿（source_file + 数量）', async () => {
    render(<DraftsPanel />, { wrapper })
    await waitFor(() => expect(screen.getByText('backlog.md')).toBeInTheDocument())
    expect(screen.getByText(/12 个任务/)).toBeInTheDocument()
  })

  it('无草稿时不渲染面板', async () => {
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/import/drafts`, () =>
        HttpResponse.json({ code: 0, data: [] }),
      ),
    )
    const { container } = render(<DraftsPanel />, { wrapper })
    await waitFor(() => expect(container).toBeEmptyDOMElement())
  })

  it('确认导入 → toast 反馈 created/archived', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/import/drafts/d1/confirm`, () =>
        HttpResponse.json({
          code: 0,
          data: { draft_id: 'd1', source_file: 'backlog.md', created: 10, archived: 2 },
        }),
      ),
    )
    const user = userEvent.setup()
    render(<DraftsPanel />, { wrapper })
    await waitFor(() => expect(screen.getByText('backlog.md')).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: /确认导入/ }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    const desc = toastSpy.mock.calls[0]?.[1] as { description: string }
    expect(desc.description).toContain('创建 10 个')
    expect(desc.description).toContain('归档 2 个')
    toastSpy.mockRestore()
  })

  it('丢弃草稿 → toast', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    server.use(
      http.delete(`${DAEMON_BASE_URL}/api/import/drafts/d1`, () =>
        HttpResponse.json({ code: 0, data: { ok: true } }),
      ),
    )
    const user = userEvent.setup()
    render(<DraftsPanel />, { wrapper })
    await waitFor(() => expect(screen.getByText('backlog.md')).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: /丢弃草稿/ }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
  })

  it('提供 onReview 时渲染审阅按钮并回调', async () => {
    const onReview = vi.fn()
    const user = userEvent.setup()
    render(<DraftsPanel onReview={onReview} />, { wrapper })
    await waitFor(() => expect(screen.getByText('backlog.md')).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: /审阅草稿/ }))
    expect(onReview).toHaveBeenCalledWith('d1')
  })
})
