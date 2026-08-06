import { contextBridge, ipcRenderer } from 'electron'

// 白名单 IPC API（docs/TECHNICAL.md §4.4）：渲染进程只能访问这里暴露的能力，
// 禁止直接暴露 Node / Electron 能力。
const api = {
  daemon: {
    ensureRunning: (): Promise<boolean> => ipcRenderer.invoke('daemon:ensureRunning'),
  },
  config: {
    readUiToken: (): Promise<string> => ipcRenderer.invoke('config:readUiToken'),
  },
}

contextBridge.exposeInMainWorld('tangoforge', api)

export type TangoForgeAPI = typeof api
