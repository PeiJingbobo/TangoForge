import { describe, it, expect } from 'vitest'
import { getTitleBarHeight } from './window-chrome'

function setPlatform(p: string | undefined) {
  if (p === undefined) {
    // @ts-expect-error 清空 API 模拟非桌面环境
    delete window.tangoforge
    return
  }
  Object.defineProperty(window, 'tangoforge', {
    value: { window: { platform: p } },
    configurable: true,
  })
}

describe('window-chrome（自绘标题栏高度）', () => {
  it('Web 预览（无 tangoforge API）→ 0（不渲染标题栏）', () => {
    setPlatform(undefined)
    expect(getTitleBarHeight()).toBe(0)
  })

  it('macOS（darwin）→ 36px（h-9 自绘标题栏）', () => {
    setPlatform('darwin')
    expect(getTitleBarHeight()).toBe(36)
  })

  it('Windows（win32）→ 36px', () => {
    setPlatform('win32')
    expect(getTitleBarHeight()).toBe(36)
  })

  it('Linux（系统标题栏，无自绘）→ 0', () => {
    setPlatform('linux')
    expect(getTitleBarHeight()).toBe(0)
  })
})
