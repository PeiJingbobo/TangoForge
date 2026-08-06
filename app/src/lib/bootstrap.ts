import { setUiToken } from '@/api/client'

/**
 * 应用启动引导（Electron 环境）：
 * 1. 探活/拉起 daemon（127.0.0.1:19810）；
 * 2. 读取全局配置 ui_token 注入 HTTP 客户端（回环 + X-UI-Token = UI 身份）。
 * Web 模式（window.tangoforge 不存在）跳过，返回 false。
 */
export async function bootstrapDaemon(): Promise<boolean> {
  const api = window.tangoforge
  if (!api) return false
  const ok = await api.daemon.ensureRunning()
  const token = await api.config.readUiToken()
  if (token) setUiToken(token)
  return ok
}
