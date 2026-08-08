import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { WorkspacePage } from './WorkspacePage'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/']}>{children}</MemoryRouter>
    </QueryClientProvider>
  )
}

describe('WorkspacePage（项目概览）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: null })
    // 非桌面环境：不触发 window.tangoforge
    Object.defineProperty(window, 'tangoforge', { value: undefined, configurable: true })
  })

  it('渲染概览标题与最近项目（导入引导入口）', async () => {
    render(<WorkspacePage />, { wrapper })
    await waitFor(() => expect(screen.getByText('项目概览')).toBeInTheDocument())
    await waitFor(() => expect(screen.getByText('TangoForge')).toBeInTheDocument())
    expect(screen.getByText('导入工作目录')).toBeInTheDocument()
  })

  it('无项目时展示导入引导（无最近项目区块）', async () => {
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/projects`, () => HttpResponse.json({ code: 0, data: [] })),
    )
    render(<WorkspacePage />, { wrapper })
    await waitFor(() => expect(screen.getByText('导入工作目录')).toBeInTheDocument())
    expect(screen.queryByText('最近项目')).not.toBeInTheDocument()
  })

  it('点击最近项目进入看板（setProject + navigate）', async () => {
    const user = userEvent.setup()
    render(<WorkspacePage />, { wrapper })
    await waitFor(() => expect(screen.getByText('TangoForge')).toBeInTheDocument())
    await user.click(screen.getByText('TangoForge'))
    expect(useProjectStore.getState().project).toBe('/data/projects/tangoforge')
  })

  it('手动输入路径导入（未完成引导）→ 打开引导流程对话框', async () => {
    const user = userEvent.setup()
    render(<WorkspacePage />, { wrapper })
    const input = screen.getByPlaceholderText('/Users/you/projects/backlog')
    await user.type(input, '/data/projects/demo')
    await user.click(screen.getByRole('button', { name: '导入该路径' }))
    // 新目录未完成引导 → 弹引导对话框（Step 1/6 + 工作目录面板）。
    await waitFor(() => expect(screen.getByText(/Step 1\/6/)).toBeInTheDocument())
    expect(screen.getByText('工作目录')).toBeInTheDocument()
    expect(screen.getByText('上一步')).toBeInTheDocument()
  })

  it('已注册且引导完成（onboarded）→ 直接进入项目，不弹引导（TF-043）', async () => {
    const user = userEvent.setup()
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/projects/check`, () =>
        HttpResponse.json({
          code: 0,
          data: { registered: true, onboarded: true, has_meta: true, meta_valid: true },
        }),
      ),
      http.get(`${DAEMON_BASE_URL}/api/projects`, () =>
        HttpResponse.json({
          code: 0,
          data: [
            {
              id: 1,
              name: 'demo',
              workdir: '/data/projects/demo',
              created_at: '2026-08-08T10:00:00+08:00',
              last_opened_at: null,
              hidden: false,
            },
          ],
        }),
      ),
    )
    render(<WorkspacePage />, { wrapper })
    const input = screen.getByPlaceholderText('/Users/you/projects/backlog')
    await user.type(input, '/data/projects/demo')
    await user.click(screen.getByRole('button', { name: '导入该路径' }))
    await waitFor(() => expect(useProjectStore.getState().project).toBe('/data/projects/demo'))
    expect(screen.queryByText(/Step 1\/6/)).not.toBeInTheDocument()
  })
})
