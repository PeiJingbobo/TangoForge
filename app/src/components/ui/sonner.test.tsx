import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { toast } from 'sonner'
import { Toaster } from './sonner'

/**
 * TF-037 Toast 容器优化验证：顶部居中 / 手动关闭按钮 / 长文本换行类。
 * sonner 经 portal 渲染到 body；通过全局 DOM 查询断言。
 */
describe('Toaster（TF-037 全局提示优化）', () => {
  beforeEach(() => {
    document.documentElement.classList.remove('dark')
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('渲染位置为顶部居中（toaster 根 data-x-position=center / data-y-position=top）', async () => {
    render(<Toaster />)
    act(() => {
      toast.info('位置测试')
    })
    await screen.findByText('位置测试')
    const root = document.querySelector('[data-sonner-toaster]')
    expect(root).toBeTruthy()
    expect(root?.getAttribute('data-x-position')).toBe('center')
    expect(root?.getAttribute('data-y-position')).toBe('top')
  })

  it('触发 toast → 消息完整展示 + 手动关闭按钮存在 + 换行类', async () => {
    render(<Toaster />)
    act(() => {
      toast.error('这是一段非常长的错误信息，用于验证长文本换行展示是否正常，包含很多字符内容。')
    })
    expect(await screen.findByText(/这是一段非常长的错误信息/)).toBeInTheDocument()
    // 手动关闭按钮（closeButton 开启，aria-label="Close toast"）。
    expect(screen.getByRole('button', { name: 'Close toast' })).toBeInTheDocument()
    // toast 根带换行类（长文本完整展示）。
    const toastEl = document.querySelector('[data-sonner-toast]')
    expect(toastEl).toBeTruthy()
    const cls = toastEl?.getAttribute('class') ?? ''
    expect(cls).toContain('break-words')
    expect(cls).toContain('whitespace-normal')
  })

  it('duration=3000：自动关闭计时（toast 根带 3s 样式计时）', async () => {
    render(<Toaster />)
    act(() => {
      toast.success('自动关闭测试')
    })
    expect(await screen.findByText('自动关闭测试')).toBeInTheDocument()
    const toastEl = document.querySelector('[data-sonner-toast]')
    // sonner 计时条元素（duration 3000 → animation-duration 3s）。
    const timer = toastEl?.querySelector('[data-sonner-timer]') as HTMLElement | null
    if (timer) {
      expect(timer.style.animationDuration).toBe('3s')
    }
  })
})
