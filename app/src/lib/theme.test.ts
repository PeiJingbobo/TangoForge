import { describe, it, expect, beforeEach } from 'vitest'
import {
  hexToRgb,
  rgbToHex,
  mixColor,
  luminance,
  lightScale,
  darkScale,
  accentForeground,
  applyAccent,
  applyThemeMode,
  applyAccentVariables,
  clearAccentVariables,
  ACCENT_PRESETS,
  PRIMARY_STEPS,
} from './theme'

const SKY = '#2292D8'

describe('颜色工具', () => {
  it('hex→rgb→hex 往返一致', () => {
    expect(rgbToHex(hexToRgb(SKY))).toBe(SKY)
    expect(hexToRgb('#abc')).toEqual({ r: 0xaa, g: 0xbb, b: 0xcc })
  })

  it('mixColor 线性混合', () => {
    const w = hexToRgb('#FFFFFF')
    const b = hexToRgb('#000000')
    expect(mixColor(w, b, 0.5)).toEqual({ r: 127.5, g: 127.5, b: 127.5 })
  })

  it('luminance：深色低、亮色高', () => {
    expect(luminance(hexToRgb('#1E3A8A'))).toBeLessThan(0.1)
    expect(luminance(hexToRgb('#FFDD00'))).toBeGreaterThan(0.6)
    expect(luminance(hexToRgb(SKY))).toBeCloseTo(0.259, 2)
  })
})

describe('色阶计算', () => {
  const base = hexToRgb(SKY)

  it('lightScale：500 为原色，级数完整', () => {
    const s = lightScale(base)
    expect(s['500']).toBe(SKY)
    expect(PRIMARY_STEPS.length).toBe(10)
    PRIMARY_STEPS.forEach((step) => {
      expect(s[step]).toMatch(/^#[0-9a-f]{6}$/i)
    })
  })

  it('lightScale：50 最亮、900 最暗（单调递减亮度）', () => {
    const s = lightScale(base)
    const lum = (h: string) => luminance(hexToRgb(h))
    expect(lum(s['50'])).toBeGreaterThan(lum(s['500']))
    expect(lum(s['500'])).toBeGreaterThan(lum(s['900']))
    expect(lum(s['100'])).toBeGreaterThan(lum(s['200']))
    expect(lum(s['700'])).toBeGreaterThan(lum(s['900']))
  })

  it('darkScale：500 比原色提亮，50 压暗作 soft 底', () => {
    const d = darkScale(base)
    expect(luminance(hexToRgb(d['500']))).toBeGreaterThan(luminance(base))
    expect(luminance(hexToRgb(d['50']))).toBeLessThan(luminance(base))
  })

  it('accentForeground：亮色深字、暗色白字', () => {
    expect(accentForeground(hexToRgb('#FFDD00'))).toBe('#16161A')
    expect(accentForeground(hexToRgb('#1E3A8A'))).toBe('#FFFFFF')
    expect(accentForeground(base)).toBe('#FFFFFF') // #2292D8 亮度 0.259
  })
})

describe('DOM 应用（jsdom）', () => {
  beforeEach(() => {
    document.documentElement.removeAttribute('data-accent')
    document.documentElement.classList.remove('dark')
    clearAccentVariables()
  })

  it('预设主色：走 data-accent，无 inline 覆盖', () => {
    applyAccent('sky-blue', 'light')
    const root = document.documentElement
    expect(root.dataset.accent).toBe('sky-blue')
    expect(root.style.getPropertyValue('--primary-500')).toBe('')
  })

  it('自定义主色：写入十级变量 + --primary + dataset=custom', () => {
    applyAccent(SKY, 'light')
    const root = document.documentElement
    expect(root.dataset.accent).toBe('custom')
    expect(root.style.getPropertyValue('--primary-500')).toBe(SKY)
    expect(root.style.getPropertyValue('--primary')).toBe(SKY)
    expect(root.style.getPropertyValue('--primary-50')).toBe('#EDF6FC')
  })

  it('预设主色：清除自定义 inline（含 --primary），回退 CSS 预设', () => {
    applyAccent(SKY, 'light')
    const root = document.documentElement
    applyAccent('sky-blue', 'light')
    expect(root.dataset.accent).toBe('sky-blue')
    expect(root.style.getPropertyValue('--primary')).toBe('')
    expect(root.style.getPropertyValue('--primary-500')).toBe('')
  })

  it('明暗切换：dark class 生效；自定义主色按主题重算', () => {
    applyAccent(SKY, 'light')
    const root = document.documentElement
    expect(root.style.getPropertyValue('--primary-500')).toBe(SKY)
    applyThemeMode('dark')
    expect(root.classList.contains('dark')).toBe(true)
    // dark 下 500 提亮，不再是原色
    expect(root.style.getPropertyValue('--primary-500')).not.toBe(SKY)
    // 深色模式前景色固定深字
    expect(root.style.getPropertyValue('--primary-foreground')).toBe('#0E1428')
  })

  it('预设主色切明暗：不产生 inline 覆盖', () => {
    applyAccent('forge-amber', 'light')
    applyThemeMode('dark')
    expect(document.documentElement.dataset.accent).toBe('forge-amber')
    expect(document.documentElement.style.getPropertyValue('--primary-500')).toBe('')
  })

  it('applyAccentVariables + clearAccentVariables 往返', () => {
    applyAccentVariables(SKY, 'light')
    expect(document.documentElement.style.getPropertyValue('--primary-900')).toBe('#0D3752')
    clearAccentVariables()
    expect(document.documentElement.style.getPropertyValue('--primary-900')).toBe('')
  })
})

describe('预设配置', () => {
  it('4 个预设与 globals.css 的 data-accent 一致', () => {
    expect(ACCENT_PRESETS.map((p) => p.key)).toEqual([
      'sky-blue',
      'forge-amber',
      'growth-green',
      'ai-violet',
    ])
    expect(ACCENT_PRESETS[0].hex).toBe(SKY)
  })
})
