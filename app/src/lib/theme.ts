/**
 * 主题系统（docs/UI-VISION.md §4.1）
 * 单一主强调色：CSS 变量整组（--primary-50 ~ --primary-900）驱动，
 * 自定义主色时按 WCAG 亮度动态计算十级色阶与前景色。
 * 算法移植自 docs/design/visual-reference.html（视觉参考 demo）。
 *
 * 用法：
 * - 预设主色：applyAccent('sky-blue', mode)
 * - 自定义主色：applyAccent('#ff0000', mode)
 * - 明暗：applyThemeMode('light' | 'dark' | 'system')
 */

export type ThemeMode = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'

export interface RGB {
  r: number
  g: number
  b: number
}

export const PRIMARY_STEPS = [
  '50',
  '100',
  '200',
  '300',
  '400',
  '500',
  '600',
  '700',
  '800',
  '900',
] as const
export type PrimaryStep = (typeof PRIMARY_STEPS)[number]
export type AccentScale = Record<PrimaryStep, string>

export interface AccentPreset {
  key: string
  hex: string
  label: string
}

/** 内置预设主色（与 globals.css 中 [data-accent] 色阶一一对应） */
export const ACCENT_PRESETS: AccentPreset[] = [
  { key: 'sky-blue', hex: '#2292D8', label: '天蓝 Sky' },
  { key: 'forge-amber', hex: '#E8912D', label: '琥珀 Forge' },
  { key: 'growth-green', hex: '#2E9C66', label: '青绿 Growth' },
  { key: 'ai-violet', hex: '#7C66EC', label: '紫罗兰 AI' },
]

/** 深色模式主按钮前景色（主色已提亮，固定用深字） */
const DARK_FOREGROUND = '#0E1428'
/** 前景色亮度阈值：超过则用深字，否则白字 */
const FOREGROUND_LUM_THRESHOLD = 0.35
const WHITE: RGB = { r: 255, g: 255, b: 255 }
const BLACK: RGB = { r: 0, g: 0, b: 0 }

export function hexToRgb(hex: string): RGB {
  let h = hex.replace('#', '')
  if (h.length === 3)
    h = h
      .split('')
      .map((c) => c + c)
      .join('')
  return {
    r: parseInt(h.slice(0, 2), 16),
    g: parseInt(h.slice(2, 4), 16),
    b: parseInt(h.slice(4, 6), 16),
  }
}

export function rgbToHex(c: RGB): string {
  const to = (n: number): string =>
    Math.round(Math.min(255, Math.max(0, n)))
      .toString(16)
      .padStart(2, '0')
      .toUpperCase()
  return `#${to(c.r)}${to(c.g)}${to(c.b)}`
}

export function mixColor(a: RGB, b: RGB, t: number): RGB {
  return {
    r: a.r + (b.r - a.r) * t,
    g: a.g + (b.g - a.g) * t,
    b: a.b + (b.b - a.b) * t,
  }
}

/** WCAG 相对亮度（0 ~ 1） */
export function luminance(c: RGB): number {
  const lin = (v: number): number => {
    v /= 255
    return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4)
  }
  return 0.2126 * lin(c.r) + 0.7152 * lin(c.g) + 0.0722 * lin(c.b)
}

/** 浅色色阶：500=原色，50~400 向白混合，600~900 向黑混合 */
export function lightScale(base: RGB): AccentScale {
  return {
    '50': rgbToHex(mixColor(base, WHITE, 0.92)),
    '100': rgbToHex(mixColor(base, WHITE, 0.8)),
    '200': rgbToHex(mixColor(base, WHITE, 0.6)),
    '300': rgbToHex(mixColor(base, WHITE, 0.4)),
    '400': rgbToHex(mixColor(base, WHITE, 0.2)),
    '500': rgbToHex(base),
    '600': rgbToHex(mixColor(base, BLACK, 0.15)),
    '700': rgbToHex(mixColor(base, BLACK, 0.32)),
    '800': rgbToHex(mixColor(base, BLACK, 0.48)),
    '900': rgbToHex(mixColor(base, BLACK, 0.62)),
  }
}

/** 深色色阶：500 提亮，50~200 压暗作 soft 底，700+ 提亮作文字 */
export function darkScale(base: RGB): AccentScale {
  return {
    '50': rgbToHex(mixColor(base, BLACK, 0.72)),
    '100': rgbToHex(mixColor(base, BLACK, 0.55)),
    '200': rgbToHex(mixColor(base, BLACK, 0.38)),
    '300': rgbToHex(mixColor(base, WHITE, 0.04)),
    '400': rgbToHex(mixColor(base, WHITE, 0.15)),
    '500': rgbToHex(mixColor(base, WHITE, 0.32)),
    '600': rgbToHex(mixColor(base, WHITE, 0.48)),
    '700': rgbToHex(mixColor(base, WHITE, 0.62)),
    '800': rgbToHex(mixColor(base, WHITE, 0.76)),
    '900': rgbToHex(mixColor(base, WHITE, 0.88)),
  }
}

/** 主色前景色：按亮度自动选深/浅字，保证按钮文字对比度 */
export function accentForeground(base: RGB): string {
  return luminance(base) > FOREGROUND_LUM_THRESHOLD ? '#16161A' : '#FFFFFF'
}

export function resolveMode(mode: ThemeMode): ResolvedTheme {
  if (mode === 'system') {
    // matchMedia 缺失（jsdom 等）时兜底浅色
    return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  }
  return mode
}

/** 写入自定义主色的 CSS 变量（整组覆盖） */
export function applyAccentVariables(hex: string, mode: ResolvedTheme): void {
  const base = hexToRgb(hex)
  const scale = mode === 'dark' ? darkScale(base) : lightScale(base)
  const fg = mode === 'dark' ? DARK_FOREGROUND : accentForeground(base)
  const root = document.documentElement
  PRIMARY_STEPS.forEach((s) => root.style.setProperty(`--primary-${s}`, scale[s]))
  // --primary 为按钮/链接等组件的「主色」变量（UI-VISION：bg-primary 类引用），必须同步覆盖
  root.style.setProperty('--primary', scale['500'])
  root.style.setProperty('--primary-foreground', fg)
  root.dataset.accent = 'custom'
}

/** 清除自定义主色的 inline 覆盖，回退 CSS 预设 */
export function clearAccentVariables(): void {
  const root = document.documentElement
  PRIMARY_STEPS.forEach((s) => root.style.removeProperty(`--primary-${s}`))
  root.style.removeProperty('--primary')
  root.style.removeProperty('--primary-foreground')
}

/**
 * 应用主色：key 命中预设走 CSS 类；否则视为自定义 hex 动态计算色阶。
 * 明暗切换后需重新调用（自定义色按主题重算）。
 */
export function applyAccent(keyOrHex: string, mode: ResolvedTheme): void {
  const preset = ACCENT_PRESETS.find((p) => p.key === keyOrHex)
  const root = document.documentElement
  if (preset) {
    clearAccentVariables()
    root.dataset.accent = preset.key
  } else {
    applyAccentVariables(keyOrHex, mode)
  }
}

/**
 * 应用明暗模式：切换 html.dark class；若当前为自定义主色，按主题重算色阶。
 * 返回解析后的实际主题。
 */
export function applyThemeMode(mode: ThemeMode): ResolvedTheme {
  const resolved = resolveMode(mode)
  const root = document.documentElement
  root.classList.toggle('dark', resolved === 'dark')
  if (root.dataset.accent === 'custom') {
    const hex = root.style.getPropertyValue('--primary-500').trim()
    if (hex) applyAccentVariables(hex, resolved)
  }
  return resolved
}
