import type { TangoForgeAPI } from '../../electron/preload'

declare global {
  interface Window {
    /** preload 暴露的白名单 IPC API（docs/TECHNICAL.md §4.4）。 */
    tangoforge: TangoForgeAPI
  }
}

export {}
