import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { KnowledgePage } from './KnowledgePage'
import { TaskKnowledgeSection } from './TaskKnowledgeSection'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { toast } from 'sonner'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('KnowledgePage', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tangoforge' })
  })

  it('渲染库列表 + 文档列表 + 状态徽标', async () => {
    render(<KnowledgePage />, { wrapper })
    await waitFor(() => expect(screen.getByText('默认库')).toBeInTheDocument())
    expect(screen.getByText('spec.md')).toBeInTheDocument()
    expect(screen.getByText('已嵌入')).toBeInTheDocument()
    expect(screen.getByText('接口规格说明')).toBeInTheDocument()
  })

  it('创建知识库 → toast 反馈', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/knowledge/bases`, () =>
        HttpResponse.json({
          code: 0,
          data: { id: 2, name: 'spec', is_default: false },
        }),
      ),
    )
    const user = userEvent.setup()
    render(<KnowledgePage />, { wrapper })
    await waitFor(() => expect(screen.getByPlaceholderText('新库名')).toBeInTheDocument())
    await user.type(screen.getByPlaceholderText('新库名'), 'spec')
    await user.click(screen.getByRole('button', { name: '' }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
  })

  it('扫描按钮触发 + 结果展示', async () => {
    render(<KnowledgePage />, { wrapper })
    const user = userEvent.setup()
    await waitFor(() => expect(screen.getByRole('button', { name: /扫描/ })).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: /扫描/ }))
    await waitFor(() => expect(toast.success).toBeDefined())
  })

  it('向量检索展示命中片段', async () => {
    render(<KnowledgePage />, { wrapper })
    const user = userEvent.setup()
    const input = await screen.findByPlaceholderText(/向量语义检索/)
    await user.type(input, '接口')
    await waitFor(() => {
      expect(screen.getByText(/接口变更说明/)).toBeInTheDocument()
    })
  })
})

describe('TaskKnowledgeSection', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tangoforge' })
  })

  it('展示任务关联文档列表', async () => {
    render(<TaskKnowledgeSection taskId="task-1" project="/data/projects/tangoforge" />, {
      wrapper,
    })
    await waitFor(() => expect(screen.getByText('(1)')).toBeInTheDocument())
    expect(screen.getAllByText('spec.md').length).toBeGreaterThan(0)
  })

  it('解除关联 → toast + 关闭', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    vi.spyOn(window, 'confirm').mockImplementation(() => true)
    render(<TaskKnowledgeSection taskId="task-1" project="/data/projects/tangoforge" />, {
      wrapper,
    })
    const user = userEvent.setup()
    await waitFor(() => expect(screen.getByLabelText('解除关联 spec.md')).toBeInTheDocument())
    await user.click(screen.getByLabelText('解除关联 spec.md'))
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
    vi.restoreAllMocks()
  })
})
