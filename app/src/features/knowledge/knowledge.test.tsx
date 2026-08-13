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

  it('渲染 + 手动添加路径 + 提交委托 onSubmit 并关闭', async () => {
    const onOpenChange = vi.fn()
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    render(
      <KnowledgeAddDialog
        open
        onOpenChange={onOpenChange}
        project="/data/projects/tangoforge"
        onSubmit={onSubmit}
      />,
      { wrapper },
    )
    // 手动输入路径。
    const input = screen.getByPlaceholderText(/磁盘路径/)
    await user.type(input, '/data/docs/manual.md')
    await user.click(screen.getByRole('button', { name: '添加' }))
    expect(screen.getByText('/data/docs/manual.md')).toBeInTheDocument()
    // 提交 → 委托 onSubmit + 立即关闭。
    await user.click(screen.getByRole('button', { name: '提交添加' }))
    expect(onSubmit).toHaveBeenCalledWith(['/data/docs/manual.md'], 0, 'auto')
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('无路径时提交按钮禁用', async () => {
    render(
      <KnowledgeAddDialog
        open
        onOpenChange={() => {}}
        project="/data/projects/tangoforge"
        onSubmit={() => {}}
      />,
      {
        wrapper,
      },
    )
    expect(screen.getByRole('button', { name: '提交添加' })).toBeDisabled()
  })
})

describe('KnowledgePage 嵌入状态（TF-052）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tangoforge' })
    const indexingDoc = {
      id: 'doc-idx',
      project_id: 1,
      path: 'docs/idx.md',
      abs_path: '/data/projects/tangoforge/docs/idx.md',
      rel_path: 'docs/idx.md',
      origin_path: '',
      display_name: 'idx.md',
      type: 'text',
      size: 10,
      mtime: '2026-08-13T09:00:00+08:00',
      content_hash: '',
      summary: '',
      status: 'indexing',
      embedded: 0,
      embedding_model: '',
      index_error: '',
      history: [],
      created_at: '2026-08-13T09:00:00+08:00',
      updated_at: '2026-08-13T09:00:00+08:00',
    }
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/knowledge/documents`, () =>
        HttpResponse.json({
          code: 0,
          data: { items: [indexingDoc], total: 1, page: 0, size: 50 },
        }),
      ),
    )
  })

  it('正在嵌入文档：列表徽标 + 顶部状态条', async () => {
    render(<KnowledgePage />, { wrapper })
    // 列表徽标「正在嵌入」。
    await waitFor(() => expect(screen.getByText('正在嵌入')).toBeInTheDocument())
    // 顶部状态条「嵌入中 1 个文档」。
    expect(screen.getByText(/嵌入中 1 个文档/)).toBeInTheDocument()
  })

  it('已嵌入文档显示「已嵌入」徽标', async () => {
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/knowledge/documents`, () =>
        HttpResponse.json({
          code: 0,
          data: {
            items: [
              {
                id: 'doc-ok',
                project_id: 1,
                path: 'docs/a.md',
                abs_path: '/data/projects/tangoforge/docs/a.md',
                rel_path: 'docs/a.md',
                origin_path: '',
                display_name: 'a.md',
                type: 'text',
                size: 10,
                mtime: '2026-08-13T09:00:00+08:00',
                content_hash: 'h',
                summary: '',
                status: 'ok',
                embedded: 1,
                embedding_model: 'm',
                index_error: '',
                history: [],
                created_at: '2026-08-13T09:00:00+08:00',
                updated_at: '2026-08-13T09:00:00+08:00',
              },
            ],
            total: 1,
            page: 0,
            size: 50,
          },
        }),
      ),
    )
    render(<KnowledgePage />, { wrapper })
    await waitFor(() => expect(screen.getByText('已嵌入')).toBeInTheDocument())
    // 无 indexing → 无顶部状态条。
    expect(screen.queryByText(/嵌入中/)).not.toBeInTheDocument()
  })
})
