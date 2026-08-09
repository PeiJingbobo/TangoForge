import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { toast } from 'sonner'
import { TaskNumberBadge } from './TaskNumberBadge'

describe('TaskNumberBadge（任务编号徽标）', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('有编号：渲染编号徽标；无编号：不渲染', () => {
    const { rerender } = render(<TaskNumberBadge number="T006" />)
    expect(screen.getByText('T006')).toBeInTheDocument()
    rerender(<TaskNumberBadge number={null} />)
    expect(screen.queryByText('T006')).not.toBeInTheDocument()
  })

  it('点击复制编号 → 剪贴板写入 + toast（QA 2026-08-09）', async () => {
    const writeText = vi.fn().mockResolvedValue(true)
    // jsdom navigator 为 Proxy 无法 defineProperty 覆盖 → mock Electron IPC 通道
    Object.defineProperty(window, 'tangoforge', {
      value: { clipboard: { writeText } },
      configurable: true,
    })
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    render(<TaskNumberBadge number="T006" />)
    fireEvent.click(screen.getByRole('button', { name: '复制编号 T006' }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('T006'))
    expect(toastSpy).toHaveBeenCalledWith('已复制编号 T006')
    toastSpy.mockRestore()
  })

  it('点击不冒泡（不触发所在行打开详情）', () => {
    const onRowClick = vi.fn()
    render(
      <div role="button" onClick={onRowClick}>
        <TaskNumberBadge number="T007" />
      </div>,
    )
    fireEvent.click(screen.getByRole('button', { name: '复制编号 T007' }))
    expect(onRowClick).not.toHaveBeenCalled()
  })
})
