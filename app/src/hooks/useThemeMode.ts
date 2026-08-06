import { useCallback, useEffect, useState } from 'react'
import {
  ACCENT_PRESETS,
  applyAccent,
  applyThemeMode,
  resolveMode,
  type ResolvedTheme,
  type ThemeMode,
} from '@/lib/theme'

const MODE_KEY = 'tf-theme-mode'
const ACCENT_KEY = 'tf-accent'

function readStored<T>(key: string, fallback: T): T {
  try {
    const v = localStorage.getItem(key)
    return v === null ? fallback : (v as T)
  } catch {
    return fallback
  }
}

/**
 * 外观偏好 hook（持久化 localStorage；明暗切换 + 主色，与 lib/theme 算法一致）。
 * 无 Provider 依赖，布局/设置页直接使用。
 */
export function useThemeMode() {
  const [mode, setModeState] = useState<ThemeMode>(() => readStored<ThemeMode>(MODE_KEY, 'system'))
  const [resolved, setResolved] = useState<ResolvedTheme>(() => resolveMode(mode))
  const [accent, setAccentState] = useState<string>(() => readStored(ACCENT_KEY, 'sky-blue'))

  useEffect(() => {
    applyThemeMode(mode)
    applyAccent(accent, resolveMode(mode))
  }, [mode, accent])

  const setMode = useCallback((m: ThemeMode) => {
    setModeState(m)
    const r = applyThemeMode(m)
    setResolved(r)
    try {
      localStorage.setItem(MODE_KEY, m)
    } catch {
      // 存储不可用时仅内存生效
    }
    // 自定义主色按主题重算
    if (document.documentElement.dataset.accent !== 'custom') {
      applyAccent(readStored(ACCENT_KEY, 'sky-blue'), r)
    }
  }, [])

  const setAccent = useCallback(
    (keyOrHex: string) => {
      setAccentState(keyOrHex)
      applyAccent(keyOrHex, resolveMode(mode))
      try {
        localStorage.setItem(ACCENT_KEY, keyOrHex)
      } catch {
        // 忽略
      }
    },
    [mode],
  )

  return { mode, resolved, accent, setMode, setAccent, presets: ACCENT_PRESETS }
}

export type { ThemeMode }
