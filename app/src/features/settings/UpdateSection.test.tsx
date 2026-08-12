import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { UpdateSection } from './UpdateSection'
import type { UpdatePayload, UpdateStatus } from '@/types/update'

let pushState: ((p: UpdatePayload) => void) | null = null

function mockUpdate(overrides: Partial<UpdateStatus> = {}) {
  const base: UpdateStatus = {
    state: 'idle',
    currentVersion: '0.5.0',
    supported: true,
    ...overrides,
  }
  const getState = vi.fn().mockResolvedValue(base)
  const check = vi.fn().mockResolvedValue(true)
  const download = vi.fn().mockResolvedValue(true)
  const install = vi.fn().mockResolvedValue(true)
  const openDownload = vi.fn().mockResolvedValue(true)
  const onState = vi.fn().mockImplementation((cb: (p: UpdatePayload) => void) => {
    pushState = cb
    return () => {
      pushState = null
    }
  })
  Object.defineProperty(window, 'tangoforge', {
    value: { update: { getState, check, download, install, openDownload, onState } },
    configurable: true,
  })
  return { getState, check, download, install, openDownload, onState }
}

const emit = (p: UpdatePayload) => act(() => pushState?.(p))

describe('UpdateSection（TF-036 在线更新板块）', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    pushState = null
    Object.defineProperty(window, 'tangoforge', { value: undefined, configurable: true })
  })

  it('Web 环境（无 window.tangoforge.update）：提示仅桌面端可用', () => {
    Object.defineProperty(window, 'tangoforge', { value: { api: {} }, configurable: true })
    render(<UpdateSection />)
    expect(screen.getByText(/在线更新仅在桌面端安装版/)).toBeInTheDocument()
  })

  it('未打包构建（supported=false）：显示版本 + 不可用提示，无检查按钮', async () => {
    mockUpdate({ supported: false })
    render(<UpdateSection />)
    expect(await screen.findByText(/0\.5\.0/)).toBeInTheDocument()
    expect(screen.getByText(/当前为开发\/未打包构建/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '检查更新' })).not.toBeInTheDocument()
  })

  it('点击检查更新 → 主进程推送 available（Windows，无 downloadUrl）→ 下载并安装', async () => {
    const { check, download } = mockUpdate()
    const user = userEvent.setup()
    render(<UpdateSection />)
    await user.click(await screen.findByRole('button', { name: '检查更新' }))
    expect(check).toHaveBeenCalledTimes(1)
    emit({ state: 'available', version: '0.6.0', releaseNotes: '新功能说明' })
    expect(screen.getByText('发现新版本 v0.6.0')).toBeInTheDocument()
    expect(screen.getByText('新功能说明')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '下载并安装' }))
    expect(download).toHaveBeenCalledTimes(1)
  })

  it('macOS（available 且带 downloadUrl）→ 打开下载按钮 → openDownload', async () => {
    const { openDownload } = mockUpdate()
    const user = userEvent.setup()
    render(<UpdateSection />)
    await screen.findByRole('button', { name: '检查更新' })
    emit({
      state: 'available',
      version: '0.6.0',
      downloadUrl:
        'https://github.com/PeiJingbobo/TangoForge/releases/download/v0.6.0/TangoForge-0.6.0.dmg',
    })
    await user.click(await screen.findByRole('button', { name: '打开下载' }))
    expect(openDownload).toHaveBeenCalledTimes(1)
  })

  it('下载进度推送 → 显示百分比', async () => {
    mockUpdate()
    render(<UpdateSection />)
    await screen.findByRole('button', { name: '检查更新' })
    emit({ state: 'downloading', percent: 42 })
    expect(screen.getByText(/正在下载更新… 42%/)).toBeInTheDocument()
  })

  it('downloaded → 重启并安装按钮 → install 调用', async () => {
    const { install } = mockUpdate()
    const user = userEvent.setup()
    render(<UpdateSection />)
    await screen.findByRole('button', { name: '检查更新' })
    emit({ state: 'downloaded', version: '0.6.0' })
    await user.click(await screen.findByRole('button', { name: '重启并安装' }))
    expect(install).toHaveBeenCalledTimes(1)
  })

  it('error → 错误提示 + 重试按钮再次检查', async () => {
    const { check } = mockUpdate()
    const user = userEvent.setup()
    render(<UpdateSection />)
    await screen.findByRole('button', { name: '检查更新' })
    emit({ state: 'error', error: '网络错误' })
    expect(screen.getByText(/检查更新失败：网络错误/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '重试' }))
    expect(check).toHaveBeenCalledTimes(1)
  })

  it('not-available → 已是最新版本提示', async () => {
    mockUpdate()
    render(<UpdateSection />)
    await screen.findByRole('button', { name: '检查更新' })
    emit({ state: 'not-available' })
    expect(screen.getByText('当前已是最新版本')).toBeInTheDocument()
  })
})
