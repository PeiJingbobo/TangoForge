import { app, BrowserWindow, dialog, ipcMain } from 'electron'
import { spawn, type ChildProcess } from 'child_process'
import { existsSync, readFileSync } from 'fs'
import { homedir } from 'os'
import { join } from 'path'

/**
 * 内嵌守护进程管理（docs/TECHNICAL.md §4.4）：
 * 1. 探活 127.0.0.1:19810（GET /ping，800ms 超时）；
 * 2. 未存活 → spawn daemon 二进制（detached + unref，退出不关守护进程）→ 轮询 ≤5s；
 * 3. 单实例：探活命中即复用，避免重复拉起。
 *
 * daemon 二进制查找顺序：TANGOFORGE_DAEMON 环境变量 → 打包资源 bin/ → 仓库根 bin/。
 */
const DAEMON_PORT = 19810
const PING_URL = `http://127.0.0.1:${DAEMON_PORT}/ping`
const POLL_INTERVAL_MS = 250
const POLL_TIMEOUT_MS = 5000

function daemonBinName(): string {
  return process.platform === 'win32' ? 'tangoforge-daemon.exe' : 'tangoforge-daemon'
}

async function isDaemonAlive(): Promise<boolean> {
  try {
    const res = await fetch(PING_URL, { signal: AbortSignal.timeout(800) })
    return res.ok
  } catch {
    return false
  }
}

function resolveDaemonPath(): string | null {
  if (process.env['TANGOFORGE_DAEMON']) return process.env['TANGOFORGE_DAEMON']
  const candidates: string[] = []
  if (app.isPackaged) {
    candidates.push(join(process.resourcesPath, 'bin', daemonBinName()))
  } else {
    // 开发期：仓库根 bin/（macOS 测试需 darwin 版二进制）
    const repoBin = join(__dirname, '..', '..', '..', '..', 'bin')
    candidates.push(join(repoBin, daemonBinName()))
  }
  for (const c of candidates) {
    if (existsSync(c)) return c
  }
  return null
}

const sleep = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms))

let spawned: ChildProcess | null = null

/** 探活 → 拉起 → 等待 Health Check；返回是否可用 */
export async function ensureDaemonRunning(): Promise<boolean> {
  if (await isDaemonAlive()) return true

  const bin = resolveDaemonPath()
  if (!bin) return false
  if (spawned && spawned.exitCode === null) {
    // 已在拉起中：等待首次探活成功
    const deadline = Date.now() + POLL_TIMEOUT_MS
    while (Date.now() < deadline) {
      await sleep(POLL_INTERVAL_MS)
      if (await isDaemonAlive()) return true
    }
    return false
  }

  spawned = spawn(bin, [], { detached: true, stdio: 'ignore' })
  spawned.unref()

  const deadline = Date.now() + POLL_TIMEOUT_MS
  while (Date.now() < deadline) {
    await sleep(POLL_INTERVAL_MS)
    if (await isDaemonAlive()) return true
  }
  return false
}

/** 读取全局配置 ui_token（~/.taskboard-app/config.yaml；daemon 首次启动时生成） */
export function readUiToken(): string {
  const cfgPath = join(homedir(), '.taskboard-app', 'config.yaml')
  try {
    const content = readFileSync(cfgPath, 'utf8')
    const m = content.match(/^ui_token:\s*(.+?)\s*$/m)
    return m ? m[1].trim() : ''
  } catch {
    return ''
  }
}

/** 系统目录选择器（项目导入）：取消返回 null */
async function selectDirectory(): Promise<string | null> {
  const win = BrowserWindow.getFocusedWindow() ?? undefined
  const result = await dialog.showOpenDialog(win as BrowserWindow, {
    title: '选择项目目录',
    properties: ['openDirectory', 'createDirectory'],
  })
  return result.canceled ? null : (result.filePaths[0] ?? null)
}

export function registerDaemonIpc(): void {
  ipcMain.handle('daemon:ensureRunning', () => ensureDaemonRunning())
}

export function registerConfigIpc(): void {
  ipcMain.handle('config:readUiToken', () => readUiToken())
}

export function registerDialogIpc(): void {
  ipcMain.handle('dialog:selectDirectory', () => selectDirectory())
}
