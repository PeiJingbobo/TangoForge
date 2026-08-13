import { app, BrowserWindow, dialog, ipcMain } from 'electron'
import { spawn, type ChildProcess } from 'child_process'
import { existsSync, readFileSync } from 'fs'
import { homedir } from 'os'
import { join } from 'path'
import WS from 'ws'
import { EventSocket, type RawSocket } from '../src/lib/event-socket'
import { shouldRestartDaemon } from '../src/lib/daemon-version'
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
    // dev：__dirname = app/out/main → 向上 3 级到仓库根（out/main → out → app → 仓库根）。
    const repoBin = join(__dirname, '..', '..', '..', 'bin')
    candidates.push(join(repoBin, daemonBinName()))
  }
  for (const c of candidates) {
    if (existsSync(c)) return c
  }
  return null
}

const sleep = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms))

let spawned: ChildProcess | null = null

/* ---------- 版本探测 + 空闲重启（TF-053） ---------- */

/** daemon 版本探测（GET /api/daemon/version，免鉴权）；失败返回 null */
async function fetchDaemonVersion(): Promise<string | null> {
  try {
    const res = await fetch(`${DAEMON_BASE_URL}/api/daemon/version`, {
      signal: AbortSignal.timeout(800),
    })
    if (!res.ok) return null
    const body = (await res.json()) as { data?: { version?: string } }
    return body.data?.version ?? null
  } catch {
    return null
  }
}

/** 请求 daemon 空闲重启（POST /api/daemon/restart，UI 身份）；返回是否已接受 */
async function requestDaemonRestart(binPath: string): Promise<boolean> {
  try {
    const res = await fetch(`${DAEMON_BASE_URL}/api/daemon/restart`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ bin_path: binPath }),
      signal: AbortSignal.timeout(2000),
    })
    return res.status === 202
  } catch {
    return false
  }
}

/** 等待 daemon 完成重启并重新可用（旧进程退出 → 新进程就绪），超时返回 false */
async function waitForDaemonRestart(timeoutMs: number): Promise<boolean> {
  const deadline = Date.now() + timeoutMs
  // 阶段 1：等待旧进程退出（ping 失败）。
  while (Date.now() < deadline) {
    if (!(await isDaemonAlive())) break
    await sleep(200)
  }
  // 阶段 2：等待新进程就绪（ping 成功 + 版本匹配）。
  while (Date.now() < deadline) {
    if (await isDaemonAlive()) return true
    await sleep(200)
  }
  return false
}

/**
 * 版本匹配检测：daemon 版本与 APP 版本不一致 → 请求空闲重启。
 * 仅在**打包版**启用（dev 下 daemon 是 `make build` 的本地产物，版本 dev，
 * 与 APP package.json 版本不一致是常态，不触发重启）。
 * 返回 true = daemon 已就绪（无需重启 / 已重启完成）。
 */
async function ensureVersionMatch(): Promise<boolean> {
  if (!app.isPackaged) return true
  const required = app.getVersion()
  const running = await fetchDaemonVersion()
  if (!shouldRestartDaemon(required, running)) return true

  const bin = resolveDaemonPath()
  if (!bin) return false
  const ok = await requestDaemonRestart(bin)
  if (!ok) return false
  return waitForDaemonRestart(20_000)
}

/** 探活 → 拉起 → 等待 Health Check；返回是否可用 */
export async function ensureDaemonRunning(): Promise<boolean> {
  if (await isDaemonAlive()) {
    // 版本不匹配 → 空闲重启（新 daemon 自我重生后由本函数再次探活确认）。
    if (await ensureVersionMatch()) return true
    return isDaemonAlive()
  }

  const bin = resolveDaemonPath()
  if (!bin) return false
  if (spawned && spawned.exitCode === null) {
    const deadline = Date.now() + POLL_TIMEOUT_MS
    while (Date.now() < deadline) {
      await sleep(POLL_INTERVAL_MS)
      if (await isDaemonAlive()) {
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
  // UI 凭据实时读取（config.yaml 极小，读开销可忽略）：彻底消除初始化时序/缓存失效导致的
  // 全局端点 403（如 ensureRunning 未完成、多实例、daemon 热重载 token 变更）。
  const token = readUiToken()
  if (token) headers['X-UI-Token'] = token
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
    // Electron 主进程（Node 20）无全局 WebSocket，注入 ws 包实现。
    // ws 的 message 事件 data 为 Buffer，RawSocket 声明 string——运行时 Buffer 可隐式 toString。
    createSocket: (url) => new WS(url) as unknown as RawSocket,
  })
  eventSocket = sock
  sock.connect()
}

/** 系统目录选择器（默认打开路径 = 调用方传入的 defaultPath，如当前项目根目录）：取消返回 null */
async function selectDirectory(defaultPath?: string): Promise<string | null> {
  const win = BrowserWindow.getFocusedWindow() ?? undefined
  const result = await dialog.showOpenDialog(win as BrowserWindow, {
    title: '选择目录',
    properties: ['openDirectory', 'createDirectory'],
    defaultPath,
  })
  return result.canceled ? null : (result.filePaths[0] ?? null)
}

/** 系统文件选择器（Markdown 导入，多选；默认打开路径 = 调用方传入的 defaultPath）：取消返回 null */
async function selectFiles(defaultPath?: string): Promise<string[] | null> {
  const win = BrowserWindow.getFocusedWindow() ?? undefined
  const result = await dialog.showOpenDialog(win as BrowserWindow, {
    title: '选择文件',
    properties: ['openFile', 'multiSelections'],
    defaultPath,
    filters: [
      { name: 'Markdown', extensions: ['md', 'markdown'] },
      { name: '全部文件', extensions: ['*'] },
    ],
  })
  return result.canceled ? null : result.filePaths
}

/**
 * 在系统文件管理器中显示项目目录（TF-035 右键菜单「在文件夹中打开」）。
 * 目录存在 → revealItemInFolder（选中该目录）；不存在 → openPath（打开上级，兜底）。
 */
async function revealPath(path: string): Promise<boolean> {
  const { shell } = await import('electron')
  try {
    if (existsSync(path)) {
      shell.showItemInFolder(path)
    } else {
      await shell.openPath(join(path, '..'))
    }
    return true
  } catch {
    return false
  }
}

/** 用系统默认应用打开文件/目录（TF-039 导出记录「打开文件」）；失败返回 false。 */
async function openPath(path: string): Promise<boolean> {
  const { shell } = await import('electron')
  try {
    const err = await shell.openPath(path)
    return err === ''
  } catch {
    return false
  }
}

export function registerDaemonIpc(): void {
  ipcMain.handle('daemon:ensureRunning', () => ensureDaemonRunning())
  ipcMain.handle('daemon:status', () => isDaemonAlive())
  ipcMain.handle('daemon:apiRequest', (_e, req: ApiRequestPayload) => apiProxy(req))
  ipcMain.handle('daemon:events:setProject', (_e, project: string | null) => {
    setWsProject(project)
    return true
  })
  ipcMain.handle('dialog:selectDirectory', (_e, defaultPath?: string) =>
    selectDirectory(defaultPath),
  )
  ipcMain.handle('dialog:selectFiles', (_e, defaultPath?: string) => selectFiles(defaultPath))
  ipcMain.handle('shell:revealPath', (_e, path: string) => revealPath(path))
  ipcMain.handle('shell:openPath', (_e, path: string) => openPath(path))
}

export function registerConfigIpc(): void {
  // 保留通道（兼容 web 调试）：token 读取由主进程 apiProxy 内部完成。
  ipcMain.handle('config:readUiToken', () => readUiToken())
}
