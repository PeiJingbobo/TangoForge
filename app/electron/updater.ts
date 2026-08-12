import { app, BrowserWindow, ipcMain, Notification, shell } from 'electron'
import { readFileSync, writeFileSync } from 'fs'
import { join } from 'path'
import type { UpdatePayload, UpdateStatus } from '../src/types/update'

// electron-updater 为 CommonJS 包，而主进程产物为 ESM（package.json "type": "module"）：
// `import { autoUpdater } from 'electron-updater'` 命名导入会被 Node ESM-CJS 互操作拒绝
// （Named export 'autoUpdater' not found），必须在 ESM 里用默认导入再解构（TF-036 踩坑）。
import electronUpdater from 'electron-updater'

const { autoUpdater } = electronUpdater

/**
 * 在线更新（TF-036，docs/CI-CD-UPDATER.md 评审版）：
 * 平台策略由发布配置决定（未签名阶段）：
 * - Windows：electron-updater 全链路（检测 → 下载 → 重启安装），元数据 latest.yml；
 * - macOS：未签名无法自动安装，改为「检测新版本 → 自动打开 dmg 下载页 → 用户手动安装」，
 *   元数据来自 GitHub Releases API（latest 标签 + dmg 资产地址）。
 * - 开发模式（未打包）不启用，getStatus().supported 供渲染层提示。
 */
const UPDATE_REPO = 'PeiJingbobo/TangoForge'
const STATE_CHANNEL = 'update:state'
const AUTO_CHECK_DELAY_MS = 10_000
const API_TIMEOUT_MS = 15_000

const isMac = process.platform === 'darwin'
const isWin = process.platform === 'win32'

let current: UpdatePayload = { state: 'idle' }
let macDmgUrl: string | null = null
let busy = false

function broadcast(payload: UpdatePayload): void {
  current = payload
  for (const win of BrowserWindow.getAllWindows()) {
    if (!win.isDestroyed()) win.webContents.send(STATE_CHANNEL, payload)
  }
}

function notify(title: string, body: string): void {
  if (Notification.isSupported()) {
    new Notification({ title, body }).show()
  }
}

/* ---------- semver（electron-updater 内部同规则，mac 手动比对用） ---------- */

function versionParts(v: string): number[] {
  return v
    .replace(/^v/i, '')
    .split('.')
    .map((s) => Number.parseInt(s, 10) || 0)
}

function isNewerVersion(a: string, b: string): boolean {
  const x = versionParts(a)
  const y = versionParts(b)
  const len = Math.max(x.length, y.length)
  for (let i = 0; i < len; i++) {
    const diff = (x[i] ?? 0) - (y[i] ?? 0)
    if (diff !== 0) return diff > 0
  }
  return false
}

/* ---------- 自动打开 dmg 的防重复标记（每个版本只自动打开一次） ---------- */

function notifiedFile(): string {
  return join(app.getPath('userData'), 'update-notified.json')
}

function readNotifiedVersion(): string {
  try {
    const raw = JSON.parse(readFileSync(notifiedFile(), 'utf8')) as { version?: unknown }
    return typeof raw.version === 'string' ? raw.version : ''
  } catch {
    return ''
  }
}

function saveNotifiedVersion(version: string): void {
  try {
    writeFileSync(notifiedFile(), JSON.stringify({ version }))
  } catch {
    // 持久化失败不影响更新流程
  }
}

/* ---------- macOS（未签名阶段：打开 dmg 手动安装） ---------- */

async function checkMacRelease(auto: boolean): Promise<void> {
  broadcast({ state: 'checking' })
  try {
    const res = await fetch(`https://api.github.com/repos/${UPDATE_REPO}/releases/latest`, {
      headers: { Accept: 'application/vnd.github+json', 'User-Agent': 'TangoForge' },
      signal: AbortSignal.timeout(API_TIMEOUT_MS),
    })
    if (!res.ok) throw new Error(`GitHub Releases 查询失败（HTTP ${res.status}）`)
    const rel = (await res.json()) as {
      tag_name?: string
      body?: string
      assets?: Array<{ name?: string; browser_download_url?: string }>
    }
    const tag = rel.tag_name ?? ''
    const version = tag.replace(/^v/i, '')
    if (!version || !isNewerVersion(version, app.getVersion())) {
      broadcast({ state: 'not-available' })
      return
    }
    const dmg = rel.assets?.find((a) => /\.dmg$/i.test(a.name ?? ''))?.browser_download_url
    macDmgUrl = dmg ?? null
    broadcast({
      state: 'available',
      version,
      releaseNotes: rel.body,
      downloadUrl: macDmgUrl ?? undefined,
    })
    notify(`发现新版本 TangoForge v${version}`, '已自动打开下载页，请手动安装新版本。')
    // 启动自动检查：每个版本仅自动打开一次（持久化防重复打扰）。
    if (auto && macDmgUrl && readNotifiedVersion() !== version) {
      saveNotifiedVersion(version)
      void openMacDownload()
    }
  } catch (err) {
    broadcast({ state: 'error', error: err instanceof Error ? err.message : String(err) })
  }
}

