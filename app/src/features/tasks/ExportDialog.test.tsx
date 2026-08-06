import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { ExportDialog } from './ExportDialog'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { toast } from 'sonner'

const EXPORT_RESULT = {
  content: '# 导出内容\n\n- [ ] 任务一',
  path: '/data/projects/tf/.taskboard/export.md',
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('ExportDialog', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tf' })
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/export`, () =>
        HttpResponse.json({ code: 0, data: EXPORT_RESULT }),
      ),
    )
  })

  it('默认 copy 模式：导出并写盘 → 预览 + toast', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    const user = userEvent.setup()
    render(<ExportDialog open onOpenChange={() => {}} />, { wrapper })
    await user.click(screen.getByRole('button', { name: /导出并写盘/ }))
    await waitFor(() => expect(screen.getByText('预览')).toBeInTheDocument())
    expect(screen.getByText(/任务一/)).toBeInTheDocument()
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
  })

  it('overwrite 模式需填路径：未填 → info 提示不请求', async () => {
    const toastSpy = vi.spyOn(toast, 'info').mockImplementation(() => '')
    const user = userEvent.setup()
    render(<ExportDialog open onOpenChange={() => {}} />, { wrapper })
    await user.click(screen.getByRole('button', { name: /覆盖指定文件/ }))
    await user.click(screen.getByRole('button', { name: /导出并写盘/ }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    expect(screen.queryByText('预览')).not.toBeInTheDocument()
    toastSpy.mockRestore()
  })

  it('LLM 生成模板：提交示例 → toast 成功并切到 LLM 模式', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/export/template/generate`, () =>
        HttpResponse.json({
          code: 0,
          data: { template: '{{template}}', path: '/x/.taskboard/generated-template.tmpl' },
        }),
      ),
    )
    const user = userEvent.setup()
    render(<ExportDialog open onOpenChange={() => {}} />, { wrapper })
    await user.click(screen.getByRole('button', { name: /用 LLM 生成模板/ }))
    await user.type(screen.getByLabelText(/示例文档/), '# 示例\n\n## 任务')
    await user.click(screen.getByRole('button', { name: /生成模板/ }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
  })
})
