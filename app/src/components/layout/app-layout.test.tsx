import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { AppLayout } from './app-layout'
import { useProjectStore } from '@/stores/project'

function Dummy({ text }: { text: string }) {
  return <div>{text}</div>
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/']}>{children}</MemoryRouter>
    </QueryClientProvider>
  )
}

function renderLayout() {
  return render(
    <Routes>
      <Route element={<AppLayout />}>
        <Route index element={<Dummy text="概览内容" />} />
        <Route path="project/:projectId/kanban" element={<Dummy text="看板内容" />} />
      </Route>
    </Routes>,
    { wrapper },
  )
}

describe('AppLayout（全局导航布局）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: null })
    Object.defineProperty(window, 'tangoforge', { value: undefined, configurable: true })
  })

  it('左侧导航：项目概览/项目列表/亮暗切换/设置/守护进程状态', async () => {
    renderLayout()
    await waitFor(() => expect(screen.getByText('项目概览')).toBeInTheDocument())
    expect(screen.getByText('项目')).toBeInTheDocument()
    // 项目列表按钮（MSW demo 数据）
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'TangoForge' })).toBeInTheDocument(),
    )
    expect(screen.getByRole('button', { name: '切换亮暗色' })).toBeInTheDocument()
    expect(screen.getByText('设置')).toBeInTheDocument()
    // 守护进程状态点（web 环境 false → 未连接）
    expect(screen.getByText('守护进程未连接')).toBeInTheDocument()
  })

  it('未激活项目时不显示二级 tabs；点击项目后激活并显示', async () => {
    renderLayout()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'TangoForge' })).toBeInTheDocument(),
    )
    // 未激活：无 tabs
    expect(screen.queryByRole('link', { name: /看板/ })).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'TangoForge' }))
    // 激活：setProject + tabs 出现
    await waitFor(() => {
      expect(useProjectStore.getState().project).toBe('/data/projects/tangoforge')
      expect(screen.getByRole('link', { name: /看板/ })).toBeInTheDocument()
    })
    expect(screen.getByRole('link', { name: /全景图/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /导入导出/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /权限/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Skills/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /审计/ })).toBeInTheDocument()
    expect(useProjectStore.getState().project).toBe('/data/projects/tangoforge')
  })

  it('亮暗切换按钮可点击（useThemeMode 持久化）', async () => {
    renderLayout()
    const btn = screen.getByRole('button', { name: '切换亮暗色' })
    await userEvent.click(btn)
    // 点击后仍存在（切换 light/dark）；localStorage 有值
    expect(localStorage.getItem('tf-theme-mode')).not.toBeNull()
  })
})
