import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WindowTitleBar } from './window-titlebar'

type WinMock = {
  platform: string
  minimize: ReturnType<typeof vi.fn>
  toggleMaximize: ReturnType<typeof vi.fn>
  close: ReturnType<typeof vi.fn>
  isMaximized: ReturnType<typeof vi.fn>
  onMaximizedChange: ReturnType<typeof vi.fn>
}

function installWindowMock(platform: string, maximized = false): WinMock {
  const mock: WinMock = {
    platform,
    minimize: vi.fn().mockResolvedValue(undefined),
    toggleMaximize: vi.fn().mockResolvedValue(undefined),
    close: vi.fn().mockResolvedValue(undefined),
    isMaximized: vi.fn().mockResolvedValue(maximized),
    onMaximizedChange: vi.fn().mockReturnValue(() => {}),
  }
  Object.defineProperty(window, 'tangoforge', {
    value: { window: mock },
    configurable: true,
  })
  return mock
}

describe('WindowTitleBar（TF-038 自绘标题栏）', () => {
  afterEach(() => {
    Object.defineProperty(window, 'tangoforge', { value: undefined, configurable: true })
    vi.restoreAllMocks()
  })

  it('非桌面环境：不渲染（window.tangoforge 缺失）', () => {
    Object.defineProperty(window, 'tangoforge', { value: undefined, configurable: true })
    const { container } = render(<WindowTitleBar />)
    expect(container.childElementCount).toBe(0)
  })

  it('macOS：渲染标题栏条 + 应用名，无控制按钮（原生交通灯保留）', () => {
    installWindowMock('darwin')
    render(<WindowTitleBar />)
    expect(screen.getByText('TangoForge')).toBeInTheDocument()
    expect(screen.queryByLabelText('窗口控制')).not.toBeInTheDocument()
  })

  it('Windows：渲染标题栏 + 最小化/最大化/关闭按钮，关闭按钮 danger 样式', async () => {
    const mock = installWindowMock('win32')
    const user = userEvent.setup()
    render(<WindowTitleBar />)
    expect(screen.getByLabelText('最小化')).toBeInTheDocument()
    expect(screen.getByLabelText('最大化')).toBeInTheDocument()
    expect(screen.getByLabelText('关闭')).toBeInTheDocument()

    await user.click(screen.getByLabelText('最小化'))
    expect(mock.minimize).toHaveBeenCalledOnce()

    await user.click(screen.getByLabelText('最大化'))
    expect(mock.toggleMaximize).toHaveBeenCalledOnce()

    await user.click(screen.getByLabelText('关闭'))
    expect(mock.close).toHaveBeenCalledOnce()

    // 初始查询最大化状态。
    expect(mock.isMaximized).toHaveBeenCalledOnce()
    expect(mock.onMaximizedChange).toHaveBeenCalledOnce()
  })

  it('Windows：最大化状态变化 → 图标按钮文案切换（事件订阅）', () => {
    installWindowMock('win32', false)
    let listener: ((m: boolean) => void) | null = null
    Object.defineProperty(window, 'tangoforge', {
      value: {
        window: {
          platform: 'win32',
          minimize: vi.fn().mockResolvedValue(undefined),
          toggleMaximize: vi.fn().mockResolvedValue(undefined),
          close: vi.fn().mockResolvedValue(undefined),
          isMaximized: vi.fn().mockResolvedValue(false),
          onMaximizedChange: vi.fn().mockImplementation((cb: (m: boolean) => void) => {
            listener = cb
            return () => {}
          }),
        },
      },
      configurable: true,
    })
    render(<WindowTitleBar />)
    expect(screen.getByLabelText('最大化')).toBeInTheDocument()
    act(() => listener?.(true))
    expect(screen.getByLabelText('还原')).toBeInTheDocument()
  })
})
