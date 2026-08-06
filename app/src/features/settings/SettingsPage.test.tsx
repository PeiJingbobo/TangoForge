import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { SettingsPage } from './SettingsPage'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { toast } from 'sonner'

const CONFIG = {
  port: 19810,
  remote_access: false,
  api_token: '',
  llm: {
    base_url: 'https://api.deepseek.com',
    api_key: 'sk-a2c****c1',
    model: 'deepseek-chat',
    api_kind: 'openai',
    timeout_sec: 120,
    retries: 1,
    max_tokens: 16384,
    concurrency: 1,
  },
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('SettingsPage（全局首选项：实时保存 + 校验回滚）', () => {
  beforeEach(() => {
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/config`, () => HttpResponse.json({ code: 0, data: CONFIG })),
    )
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('LLM 分组：修改模型 → debounce 后 PUT 保存 + toast', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    let putBody: unknown = null
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/config`, async ({ request }) => {
        putBody = await request.json()
        return HttpResponse.json({ code: 0, data: CONFIG })
      }),
    )
    const user = userEvent.setup()
    render(<SettingsPage />, { wrapper })
    const modelInput = await screen.findByLabelText('模型名')
    expect(modelInput).toHaveValue('deepseek-chat')
    await user.clear(modelInput)
    await user.type(modelInput, 'deepseek-v4-flash')
    await waitFor(() => expect(putBody).not.toBeNull(), { timeout: 2000 })
    expect(putBody).toEqual(
      expect.objectContaining({ llm: expect.objectContaining({ model: 'deepseek-v4-flash' }) }),
    )
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
  })

  it('校验失败：PUT 返回 422 CONFIG_INVALID → toast 提示 + 输入回滚', async () => {
    const toastError = vi.spyOn(toast, 'error').mockImplementation(() => '')
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/config`, () =>
        HttpResponse.json(
          { code: 'CONFIG_INVALID', message: 'LLM 模型名不能为空' },
          { status: 422 },
        ),
      ),
    )
    const user = userEvent.setup()
    render(<SettingsPage />, { wrapper })
    const modelInput = await screen.findByLabelText('模型名')
    await user.clear(modelInput)
    await user.type(modelInput, '  ')
    await waitFor(() => expect(toastError).toBeCalled(), { timeout: 2000 })
    expect(String(toastError.mock.calls[0]?.[0])).toContain('模型名不能为空')
    // 回滚：输入框恢复服务端值
    await waitFor(() => expect(modelInput).toHaveValue('deepseek-chat'))
    toastError.mockRestore()
  })

  it('外观分组：主题模式切换实时生效（localStorage 持久化）', async () => {
    const user = userEvent.setup()
    render(<SettingsPage />, { wrapper })
    await user.click(screen.getByRole('tab', { name: '外观' }))
    await screen.findByRole('button', { name: /深色/ })
    await user.click(screen.getByRole('button', { name: /深色/ }))
    expect(localStorage.getItem('tf-theme-mode')).toBe('dark')
    await user.click(screen.getByRole('button', { name: /浅色/ }))
    expect(localStorage.getItem('tf-theme-mode')).toBe('light')
  })
})
