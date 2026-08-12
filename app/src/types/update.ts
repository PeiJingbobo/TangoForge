/**
 * 在线更新共享类型（TF-036，CI-CD-UPDATER.md）：
 * 主进程（app/electron/updater.ts）与渲染进程（UpdateSection）共用。
 * 平台策略：
 * - Windows：electron-updater 全链路（检测→下载→安装），latest.yml 元数据；
 * - macOS（未签名阶段）：仅检测新版本，自动打开 dmg 下载页由用户手动安装。
 */

export type UpdateState =
  'idle' | 'checking' | 'available' | 'not-available' | 'downloading' | 'downloaded' | 'error'

/** 主进程 → 渲染进程的更新状态载荷（IPC update:state + update:getState） */
export interface UpdatePayload {
  state: UpdateState
  /** 可用/已下载的新版本号 */
  version?: string
  /** GitHub Release body（变更说明） */
  releaseNotes?: string
  /** macOS：dmg 下载地址（未签名阶段手动安装） */
  downloadUrl?: string
  /** 下载进度 0-100（Windows） */
  percent?: number
  error?: string
}

/** 渲染进程初始化时拉取的完整状态（update:getState） */
export interface UpdateStatus extends UpdatePayload {
  /** 当前安装版本（app.getVersion） */
  currentVersion: string
  /** 当前平台/环境是否支持在线更新（打包版 darwin/win32） */
  supported: boolean
}
