import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { AppLayout } from './app-layout'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { toast } from 'sonner'

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

  it('TF-035 右键菜单：重命名项目（PATCH）', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    let patchBody: unknown = null
    server.use(
      http.patch(`${DAEMON_BASE_URL}/api/projects/1`, async ({ request }) => {
        patchBody = await request.json()
        return HttpResponse.json({
          code: 0,
          data: {
            id: 1,
            name: '重命名后',
            workdir: '/data/projects/tangoforge',
            created_at: '2026-08-06T10:00:00+08:00',
            last_opened_at: null,
          },
        })
      }),
    )
    const user = userEvent.setup()
    renderLayout()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'TangoForge' })).toBeInTheDocument(),
    )
    // 右键项目项 → 菜单 → 重命名。
    await user.pointer({
      keys: '[MouseRight]',
      target: screen.getByRole('button', { name: 'TangoForge' }),
    })
    await user.click(await screen.findByRole('menuitem', { name: /重命名/ }))
    const input = await screen.findByLabelText('项目名称')
    await user.clear(input)
    await user.type(input, '重命名后')
    await user.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(patchBody).toEqual({ name: '重命名后' }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
  })

  it('TF-035 右键菜单：删除项目（二次确认 + DELETE）', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    let deleted = false
    server.use(
      http.delete(`${DAEMON_BASE_URL}/api/projects/1`, () => {
        deleted = true
        return HttpResponse.json({ code: 0, data: { removed: true } })
      }),
    )
    const user = userEvent.setup()
    renderLayout()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'TangoForge' })).toBeInTheDocument(),
    )
    await user.pointer({
      keys: '[MouseRight]',
      target: screen.getByRole('button', { name: 'TangoForge' }),
    })
    await user.click(await screen.findByRole('menuitem', { name: /删除项目/ }))
    // 确认对话框。
    await screen.findByText(/删除项目「TangoForge」/)
    await user.click(screen.getByRole('button', { name: '确认删除' }))
    await waitFor(() => expect(deleted).toBe(true))
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
  })

  it('TF-035 删除当前选中项目 → 取消选中 + 重定向到项目概览页', async () => {
    server.use(
      http.delete(`${DAEMON_BASE_URL}/api/projects/1`, () =>
        HttpResponse.json({ code: 0, data: { removed: true } }),
      ),
    )
    const user = userEvent.setup()
    renderLayout()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'TangoForge' })).toBeInTheDocument(),
    )
    // 先进入项目（当前选中 + 路由在看板）。
    await user.click(screen.getByRole('button', { name: 'TangoForge' }))
    await screen.findByText('看板内容')
    expect(useProjectStore.getState().project).toBe('/data/projects/tangoforge')

    // 右键删除当前项目 → 确认。
    await user.pointer({
      keys: '[MouseRight]',
      target: screen.getByRole('button', { name: 'TangoForge' }),
    })
    await user.click(await screen.findByRole('menuitem', { name: /删除项目/ }))
    await screen.findByText(/删除项目「TangoForge」/)
    await user.click(screen.getByRole('button', { name: '确认删除' }))

    // 断言：取消选中 + 重定向到概览页（/ 渲染「概览内容」）。
    await waitFor(() => expect(useProjectStore.getState().project).toBeNull())
    expect(await screen.findByText('概览内容')).toBeInTheDocument()
  })

  it('TF-035 右键菜单：在文件夹中打开（Electron shell；Web 环境提示不可用）', async () => {
    const toastSpy = vi.spyOn(toast, 'error').mockImplementation(() => '')
    // Web 环境：window.tangoforge 未注入（beforeEach 已置 undefined）→ 提示不可用。
    const user = userEvent.setup()
    renderLayout()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'TangoForge' })).toBeInTheDocument(),
    )
    await user.pointer({
      keys: '[MouseRight]',
      target: screen.getByRole('button', { name: 'TangoForge' }),
    })
    await user.click(await screen.findByRole('menuitem', { name: /在文件夹中打开/ }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    expect(String(toastSpy.mock.calls[0]?.[0])).toContain('仅桌面版可用')
    toastSpy.mockRestore()
  })
})
