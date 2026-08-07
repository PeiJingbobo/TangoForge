import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { ProjectPanel } from './project-panel'
import { useProjectStore } from '@/stores/project'

function Dummy({ text }: { text: string }) {
  return <div>{text}</div>
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/project/%2Fdata%2Fprojects%2Fdemo/kanban']}>
        {children}
      </MemoryRouter>
    </QueryClientProvider>
  )
}

function renderPanel() {
  return render(
    <Routes>
      <Route path="project/:projectId" element={<ProjectPanel />}>
        <Route index element={<Dummy text="index" />} />
        <Route path="kanban" element={<Dummy text="看板页内容" />} />
        <Route path="graph" element={<Dummy text="全景图页内容" />} />
      </Route>
    </Routes>,
    { wrapper },
  )
}

describe('ProjectPanel（项目二级 Tab）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: null })
  })

  it('项目路由下渲染全部二级 tab', async () => {
    renderPanel()
    expect(screen.getByRole('link', { name: /看板/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /导航/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /全景图/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /导入导出/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /权限/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Skills/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /审计/ })).toBeInTheDocument()
  })

  it('URL 中的 projectId 同步到全局 store（API 上下文一致）', async () => {
    renderPanel()
    await waitFor(() => {
      expect(useProjectStore.getState().project).toBe('/data/projects/demo')
    })
  })

  it('当前 tab 高亮 + 子路由内容渲染', async () => {
    renderPanel()
    expect(screen.getByText('看板页内容')).toBeInTheDocument()
    const active = screen.getByRole('link', { name: /看板/ })
    expect(active.className).toContain('primary')
  })
})
