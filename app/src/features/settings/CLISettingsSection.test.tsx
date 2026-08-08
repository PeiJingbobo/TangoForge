import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'sonner'
import { CLISettingsSection, type CliStatus } from './CLISettingsSection'

const CLI_PATH = 'C:\\Program Files\\TangoForge\\resources\\bin\\tangoforge.exe'

function mockCli(overrides: Partial<CliStatus> = {}) {
  const base: CliStatus = {
    registered: false,
    path: null,
    ours: false,
    cliPath: CLI_PATH,
    ...overrides,
  }
  const status = vi.fn().mockResolvedValue(base)
  const register = vi.fn().mockResolvedValue({ ok: true, message: '已注册到全局' })
  const unregister = vi.fn().mockResolvedValue({ ok: true, message: '已卸载注册' })
  Object.defineProperty(window, 'tangoforge', {
    value: { cli: { status, register, unregister } },
    configurable: true,
  })
  return { status, register, unregister }
}

describe('CLISettingsSection（QA 2026-08-08 CLI 板块）', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    Object.defineProperty(window, 'tangoforge', { value: undefined, configurable: true })
  })

  it('未注册：警示文案 + 注册按钮 + 本机 CLI 路径显示', async () => {
    mockCli()
    render(<CLISettingsSection />)
    expect(await screen.findByText('未注册')).toBeInTheDocument()
    expect(screen.getByText(/尚未注册到全局命令/)).toBeInTheDocument()
    expect(screen.getByText('注册到全局')).toBeInTheDocument()
    expect(screen.getByText(CLI_PATH)).toBeInTheDocument()
  })

  it('点击注册 → 调用 register + toast + 刷新状态', async () => {
    const { register, status } = mockCli()
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    const user = userEvent.setup()
    render(<CLISettingsSection />)
    await user.click(await screen.findByRole('button', { name: '注册到全局' }))
    expect(register).toHaveBeenCalledTimes(1)
    await waitFor(() => expect(toastSpy).toHaveBeenCalledWith('已注册到全局'))
    // 注册后刷新状态（第二次 status 调用）。
    await waitFor(() => expect(status).toHaveBeenCalledTimes(2))
    toastSpy.mockRestore()
  })

  it('已注册：绿色徽标 + 解析路径 + 卸载按钮；点击卸载 → unregister + toast', async () => {
    const { unregister } = mockCli({ registered: true, path: CLI_PATH, ours: true })
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    const user = userEvent.setup()
    render(<CLISettingsSection />)
    expect(await screen.findByText('已注册')).toBeInTheDocument()
    // 路径出现在「本机 CLI 路径」与「当前解析」两处。
    expect(screen.getAllByText(CLI_PATH).length).toBeGreaterThan(0)
    await user.click(screen.getByRole('button', { name: '卸载注册' }))
    expect(unregister).toHaveBeenCalledTimes(1)
    await waitFor(() => expect(toastSpy).toHaveBeenCalledWith('已卸载注册'))
    toastSpy.mockRestore()
  })

  it('解析到非本 App CLI：提示非本 App 分发', async () => {
    mockCli({ registered: true, path: '/usr/local/bin/tangoforge', ours: false })
    render(<CLISettingsSection />)
    expect(await screen.findByText(/非本 App 分发的 CLI/)).toBeInTheDocument()
  })

  it('Web 环境（无 window.tangoforge.cli）：提示仅桌面端可用', () => {
    Object.defineProperty(window, 'tangoforge', {
      value: { api: {} },
      configurable: true,
    })
    render(<CLISettingsSection />)
    expect(screen.getByText(/仅在桌面端 App 中可用/)).toBeInTheDocument()
  })
})
