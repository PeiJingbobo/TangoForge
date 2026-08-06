import { ipcMain } from 'electron'

/**
 * 内嵌守护进程管理（docs/TECHNICAL.md §4.4）：
 * 1. 探活 127.0.0.1:19810（GET /ping）；
 * 2. 未存活则 spawn 内嵌 daemon 二进制并等待 Health Check 通过；
 * 3. 单实例检查：端口 + PID 文件双重检测，避免重复拉起。
 *
 * TODO: 完整实现见 docs/AGENTS.md「当前开发阶段重点任务」#3（cmd/daemon 最小骨架
 * 已提供 /ping 端点）。本骨架先注册 IPC 通道，业务逻辑随 daemon 集成逐步补齐。
 */
export function ensureDaemonRunning(): Promise<boolean> {
  // TODO: 实现探活与 spawn 逻辑。
  return Promise.resolve(false)
}

export function registerDaemonIpc(): void {
  ipcMain.handle('daemon:ensureRunning', () => ensureDaemonRunning())
}

export function registerConfigIpc(): void {
  ipcMain.handle('config:readUiToken', () => {
    // TODO: 从 ~/.taskboard-app/config.yaml 读取 ui_token（全局配置由 internal/config 管理）。
    return ''
  })
}
