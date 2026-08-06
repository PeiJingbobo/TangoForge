import { app, BrowserWindow, dialog, ipcMain } from 'electron'
import { spawn, type ChildProcess } from 'child_process'
import { existsSync, readFileSync } from 'fs'
import { homedir } from 'os'
import { join } from 'path'
import { EventSocket } from '../src/lib/event-socket'
import type { WSEvent } from '../src/types/models'

/**
 * 内嵌守护进程管理 + 数据层（Electron 最佳实践：渲染进程只做 UI，
 * HTTP/WS/IO 全部经 IPC 委托主进程完成，异步非阻塞）：
 *
 * 1. 守护进程：探活 127.0.0.1:19810（GET /ping）→ spawn（detached，退出不关）→ 轮询 ≤5s；
 * 2. API 代理：`daemon:apiRequest` — 主进程 fetch daemon，注入 X-UI-Token（token 不暴露渲染进程），
 *    path 白名单（/ping、/api/*），错误透传业务码；
 * 3. WS 事件：主进程持有 EventSocket（指数退避重连），事件 webContents.send 广播给所有窗口；
 *    `daemon:events:setProject` 切换项目（断开重建）。
 */
const DAEMON_PORT = 19810
const DAEMON_BASE_URL = `http://127.0.0.1:${DAEMON_PORT}`
const PING_URL = `${DAEMON_BASE_URL}/ping`
const POLL_INTERVAL_MS = 250
const POLL_TIMEOUT_MS = 5000
/** 渲染进程可请求的 path 白名单（仅 daemon 端点，防任意 URL 代理） */
const ALLOWED_PATH = /^\/(ping|api\/)/

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
  if (await isDaemonAlive()) {
    refreshUiToken()
    return true
  }

  const bin = resolveDaemonPath()
  if (!bin) return false
  if (spawned && spawned.exitCode === null) {
    const deadline = Date.now() + POLL_TIMEOUT_MS
    while (Date.now() < deadline) {
      await sleep(POLL_INTERVAL_MS)
      if (await isDaemonAlive()) {
        refreshUiToken()
        return true
      }
    }
    return false
  }

  spawned = spawn(bin, [], { detached: true, stdio: 'ignore' })
  spawned.unref()

  const deadline = Date.now() + POLL_TIMEOUT_MS
  while (Date.now() < deadline) {
    await sleep(POLL_INTERVAL_MS)
    if (await isDaemonAlive()) {
      refreshUiToken()
      return true
    }
  }
  return false
}

/* ---------- UI 凭据（主进程持有，不暴露渲染进程） ---------- */

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

let uiToken = ''
function refreshUiToken(): void {
  uiToken = readUiToken()
}

/* ---------- API 代理（渲染进程 → IPC → 主进程 fetch daemon） ---------- */

interface ApiRequestPayload {
  method?: string
  path: string
  body?: unknown
  project?: string
}

interface ApiProxyResult {
  ok: boolean
  status: number
  /** 完整响应体（统一信封或原始文本） */
  body: unknown
}

async function apiProxy(req: ApiRequestPayload): Promise<ApiProxyResult> {
  const method = (req.method ?? 'GET').toUpperCase()
  if (!ALLOWED_PATH.test(req.path)) {
    return { ok: false, status: 403, body: { code: 'FORBIDDEN', message: 'path 不在白名单' } }
  }
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (req.project) headers['X-Project'] = req.project
  if (uiToken) headers['X-UI-Token'] = uiToken
  try {
    const res = await fetch(`${DAEMON_BASE_URL}${req.path}`, {
      method,
      headers,
      body: req.body === undefined ? undefined : JSON.stringify(req.body),
      signal: AbortSignal.timeout(30_000),
    })
    const text = await res.text()
    let body: unknown = text
    try {
      body = JSON.parse(text)
    } catch {
      // 非 JSON（如导出内容）
    }
    return { ok: res.ok, status: res.status, body }
  } catch {
    return { ok: false, status: 0, body: { code: 'NETWORK_ERROR', message: '无法连接守护进程' } }
  }
}

/* ---------- WS 事件（主进程持有，事件广播给所有窗口） ---------- */

let eventSocket: EventSocket | null = null

function broadcast(event: WSEvent): void {
  for (const win of BrowserWindow.getAllWindows()) {
    if (!win.isDestroyed()) win.webContents.send('daemon:event', event)
  }
}

function setWsProject(project: string | null): void {
  eventSocket?.disconnect()
  eventSocket = null
  if (!project) return
  const sock = new EventSocket({
    url: `ws://127.0.0.1:${DAEMON_PORT}/ws/events?project=${encodeURIComponent(project)}`,
    onEvent: broadcast,
  })
  eventSocket = sock
  sock.connect()
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
  ipcMain.handle('daemon:status', () => isDaemonAlive())
  ipcMain.handle('daemon:apiRequest', (_e, req: ApiRequestPayload) => apiProxy(req))
  ipcMain.handle('daemon:events:setProject', (_e, project: string | null) => {
    setWsProject(project)
    return true
  })
  ipcMain.handle('dialog:selectDirectory', () => selectDirectory())
}

export function registerConfigIpc(): void {
  // 保留通道（兼容 web 调试）：token 读取由主进程 apiProxy 内部完成。
  ipcMain.handle('config:readUiToken', () => readUiToken())
}
