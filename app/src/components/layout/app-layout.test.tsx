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

  it('左侧导航：项目概览/项目列表/底部一行（图标切换+设置+指示点）', async () => {
    renderLayout()
    await waitFor(() => expect(screen.getByText('项目概览')).toBeInTheDocument())
    // 项目列表按钮（MSW demo 数据）
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'TangoForge' })).toBeInTheDocument(),
    )
    // 底部：亮暗切换（仅图标，aria-label）、设置（仅图标）、指示点（无文字，role=status）
    expect(screen.getByRole('button', { name: '切换亮暗色' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '设置' })).toBeInTheDocument()
    const dot = screen.getByRole('status')
    expect(dot).toBeInTheDocument()
    // 指示点不应包含提示文字（纯圆点）
    expect(dot.textContent).toBe('')
  })

  it('一级页面（/）不展示二级 tab（看板等链接不存在）', async () => {
    renderLayout()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'TangoForge' })).toBeInTheDocument(),
    )
    expect(screen.queryByRole('link', { name: /看板/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /全景图/ })).not.toBeInTheDocument()
  })

  it('点击侧边栏项目 → setProject + 导航到该项目看板', async () => {
    renderLayout()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'TangoForge' })).toBeInTheDocument(),
    )
    await userEvent.click(screen.getByRole('button', { name: 'TangoForge' }))
    await waitFor(() => {
      expect(useProjectStore.getState().project).toBe('/data/projects/tangoforge')
    })
    expect(await screen.findByText('看板内容')).toBeInTheDocument()
  })

  it('亮暗切换按钮可点击（仅图标，useThemeMode 持久化）', async () => {
    renderLayout()
    const btn = screen.getByRole('button', { name: '切换亮暗色' })
    await userEvent.click(btn)
    expect(localStorage.getItem('tf-theme-mode')).not.toBeNull()
  })
})
