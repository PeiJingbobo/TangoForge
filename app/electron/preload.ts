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
    /** 纯探活（不拉起），供 UI 状态指示点轮询 */
    status: (): Promise<boolean> => ipcRenderer.invoke('daemon:status'),
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
    /** 系统文件选择器（Markdown 多选）；取消返回 null */
    selectFiles: (): Promise<string[] | null> => ipcRenderer.invoke('dialog:selectFiles'),
  },
  /** 系统文件操作（TF-035 右键菜单「在文件夹中打开」） */
  shell: {
    /** 在系统文件管理器中显示目录（不存在则打开上级）；成功返回 true */
    revealPath: (path: string): Promise<boolean> => ipcRenderer.invoke('shell:revealPath', path),
    /** 用系统默认应用打开文件/目录（TF-039 导出记录「打开文件」）；成功返回 true */
    openPath: (path: string): Promise<boolean> => ipcRenderer.invoke('shell:openPath', path),
  },
  /** 窗口控制（TF-038 自绘标题栏）：平台 + 最小化/最大化切换/关闭 + 最大化状态 */
  window: {
    /** 主进程平台（darwin / win32 / linux / web）——渲染层据此决定标题栏形态 */
    platform: process.platform as string,
    minimize: (): Promise<void> => ipcRenderer.invoke('window:minimize'),
    toggleMaximize: (): Promise<void> => ipcRenderer.invoke('window:toggleMaximize'),
    close: (): Promise<void> => ipcRenderer.invoke('window:close'),
    isMaximized: (): Promise<boolean> => ipcRenderer.invoke('window:isMaximized'),
    /** 订阅最大化状态变化（Windows 自绘按钮图标切换）；返回取消订阅函数 */
    onMaximizedChange: (cb: (maximized: boolean) => void): (() => void) => {
      const handler = (_e: IpcRendererEvent, maximized: boolean): void => cb(maximized)
      ipcRenderer.on('window:maximized-change', handler)
      return () => ipcRenderer.removeListener('window:maximized-change', handler)
    },
  },
  /** CLI 全局注册管理（全局设置页「CLI 板块」）：状态检测 / 注册 / 更新 / 卸载 */
  cli: {
    status: (): Promise<{
      registered: boolean
      path: string | null
      ours: boolean
      cliPath: string
    }> => ipcRenderer.invoke('cli:status'),
    register: (): Promise<{ ok: boolean; message: string }> => ipcRenderer.invoke('cli:register'),
    /** 已注册但指向其他版本（dev/旧版）时，更新为当前 App 的 CLI 并保持优先 */
    update: (): Promise<{ ok: boolean; message: string }> => ipcRenderer.invoke('cli:update'),
    unregister: (): Promise<{ ok: boolean; message: string }> =>
      ipcRenderer.invoke('cli:unregister'),
  },
  /** 剪贴板（QA 2026-08-09：任务编号复制；file:// 下 navigator.clipboard 不可靠） */
  clipboard: {
    writeText: (text: string): Promise<boolean> => ipcRenderer.invoke('clipboard:writeText', text),
  },
}

contextBridge.exposeInMainWorld('tangoforge', api)

export type TangoForgeAPI = typeof api
