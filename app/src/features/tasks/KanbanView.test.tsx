import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { KanbanView } from './KanbanView'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { toast } from 'sonner'

const TREE = [
  {
    id: 'a',
    project_id: 1,
    parent_id: null,
    title: '前端 token 接入',
    number: 'T01',
    description: '',
    status: 'todo',
    priority: 4,
    tags: ['前端'],
    assignee: 'PB',
    depends_on: [],
    archived_from: '',
    source_file: '',
    source_section: '',
    created_at: '2026-08-06T10:00:00+08:00',
    updated_at: '2026-08-06T10:00:00+08:00',
    children: [],
  },
  {
    id: 'b',
    project_id: 1,
    parent_id: null,
    title: '看板拖拽排序',
    number: 'T02',
    description: '',
    status: 'doing',
    priority: 2,
    tags: ['交互'],
    assignee: '',
    depends_on: [],
    archived_from: '',
    source_file: '',
    source_section: '',
    created_at: '2026-08-06T10:00:00+08:00',
    updated_at: '2026-08-06T10:00:00+08:00',
    children: [],
  },
]

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/project/x/kanban']}>{children}</MemoryRouter>
    </QueryClientProvider>
  )
}

describe('KanbanView', () => {
  const originalGetBoundingClientRect = Element.prototype.getBoundingClientRect

  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tf' })
    // @tanstack/react-virtual 需要 ResizeObserver 即时回调尺寸（jsdom 无布局）。
    // 注意：回调 entries 必须带 target（被观察元素）——measureElement 模式下
    // virtual-core 通过 entry.target 反查 data-index，缺 target 会崩。
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(
          private cb: (
            entries: {
              target: Element
              borderBoxSize?: { inlineSize: number; blockSize: number }[]
            }[],
          ) => void,
        ) {}
        observe(target: Element): void {
          // 模拟真实浏览器：首次 observe 立即回调一次尺寸（带被观察元素）
          this.cb([{ target, borderBoxSize: [{ inlineSize: 300, blockSize: 800 }] }])
        }
        unobserve(): void {}
        disconnect(): void {}
      },
    )
    // 动态测量模式：虚拟项容器（data-index）返回卡片高度，其他元素返回视口尺寸
    Element.prototype.getBoundingClientRect = vi.fn(function (this: Element) {
      if (this.hasAttribute && this.hasAttribute('data-index')) {
        return {
          x: 0,
          y: 0,
          width: 260,
          height: 108,
          top: 0,
          left: 0,
          right: 260,
          bottom: 108,
          toJSON: () => ({}),
        }
      }
      return {
        x: 0,
        y: 0,
        width: 300,
        height: 800,
        top: 0,
        left: 0,
        right: 300,
        bottom: 800,
        toJSON: () => ({}),
      }
    })
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/tasks`, () =>
        HttpResponse.json({ code: 0, data: { tree: TREE, total: 2, page: 0, size: 0 } }),
      ),
      http.get(`${DAEMON_BASE_URL}/api/import/drafts`, () =>
        HttpResponse.json({ code: 0, data: [] }),
      ),
    )
  })

  afterEach(() => {
    Element.prototype.getBoundingClientRect = originalGetBoundingClientRect
    vi.unstubAllGlobals()
  })

  it('按状态机渲染列与任务卡片', async () => {
    render(<KanbanView />, { wrapper })
    await waitFor(() => expect(screen.getByText('待办')).toBeInTheDocument())
    expect(screen.getByText('进行中')).toBeInTheDocument()
    expect(screen.getByText('已完成')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '任务 前端 token 接入' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '任务 看板拖拽排序' })).toBeInTheDocument()
  })

  it('搜索过滤：命中标题的任务保留', async () => {
    const user = userEvent.setup()
    render(<KanbanView />, { wrapper })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '任务 前端 token 接入' })).toBeInTheDocument(),
    )
    await user.type(screen.getByRole('textbox', { name: '搜索任务' }), '拖拽')
    expect(screen.queryByRole('button', { name: '任务 前端 token 接入' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '任务 看板拖拽排序' })).toBeInTheDocument()
  })

  it('标签过滤：点击标签仅显示该标签任务', async () => {
    const user = userEvent.setup()
    render(<KanbanView />, { wrapper })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '任务 前端 token 接入' })).toBeInTheDocument(),
    )
    await user.click(screen.getByRole('button', { name: '#交互' }))
    expect(screen.queryByRole('button', { name: '任务 前端 token 接入' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '任务 看板拖拽排序' })).toBeInTheDocument()
  })

  it('新建任务 Dialog：创建成功 toast', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    const user = userEvent.setup()
    render(<KanbanView />, { wrapper })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /新建任务/ })).toBeInTheDocument(),
    )
    await user.click(screen.getByRole('button', { name: /新建任务/ }))
    expect(await screen.findByRole('heading', { name: '新建任务' })).toBeInTheDocument()
    await user.type(screen.getByLabelText('标题'), 'M5 冒烟')
    await user.click(screen.getByRole('button', { name: '创建' }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
  })
})
