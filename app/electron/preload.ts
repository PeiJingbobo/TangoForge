import { contextBridge, ipcRenderer, type IpcRendererEvent } from 'electron'

/**
 * 白名单 IPC API（docs/TECHNICAL.md §4.4 + Electron 官方安全清单）：
 * 渲染进程只做 UI；数据请求（HTTP/WS/IO）全部委托主进程，经此白名单透出。
 * 禁止直接暴露 Node / Electron / ipcRenderer 能力。
 */
export interface ApiRequestPayload {
  method?: string
  path: string
  body?: unknown
  project?: string
}

const api = {
  daemon: {
    ensureRunning: (): Promise<boolean> => ipcRenderer.invoke('daemon:ensureRunning'),
  },
  /** HTTP 代理：主进程 fetch daemon（token 主进程持有，path 白名单） */
  api: {
    request: (req: ApiRequestPayload): Promise<unknown> =>
      ipcRenderer.invoke('daemon:apiRequest', req),
  },
  /** WS 事件订阅：主进程持有连接，事件推送渲染进程；返回取消订阅函数 */
  events: {
    setProject: (project: string | null): Promise<boolean> =>
      ipcRenderer.invoke('daemon:events:setProject', project),
    on: (callback: (event: unknown) => void): (() => void) => {
      const handler = (_e: IpcRendererEvent, event: unknown): void => callback(event)
      ipcRenderer.on('daemon:event', handler)
      return () => ipcRenderer.removeListener('daemon:event', handler)
    },
  },
  dialog: {
    /** 系统目录选择器；取消返回 null */
    selectDirectory: (): Promise<string | null> => ipcRenderer.invoke('dialog:selectDirectory'),
  },
}

contextBridge.exposeInMainWorld('tangoforge', api)

export type TangoForgeAPI = typeof api
