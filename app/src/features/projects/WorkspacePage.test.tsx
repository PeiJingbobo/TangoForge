import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { WorkspacePage } from './WorkspacePage'
import { useProjectStore } from '@/stores/project'
import { setUiToken } from '@/api/client'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/']}>{children}</MemoryRouter>
    </QueryClientProvider>
  )
}

describe('WorkspacePage', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: null })
    setUiToken(null)
    // 非桌面环境：不触发 window.tangoforge
    Object.defineProperty(window, 'tangoforge', { value: undefined, configurable: true })
  })

  it('加载并展示项目列表（行式，非卡片）', async () => {
    render(<WorkspacePage />, { wrapper })
    await waitFor(() => expect(screen.getByText('工作区概览')).toBeInTheDocument())
    expect(screen.getByRole('button', { name: 'TangoForge' })).toBeInTheDocument()
  })

  it('无项目时展示空态引导', async () => {
    const { http, HttpResponse } = await import('msw')
    const { server } = await import('@/test/server')
    const { DAEMON_BASE_URL } = await import('@/api/client')
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/projects`, () => HttpResponse.json({ code: 0, data: [] })),
    )
    render(<WorkspacePage />, { wrapper })
    await waitFor(() => expect(screen.getByText('从工作目录开始')).toBeInTheDocument())
    expect(screen.getByText('选择目录导入')).toBeInTheDocument()
  })

  it('点击项目进入看板（setProject + navigate）', async () => {
    const user = userEvent.setup()
    render(<WorkspacePage />, { wrapper })
    await waitFor(() => expect(screen.getByText('工作区概览')).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: 'TangoForge' }))
    expect(useProjectStore.getState().project).toBe('/data/projects/tangoforge')
  })

  it('手动输入路径导入 → setProject + 跳转', async () => {
    const user = userEvent.setup()
    render(<WorkspacePage />, { wrapper })
    const input = screen.getByPlaceholderText('/Users/you/projects/backlog')
    await user.type(input, '/data/projects/demo')
    await user.click(screen.getByRole('button', { name: '导入该路径' }))
    await waitFor(() => expect(useProjectStore.getState().project).toBe('/data/projects/demo'))
  })
})
