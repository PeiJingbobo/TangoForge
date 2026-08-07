import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { ExportPanel } from './ExportPanel'
import { addExportRecord, getExportRecords } from './export-records'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { toast } from 'sonner'

const EXPORT_RESULT = {
  content: '# 导出内容\n\n- [ ] 任务一',
  path: '/data/projects/tf/.taskboard/export.md',
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('export-records（localStorage）', () => {
  beforeEach(() => localStorage.clear())
  afterEach(() => localStorage.clear())

  it('追加记录：新纪录在前，按项目隔离', () => {
    addExportRecord('/p1', { path: '/p1/a.md', mode: 'default', taskCount: 3 })
    addExportRecord('/p1', { path: '/p1/b.md', mode: 'llm', taskCount: 5 })
    addExportRecord('/p2', { path: '/p2/c.md', mode: 'default' })

    const p1 = getExportRecords('/p1')
    expect(p1).toHaveLength(2)
    expect(p1[0].path).toBe('/p1/b.md') // 新纪录在前
    expect(p1[0].mode).toBe('llm')
    expect(getExportRecords('/p2')).toHaveLength(1)
  })

  it('超过 50 条裁剪', () => {
    for (let i = 0; i < 55; i++) {
      addExportRecord('/p1', { path: `/p1/${i}.md`, mode: 'default' })
    }
    expect(getExportRecords('/p1')).toHaveLength(50)
  })
})

describe('ExportPanel（TF-039 导出记录 + 完成提示）', () => {
  beforeEach(() => {
    localStorage.clear()
    useProjectStore.setState({ project: '/data/projects/tf' })
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/export`, () =>
        HttpResponse.json({ code: 0, data: EXPORT_RESULT }),
      ),
      http.get(`${DAEMON_BASE_URL}/api/export/template`, () =>
        HttpResponse.json({
          code: 0,
          data: { template: '---\ntitle: "{{.Project.Name}}"\n---', mode: 'default' },
        }),
      ),
    )
  })
  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('无记录：空态提示', () => {
    render(<ExportPanel />, { wrapper })
    expect(screen.getByText(/暂无导出记录/)).toBeInTheDocument()
  })

  it('展示已有导出记录（时间/路径/模式）', () => {
    addExportRecord('/data/projects/tf', {
      path: '/data/projects/tf/.taskboard/export.md',
      mode: 'llm',
    })
    render(<ExportPanel />, { wrapper })
    expect(screen.getByText(/导出记录/)).toBeInTheDocument()
    expect(screen.getByText(/\.taskboard\/export\.md/)).toBeInTheDocument()
    expect(screen.getByText('LLM 模板')).toBeInTheDocument()
  })

  it('导出成功 → 完成提示（路径 + 打开目录/文件按钮）+ 追加记录', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    const user = userEvent.setup()
    render(<ExportPanel />, { wrapper })
    await user.click(screen.getByRole('button', { name: '打开导出' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /导出并写盘/ })).toBeInTheDocument(),
    )
    await user.click(screen.getByRole('button', { name: /导出并写盘/ }))

    // 完成提示 + 路径 + 操作按钮。
    await waitFor(() => expect(screen.getByText('导出完成')).toBeInTheDocument())
    expect(screen.getAllByText(/export\.md/).length).toBeGreaterThanOrEqual(1)
    // 完成提示区：打开目录/打开文件按钮（记录列表也有同名按钮，用 getAllByRole）。
    expect(screen.getAllByRole('button', { name: /打开目录/ }).length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByRole('button', { name: /打开文件/ }).length).toBeGreaterThanOrEqual(1)

    // 记录已追加（进入记录列表）。
    await waitFor(() => expect(screen.getByText(/导出记录/)).toBeInTheDocument())
    expect(localStorage.getItem('tangoforge.export-records')).toContain('export.md')
    toastSpy.mockRestore()
  })

  it('Web 环境：打开目录/文件 → error toast（无桌面 shell）', async () => {
    Object.defineProperty(window, 'tangoforge', { value: undefined, configurable: true })
    const toastSpy = vi.spyOn(toast, 'error').mockImplementation(() => '')
    addExportRecord('/data/projects/tf', {
      path: '/data/projects/tf/.taskboard/export.md',
      mode: 'default',
    })
    const user = userEvent.setup()
    render(<ExportPanel />, { wrapper })
    await user.click(screen.getByRole('button', { name: /打开目录/ }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    expect(toastSpy).toHaveBeenCalledWith('「打开目录」仅桌面版可用（Web 预览不支持）')
    toastSpy.mockRestore()
  })
})
