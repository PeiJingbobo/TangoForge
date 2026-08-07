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

const DEFAULT_TMPL = '---\ntitle: "{{.Project.Name}}"\ngenerated_at: "{{.GeneratedAt}}"\n---'

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
      // TF-038 模板内容：default 返回内置；llm 默认未生成（TEMPLATE_INVALID）。
      http.get(`${DAEMON_BASE_URL}/api/export/template`, ({ request }) => {
        const mode = new URL(request.url).searchParams.get('mode')
        if (mode === 'llm') {
          return HttpResponse.json(
            { code: 'TEMPLATE_INVALID', message: '尚未生成 LLM 模板' },
            { status: 422 },
          )
        }
        return HttpResponse.json({ code: 0, data: { template: DEFAULT_TMPL, mode: 'default' } })
      }),
    )
  })

  it('默认 copy 模式：导出并写盘 → 预览 + toast', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    const user = userEvent.setup()
    render(<ExportDialog open onOpenChange={() => {}} />, { wrapper })
    // 默认模板内容已展示（异步加载）。
    await waitFor(() => expect(screen.getByText(/generated_at/)).toBeInTheDocument())
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
    await waitFor(() => expect(screen.getByText(/模板内容/)).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: /覆盖指定文件/ }))
    await user.click(screen.getByRole('button', { name: /导出并写盘/ }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    expect(screen.queryByText('预览')).not.toBeInTheDocument()
    toastSpy.mockRestore()
  })

  it('TF-038 切换 LLM 模板（未生成）→ 自动展开「用 LLM 生成模板」表单', async () => {
    const user = userEvent.setup()
    render(<ExportDialog open onOpenChange={() => {}} />, { wrapper })
    await waitFor(() => expect(screen.getByText(/模板内容/)).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: /LLM 模板/ }))
    // 未生成 → 自动出现示例文档表单（无需手动点「用 LLM 生成模板」）。
    await waitFor(() => expect(screen.getByLabelText(/示例文档/)).toBeInTheDocument())
  })

  it('LLM 生成模板：提交示例 → toast 成功并切到 LLM 模式 + 模板内容刷新', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    const generated = {
      template: 'LLM-TPL: {{header .Level .Title}}',
      path: '/data/projects/tf/.taskboard/generated-template.tmpl',
    }
    // 生成后 llm 模板查询返回生成内容。
    let llmGenerated = false
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/export/template/generate`, () => {
        llmGenerated = true
        return HttpResponse.json({ code: 0, data: generated })
      }),
      http.get(`${DAEMON_BASE_URL}/api/export/template`, ({ request }) => {
        const mode = new URL(request.url).searchParams.get('mode')
        if (mode === 'llm') {
          return llmGenerated
            ? HttpResponse.json({ code: 0, data: { template: generated.template, mode: 'llm' } })
            : HttpResponse.json(
                { code: 'TEMPLATE_INVALID', message: '尚未生成 LLM 模板' },
                { status: 422 },
              )
        }
        return HttpResponse.json({ code: 0, data: { template: DEFAULT_TMPL, mode: 'default' } })
      }),
    )
    const user = userEvent.setup()
    render(<ExportDialog open onOpenChange={() => {}} />, { wrapper })
    await waitFor(() => expect(screen.getByText(/模板内容/)).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: /用 LLM 生成模板/ }))
    await user.type(screen.getByLabelText(/示例文档/), '# 示例\n\n## 任务')
    await user.click(screen.getByRole('button', { name: /生成模板/ }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    // 生成后展示 LLM 模板内容。
    await waitFor(() => expect(screen.getByText(/LLM-TPL/)).toBeInTheDocument())
    toastSpy.mockRestore()
  })
})
