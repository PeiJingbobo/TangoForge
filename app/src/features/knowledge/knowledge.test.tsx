import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { KnowledgePage } from './KnowledgePage'
import { TaskKnowledgeSection } from './TaskKnowledgeSection'
import { KnowledgeAddDialog } from './KnowledgeAddDialog'
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

describe('KnowledgeAddDialog（TF-053 添加文件）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tangoforge' })
  })

  it('渲染 + 手动添加路径 + 提交注册', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    let postBody: unknown = null
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/knowledge/documents`, async ({ request }) => {
        postBody = await request.json()
        return HttpResponse.json(
          {
            code: 0,
            data: {
              id: 'doc-new',
              display_name: 'manual.md',
              path: 'manual.md',
              abs_path: '/data/projects/tangoforge/manual.md',
              rel_path: 'manual.md',
              type: 'text',
              status: 'ok',
              embedded: 0,
              history: [],
            },
          },
          { status: 201 },
        )
      }),
    )
    const user = userEvent.setup()
    const { rerender } = render(
      <KnowledgeAddDialog open onOpenChange={() => {}} project="/data/projects/tangoforge" />,
      { wrapper },
    )
    // 手动输入路径。
    const input = screen.getByPlaceholderText(/磁盘路径/)
    await user.type(input, '/data/docs/manual.md')
    await user.click(screen.getByRole('button', { name: '添加' }))
    expect(screen.getByText('/data/docs/manual.md')).toBeInTheDocument()
    // 提交。
    await user.click(screen.getByRole('button', { name: '提交添加' }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    const body = postBody as { path?: string; kb_ids?: number[] }
    expect(body.path).toBe('/data/docs/manual.md')
    toastSpy.mockRestore()
    rerender(
      <KnowledgeAddDialog open onOpenChange={() => {}} project="/data/projects/tangoforge" />,
    )
  })

  it('无路径时提交按钮禁用', async () => {
    render(
      <KnowledgeAddDialog open onOpenChange={() => {}} project="/data/projects/tangoforge" />,
      {
        wrapper,
      },
    )
    expect(screen.getByRole('button', { name: '提交添加' })).toBeDisabled()
  })
})