async function openMacDownload(): Promise<boolean> {
  if (!macDmgUrl) return false
  try {
    await shell.openExternal(macDmgUrl)
    return true
  } catch {
    return false
  }
}

/* ---------- Windows（electron-updater 全链路） ---------- */

function releaseNotesOf(info: { releaseNotes?: unknown }): string | undefined {
  const notes = info.releaseNotes
  if (typeof notes === 'string') return notes
  if (Array.isArray(notes)) {
    return notes
      .map((x) =>
        typeof x === 'string'
          ? x
          : x && typeof x === 'object' && 'note' in x
            ? String((x as { note: unknown }).note)
            : String(x),
      )
      .join('\n')
  }
  return undefined
}

let winUpdaterReady = false

function ensureWinUpdater(): void {
  if (winUpdaterReady) return
  winUpdaterReady = true
  autoUpdater.autoDownload = false
  autoUpdater.autoInstallOnAppQuit = true
  // 自签名阶段（TF-036）：electron-updater 默认的 Windows 签名校验用 Get-AuthenticodeSignature
  // 要求证书链受信任（Status==Valid），自签名证书（无受信任根）在终端机必然失败，导致
  // "not signed by the application owner"。通过公开钩子覆盖为跳过该校验：
  // 安装包完整性已由 latest.yml 的 sha512 + GitHub HTTPS 传输保证。
  // ⚠️ Phase 2 换上正式代码签名证书后必须移除本覆盖，恢复默认严格校验。
  const nsis = autoUpdater as unknown as {
    verifyUpdateCodeSignature: (publisherNames: string[], path: string) => Promise<string | null>
  }
  nsis.verifyUpdateCodeSignature = async (): Promise<string | null> => null
  autoUpdater.on('checking-for-update', () => broadcast({ state: 'checking' }))
  autoUpdater.on('update-available', (info) => {
    broadcast({ state: 'available', version: info.version, releaseNotes: releaseNotesOf(info) })
    notify(`发现新版本 TangoForge v${info.version}`, '可在「设置 → 关于」中下载更新。')
  })
  autoUpdater.on('update-not-available', () => broadcast({ state: 'not-available' }))
  autoUpdater.on('download-progress', (p) =>
    broadcast({ state: 'downloading', percent: Math.round(p.percent) }),
  )
  autoUpdater.on('update-downloaded', (info) => {
    broadcast({ state: 'downloaded', version: info.version })
    notify('新版本已下载', '在「设置 → 关于」中点击「重启并安装」完成更新。')
  })
  autoUpdater.on('error', (err) => {
    const msg = err instanceof Error ? err.message : String(err)
    // 尚无任何发布（latest.yml 不存在）视为“已是最新”。
    if (/latest\.yml/i.test(msg) || msg.includes('404')) {
      broadcast({ state: 'not-available' })
    } else {
      broadcast({ state: 'error', error: msg })
    }
  })
}

async function checkWinRelease(): Promise<boolean> {
  ensureWinUpdater()
  try {
    await autoUpdater.checkForUpdates()
    return true
  } catch (err) {
    broadcast({ state: 'error', error: err instanceof Error ? err.message : String(err) })
    return false
  }
}

/* ---------- 对外入口（IPC） ---------- */

/** 检查更新；auto=true 表示启动后台自动检查（mac 下自动打开 dmg） */
export async function checkForUpdates(options?: { auto?: boolean }): Promise<boolean> {
  if (!app.isPackaged) {
    broadcast({ state: 'error', error: '在线更新仅安装版可用（开发模式已禁用）' })
    return false
  }
  if (busy) return false
  busy = true
  try {
    if (isMac) {
      await checkMacRelease(!!options?.auto)
      return true
    }
    if (isWin) return await checkWinRelease()
    broadcast({ state: 'error', error: '当前平台暂不支持在线更新' })
    return false
  } finally {
    busy = false
  }
}

async function downloadUpdate(): Promise<boolean> {
  if (!app.isPackaged || !isWin) return false
  ensureWinUpdater()
  try {
    await autoUpdater.downloadUpdate()
    return true
  } catch (err) {
    broadcast({ state: 'error', error: err instanceof Error ? err.message : String(err) })
    return false
  }
}

function installUpdate(): boolean {
  if (!app.isPackaged || !isWin) return false
  ensureWinUpdater()
  autoUpdater.quitAndInstall()
  return true
}

export function getStatus(): UpdateStatus {
  return {
    ...current,
    currentVersion: app.getVersion(),
    supported: app.isPackaged && (isMac || isWin),
  }
}

export function registerUpdaterIpc(): void {
  ipcMain.handle('update:check', () => checkForUpdates())
  ipcMain.handle('update:download', () => downloadUpdate())
  ipcMain.handle('update:install', () => installUpdate())
  ipcMain.handle('update:openDownload', () => openMacDownload())
  ipcMain.handle('update:getState', () => getStatus())
}

/** App 就绪后延迟后台检查一次（仅安装版）；mac 下发现新版本自动打开 dmg。 */
export function scheduleAutoCheck(): void {
  if (!app.isPackaged || !(isMac || isWin)) return
  setTimeout(() => {
    void checkForUpdates({ auto: true })
  }, AUTO_CHECK_DELAY_MS)
}
