import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { useSessionRestore } from './useSessionRestore'
import { useProjectStore } from '@/stores/project'

function Harness() {
  useSessionRestore()
  return <div>harness-root</div>
}

function Dummy() {
  return <div>project-restored</div>
}

function renderAtRoot() {
  return render(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route path="/" element={<Harness />} />
        <Route path="/project/:projectId/:section" element={<Dummy />} />
      </Routes>
    </MemoryRouter>,
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
    renderAtRoot()
    await waitFor(() => expect(screen.getByText('project-restored')).toBeInTheDocument())
    expect(screen.queryByText('harness-root')).not.toBeInTheDocument()
  })

  it('无上次项目 → 停留在项目概览', () => {
    useProjectStore.setState({ project: null, lastSection: 'kanban' })
    renderAtRoot()
    expect(screen.getByText('harness-root')).toBeInTheDocument()
    expect(screen.queryByText('project-restored')).not.toBeInTheDocument()
  })

  it('非概览路由（如 /settings）不干预', () => {
    useProjectStore.setState({ project: '/data/projects/demo', lastSection: 'kanban' })
    render(
      <MemoryRouter initialEntries={['/settings']}>
        <Routes>
          <Route path="/settings" element={<Harness />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('harness-root')).toBeInTheDocument()
  })
})
