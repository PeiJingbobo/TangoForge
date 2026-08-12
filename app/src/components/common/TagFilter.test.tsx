import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TagFilter } from './TagFilter'

describe('TagFilter（标签多选筛选，看板/导航复用）', () => {
  const TAGS = ['前端', '交互', '后端']

  it('渲染全部标签 + 「全部」清空入口，高度与 TabsList 对齐（h-9）', () => {
    render(<TagFilter tags={TAGS} selected={new Set()} onChange={() => {}} />)
    expect(screen.getByRole('button', { name: '全部' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '#前端' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '#交互' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '#后端' })).toBeInTheDocument()
    // 容器 h-9 + overflow-x-auto（单行横向滚动）+ 隐藏原生滚动条
    const container = screen.getByLabelText('按标签筛选')
    expect(container.className).toContain('h-9')
    expect(container.className).toContain('overflow-x-auto')
    expect(container.className).toContain('scrollbar-width:none')
  })

  it('鼠标滚轮：垂直 wheel 增量转为容器水平滚动', async () => {
    render(<TagFilter tags={TAGS} selected={new Set()} onChange={() => {}} />)
    const container = screen.getByLabelText('按标签筛选')
    // 模拟可横向滚动的容器（scrollWidth > clientWidth）
    Object.defineProperty(container, 'scrollWidth', { value: 400, configurable: true })
    Object.defineProperty(container, 'clientWidth', { value: 200, configurable: true })
    container.scrollLeft = 0
    const fire = (deltaY: number) =>
      container.dispatchEvent(new WheelEvent('wheel', { deltaY, bubbles: true, cancelable: true }))
    // 向下滚轮 → scrollLeft 增加
    fire(120)
    expect(container.scrollLeft).toBe(120)
    // 向上滚轮 → scrollLeft 回退
    fire(-60)
    expect(container.scrollLeft).toBe(60)
  })

  it('容器不可滚动时 wheel 不拦截（页面可正常滚动）', () => {
    render(<TagFilter tags={TAGS} selected={new Set()} onChange={() => {}} />)
    const container = screen.getByLabelText('按标签筛选')
    // scrollWidth == clientWidth（无溢出）
    Object.defineProperty(container, 'scrollWidth', { value: 200, configurable: true })
    Object.defineProperty(container, 'clientWidth', { value: 200, configurable: true })
    container.scrollLeft = 0
    const evt = new WheelEvent('wheel', { deltaY: 120, bubbles: true, cancelable: true })
    container.dispatchEvent(evt)
    expect(evt.defaultPrevented).toBe(false)
    expect(container.scrollLeft).toBe(0)
  })

  it('未滚到最右时右端悬浮箭头：仅容器 hover 时淡入（带过渡动画）', async () => {
    render(<TagFilter tags={TAGS} selected={new Set()} onChange={() => {}} />)
    const container = screen.getByLabelText('按标签筛选')
    // 有溢出（scrollWidth 400 > clientWidth 200）且未滚到底 → 箭头渲染但默认隐藏
    Object.defineProperty(container, 'scrollWidth', { value: 400, configurable: true })
    Object.defineProperty(container, 'clientWidth', { value: 200, configurable: true })
    container.scrollLeft = 0
    container.dispatchEvent(new Event('scroll'))
    const arrow = await screen.findByRole('button', { name: '更多标签' })
    // 默认 opacity-0 / pointer-events-none；hover 时经 group-hover 淡入 + 右滑动画
    expect(arrow.className).toContain('opacity-0')
    expect(arrow.className).toContain('pointer-events-none')
    expect(arrow.className).toContain('group-hover:opacity-100')
    expect(arrow.className).toContain('group-hover:translate-x-0')
    expect(arrow.className).toContain('transition-all')

    // 点击箭头 → scrollTo 被调用（jsdom 缺 scrollTo，直接注入 mock）
    const scrollTo = vi.fn((opts: ScrollToOptions) => {
      container.scrollLeft = (opts as { left: number }).left
    })
    // @ts-expect-error jsdom 元素未实现 scrollTo
    container.scrollTo = scrollTo
    fireEvent.click(arrow)
    expect(scrollTo).toHaveBeenCalled()
    expect(container.scrollLeft).toBeGreaterThan(0)
  })

  it('滚动到最右后箭头消失', async () => {
    render(<TagFilter tags={TAGS} selected={new Set()} onChange={() => {}} />)
    const container = screen.getByLabelText('按标签筛选')
    Object.defineProperty(container, 'scrollWidth', { value: 400, configurable: true })
    Object.defineProperty(container, 'clientWidth', { value: 200, configurable: true })
    // 已滚到底（scrollLeft = max = 200）
    container.scrollLeft = 200
    container.dispatchEvent(new Event('scroll'))
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: '更多标签' })).not.toBeInTheDocument(),
    )
  })

  it('点击标签多选切换，onChange 收到新 Set', async () => {
    const user = userEvent.setup()
    const received: string[][] = []
    const onChange = (next: Set<string>) => received.push([...next].sort())
    const selected = new Set<string>()
    const ui = (sel: Set<string>) => <TagFilter tags={TAGS} selected={sel} onChange={onChange} />
    const { rerender } = render(ui(selected))
    // 选择 前端
    await user.click(screen.getByRole('button', { name: '#前端' }))
    rerender(ui(new Set(['前端'])))
    // 追加 交互（多选）
    await user.click(screen.getByRole('button', { name: '#交互' }))
    rerender(ui(new Set(['前端', '交互'])))
    // 取消 前端
    await user.click(screen.getByRole('button', { name: '#前端' }))
    rerender(ui(new Set(['交互'])))

    expect(received).toEqual([['前端'], ['交互', '前端'], ['交互']])
    expect(screen.getByRole('button', { name: '#交互' }).getAttribute('aria-pressed')).toBe('true')
    expect(screen.getByRole('button', { name: '#前端' }).getAttribute('aria-pressed')).toBe('false')
  })

  it('「全部」清空选择', async () => {
    const user = userEvent.setup()
    const onChange = (next: Set<string>) => expect(next.size).toBe(0)
    render(<TagFilter tags={TAGS} selected={new Set(['前端'])} onChange={onChange} />)
    await user.click(screen.getByRole('button', { name: '全部' }))
  })

  it('无标签时不渲染', () => {
    const { container } = render(<TagFilter tags={[]} selected={new Set()} onChange={() => {}} />)
    expect(container.firstChild).toBeNull()
  })
})
