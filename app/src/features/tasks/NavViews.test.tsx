import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { NavPage, TimelineView, StatusView } from './NavViews'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import type { StateMachineState } from '@/types/models'
import type { Task, TaskTreeNode } from '@/types/task'

const STATES: StateMachineState[] = [
  { Key: 'todo', Label: '待办', Color: '#9aa0a6' },
  { Key: 'doing', Label: '进行中', Color: '#1a73e8' },
]

function mk(id: string, title: string, status: string, over: Partial<Task> = {}): Task {
  return {
    id,
    project_id: 1,
    parent_id: null,
    title,
    number: '',
    description: '',
    status,
    priority: 0,
    tags: [],
    assignee: '',
    depends_on: [],
    archived_from: '',
    source_file: '',
    source_section: '',
    created_at: '2026-08-09T10:00:00+08:00',
    updated_at: '2026-08-09T10:00:00+08:00',
    ...over,
  }
}

describe('NavViews 任务状态展示（QA 2026-08-09）', () => {
  it('TimelineView：任务行显示状态机 Label 徽标（含彩色圆点）', () => {
    render(<TimelineView tasks={[mk('a', '任务甲', 'doing')]} states={STATES} onOpen={() => {}} />)
    expect(screen.getByText('任务甲')).toBeInTheDocument()
    expect(screen.getByText('进行中')).toBeInTheDocument()
    const dot = document.querySelector('span[style*="background-color: rgb(26, 115, 232)"]')
    expect(dot).not.toBeNull()
  })

  it('StatusView：分组头使用状态机颜色圆点 + 行内状态徽标', () => {
    render(
      <StatusView
        tasks={[mk('a', '任务甲', 'todo'), mk('b', '任务乙', 'doing')]}
        states={STATES}
        onOpen={() => {}}
      />,
    )
    // 「待办/进行中」出现在分组头与任务行状态徽标
    expect(screen.getAllByText('待办').length).toBeGreaterThan(0)
    expect(screen.getAllByText('进行中').length).toBeGreaterThan(0)
  })
})

describe('NavPage（标签多选筛选，Tab 右侧）', () => {
  const TREE: TaskTreeNode[] = [
    { ...mk('a', '前端 token 接入', 'todo', { tags: ['前端'], priority: 4 }), children: [] },
    { ...mk('b', '看板拖拽排序', 'doing', { tags: ['交互'] }), children: [] },
    { ...mk('c', '后端 API', 'doing', { tags: ['后端'] }), children: [] },
  ]

  function wrapper({ children }: { children: ReactNode }) {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    return (
      <QueryClientProvider client={qc}>
        <MemoryRouter initialEntries={['/project/x/nav']}>{children}</MemoryRouter>
      </QueryClientProvider>
    )
  }

  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tf' })
    vi.stubGlobal(
      'WebSocket',
      class {
        close(): void {}
      },
    )
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/tasks`, () =>
        HttpResponse.json({ code: 0, data: { tree: TREE, total: 3, page: 0, size: 0 } }),
      ),
      http.get(`${DAEMON_BASE_URL}/api/state-machine`, () =>
        HttpResponse.json({ code: 0, data: { States: STATES, Transitions: [] } }),
      ),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('标签筛选区位于 Tab 右侧，多选过滤三视图共用', async () => {
    const user = userEvent.setup()
    render(<NavPage />, { wrapper })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '任务 前端 token 接入' })).toBeInTheDocument(),
    )
    // Tab 与标签筛选同排（标签按钮存在）
    expect(screen.getByRole('tab', { name: '树形' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '#前端' })).toBeInTheDocument()

    // 多选 #前端 → 仅保留前端标签任务（树形）
    await user.click(screen.getByRole('button', { name: '#前端' }))
    expect(screen.queryByRole('button', { name: '任务 看板拖拽排序' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '任务 前端 token 接入' })).toBeInTheDocument()

    // 追加 #交互 → 两个标签任务均显示（多选取并集）
    await user.click(screen.getByRole('button', { name: '#交互' }))
    expect(screen.getByRole('button', { name: '任务 看板拖拽排序' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '任务 前端 token 接入' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '任务 后端 API' })).not.toBeInTheDocument()
  })

  it('标签筛选切到时间线视图同样生效', async () => {
    const user = userEvent.setup()
    render(<NavPage />, { wrapper })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '任务 前端 token 接入' })).toBeInTheDocument(),
    )
    await user.click(screen.getByRole('button', { name: '#交互' }))
    await user.click(screen.getByRole('tab', { name: '时间线' }))
    expect(screen.getByText('看板拖拽排序')).toBeInTheDocument()
    expect(screen.queryByText('前端 token 接入')).not.toBeInTheDocument()
  })
})
