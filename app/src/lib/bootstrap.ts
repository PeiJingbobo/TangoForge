/**
 * 应用启动引导（Electron 环境）：
 * 探活/拉起 daemon（127.0.0.1:19810）。
 * 说明：X-UI-Token 由主进程持有并在 API 代理中注入（渲染进程不接触凭据，
 * Electron 最佳实践）；Web 模式（window.tangoforge 不存在）跳过，返回 false。
 */
export async function bootstrapDaemon(): Promise<boolean> {
  const api = window.tangoforge
  if (!api) return false
  return api.daemon.ensureRunning()
}
