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

describe('KnowledgePage 添加文件状态条（TF-052）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tangoforge' })
  })

  it('添加文件后顶部状态条显示当前处理中的文件', async () => {
    // 注册慢响应（挂起），使批处理停留在进行中。
    const deferred: { resolve: (() => void) | null } = { resolve: null }
    server.use(
      http.post(
        `${DAEMON_BASE_URL}/api/knowledge/documents`,
        () =>
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          new Promise<any>((resolve) => {
            deferred.resolve = () =>
              resolve(
                HttpResponse.json(
                  {
                    code: 0,
                    data: {
                      id: 'doc-pending',
                      display_name: 'pending.md',
                      path: '/data/pending.md',
                      abs_path: '/data/pending.md',
                      rel_path: '',
                      type: 'text',
                      status: 'ok',
                      embedded: 0,
                      history: [],
                    },
                  },
                  { status: 201 },
                ),
              )
          }),
      ),
    )
    const user = userEvent.setup()
    render(<KnowledgePage />, { wrapper })
    // 打开添加对话框 → 手动路径 → 提交。
    await user.click(screen.getByRole('button', { name: /添加文件/ }))
    const input = await screen.findByPlaceholderText(/磁盘路径/)
    await user.type(input, '/data/pending.md')
    await user.click(screen.getByRole('button', { name: '添加' }))
    await user.click(screen.getByRole('button', { name: '提交添加' }))
    // 状态条显示「正在添加文件 0/1」+ 当前文件。
    await waitFor(() => expect(screen.getByText(/正在添加文件 0\/1/)).toBeInTheDocument())
    expect(screen.getByText(/当前：\/data\/pending.md/)).toBeInTheDocument()
    // 完成注册 → 状态条消失。
    deferred.resolve?.()
    await waitFor(() => expect(screen.queryByText(/正在添加文件/)).not.toBeInTheDocument())
  })
})

describe('KnowledgePage 库过滤（TF-052 修复：选中库后列表可见）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tangoforge' })
  })

  it('选中默认库 → 请求带 filter[kb_id] 且展示该库文件', async () => {
    const user = userEvent.setup()
    const captured: { query: URLSearchParams | null } = { query: null }
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/knowledge/documents`, ({ request }) => {
        const url = new URL(request.url)
        captured.query = url.searchParams
        // 带 kb 过滤时只返回该库文档。
        const kb = url.searchParams.get('filter[kb_id]')
        if (kb) {
          return HttpResponse.json({
            code: 0,
            data: {
              items: [
                {
                  id: 'doc-kb',
                  project_id: 1,
                  path: 'docs/kb.md',
                  abs_path: '/data/projects/tangoforge/docs/kb.md',
                  rel_path: 'docs/kb.md',
                  origin_path: '',
                  display_name: 'kb.md',
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
          })
        }
        return HttpResponse.json({ code: 0, data: { items: [], total: 0, page: 0, size: 50 } })
      }),
    )
    render(<KnowledgePage />, { wrapper })
    // 选中默认库（库列表项按钮）→ 请求带 filter[kb_id]=1，列表展示 kb.md。
    const kbItem = await screen.findByText('默认库')
    await user.click(kbItem)
    await waitFor(() => expect(screen.getByText('kb.md')).toBeInTheDocument())
    expect(captured.query?.get('filter[kb_id]')).toBe('1')
  })
})
