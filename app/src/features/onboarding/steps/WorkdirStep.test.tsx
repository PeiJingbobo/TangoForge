import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { WorkdirStep } from './WorkdirStep'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('WorkdirStep（TF-041 Step 0）', () => {
  beforeEach(() => {
    localStorage.clear()
    useProjectStore.setState({ project: null })
    Object.defineProperty(window, 'tangoforge', { value: undefined, configurable: true })
  })
  afterEach(() => localStorage.clear())

  it('已注册目录 → 直接放行（onReady(true)）', async () => {
    const onReady = vi.fn()
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/projects/check`, () =>
        HttpResponse.json({
          code: 0,
          data: { registered: true, has_meta: true, meta_valid: true },
        }),
      ),
    )
    render(<WorkdirStep workdir="/data/projects/tf" onReady={onReady} />, { wrapper })
    await waitFor(() => expect(onReady).toHaveBeenCalledWith(true))
    expect(screen.getByText(/目录已就绪/)).toBeInTheDocument()
    // 检查中提示应消失（不卡住）。
    await waitFor(() => expect(screen.queryByText(/正在检查并初始化目录/)).not.toBeInTheDocument())
  })

  it('空目录（未注册无元数据）→ 自动初始化注册并放行', async () => {
    const onReady = vi.fn()
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/projects/check`, () =>
        HttpResponse.json({
          code: 0,
          data: { registered: false, has_meta: false, meta_valid: true },
        }),
      ),
      http.post(`${DAEMON_BASE_URL}/api/projects/import`, () =>
        HttpResponse.json({
          code: 0,
          data: {
            id: 9,
            name: 'x',
            workdir: '/data/projects/tf',
            created_at: '',
            last_opened_at: null,
          },
        }),
      ),
    )
    render(<WorkdirStep workdir="/data/projects/tf" onReady={onReady} />, { wrapper })
    await waitFor(() => expect(onReady).toHaveBeenCalledWith(true), { timeout: 3000 })
    await waitFor(() => expect(screen.queryByText(/正在检查并初始化目录/)).not.toBeInTheDocument())
  })

  it('元数据非法 → 询问清空（不卡在检查中）', async () => {
    const onReady = vi.fn()
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/projects/check`, () =>
        HttpResponse.json({
          code: 0,
          data: {
            registered: false,
            has_meta: true,
            meta_valid: false,
            meta_reason: 'config.yaml 缺失',
          },
        }),
      ),
    )
    render(<WorkdirStep workdir="/data/projects/tf" onReady={onReady} />, { wrapper })
    await waitFor(() => expect(screen.getByText(/清空并重新初始化/)).toBeInTheDocument())
    expect(onReady).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.queryByText(/正在检查并初始化目录/)).not.toBeInTheDocument())
  })

  it('check 请求失败 → 显示错误 + 重试按钮（不静默卡住）', async () => {
    const onReady = vi.fn()
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/projects/check`, () =>
        HttpResponse.json({ code: 'INTERNAL_ERROR', message: 'boom' }, { status: 500 }),
      ),
    )
    render(<WorkdirStep workdir="/data/projects/tf" onReady={onReady} />, { wrapper })
    await waitFor(() => expect(screen.getByText('目录检查失败')).toBeInTheDocument())
    expect(screen.getByRole('button', { name: /重试/ })).toBeInTheDocument()
    expect(onReady).not.toHaveBeenCalled()
  })

  it('失败后点重试 → 重新检查并成功放行', async () => {
    const onReady = vi.fn()
    let fail = true
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/projects/check`, () => {
        if (fail) {
          fail = false
          return HttpResponse.json({ code: 'INTERNAL_ERROR', message: 'boom' }, { status: 500 })
        }
        return HttpResponse.json({
          code: 0,
          data: { registered: false, has_meta: false, meta_valid: true },
        })
      }),
      http.post(`${DAEMON_BASE_URL}/api/projects/import`, () =>
        HttpResponse.json({
          code: 0,
          data: {
            id: 9,
            name: 'x',
            workdir: '/data/projects/tf',
            created_at: '',
            last_opened_at: null,
          },
        }),
      ),
    )
    const user = userEvent.setup()
    render(<WorkdirStep workdir="/data/projects/tf" onReady={onReady} />, { wrapper })
    await waitFor(() => expect(screen.getByText('目录检查失败')).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: /重试/ }))
    await waitFor(() => expect(onReady).toHaveBeenCalledWith(true), { timeout: 3000 })
  })
})
