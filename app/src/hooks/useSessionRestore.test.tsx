import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router'
import { useSessionRestore } from './useSessionRestore'
import { useProjectStore } from '@/stores/project'

/** 常驻根组件（模拟 App.tsx：useSessionRestore 挂在根组件，不随路由卸载） */
function Shell() {
  useSessionRestore()
  return (
    <Routes>
      <Route path="/" element={<div>harness-root</div>} />
      <Route path="/settings" element={<div>harness-root</div>} />
      <Route path="/project/:projectId/:section" element={<ProjectPageWithNav />} />
    </Routes>
  )
}

/** 项目页 + 返回概览按钮（模拟侧边栏「项目概览」点击） */
function ProjectPageWithNav() {
  const navigate = useNavigate()
  return (
    <div>
      <div>project-restored</div>
      <button onClick={() => navigate('/')}>go-overview</button>
    </div>
  )
}

describe('useSessionRestore（启动会话恢复）', () => {
  beforeEach(() => {
    localStorage.clear()
  })
  afterEach(() => {
    useProjectStore.setState({ project: null, lastSection: 'kanban' })
  })

  it('有上次项目 → 从概览页恢复进入项目（上次二级页）', async () => {
    useProjectStore.setState({ project: '/data/projects/demo', lastSection: 'graph' })
    render(
      <MemoryRouter initialEntries={['/']}>
        <Shell />
      </MemoryRouter>,
    )
    await waitFor(() => expect(screen.getByText('project-restored')).toBeInTheDocument())
    expect(screen.queryByText('harness-root')).not.toBeInTheDocument()
  })

  it('无上次项目 → 停留在项目概览', () => {
    useProjectStore.setState({ project: null, lastSection: 'kanban' })
    render(
      <MemoryRouter initialEntries={['/']}>
        <Shell />
      </MemoryRouter>,
    )
    expect(screen.getByText('harness-root')).toBeInTheDocument()
    expect(screen.queryByText('project-restored')).not.toBeInTheDocument()
  })

  it('非概览路由（如 /settings）不干预', () => {
    useProjectStore.setState({ project: '/data/projects/demo', lastSection: 'kanban' })
    render(
      <MemoryRouter initialEntries={['/settings']}>
        <Shell />
      </MemoryRouter>,
    )
    expect(screen.getByText('harness-root')).toBeInTheDocument()
  })

  it('初始化后手动进入项目概览不再被恢复逻辑干预（可正常停留）', async () => {
    useProjectStore.setState({ project: '/data/projects/demo', lastSection: 'kanban' })
    const user = userEvent.setup()
    render(
      <MemoryRouter initialEntries={['/project/p/kanban']}>
        <Shell />
      </MemoryRouter>,
    )
    // 启动落在项目页：首次 effect 已消费（pathname 非 /，不干预）
    await waitFor(() => expect(screen.getByText('project-restored')).toBeInTheDocument())
    // 点击「项目概览」→ / ：不应再跳回项目页，可正常停留
    await user.click(screen.getByRole('button', { name: 'go-overview' }))
    await waitFor(() => expect(screen.getByText('harness-root')).toBeInTheDocument())
    expect(screen.queryByText('project-restored')).not.toBeInTheDocument()
  })
})
