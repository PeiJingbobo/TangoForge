import { app, ipcMain } from 'electron'
import { execFileSync } from 'child_process'
import {
  existsSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from 'fs'
import { homedir } from 'os'
import { join, resolve } from 'path'

/**
 * CLI 全局注册管理（全局设置页「CLI 板块」数据层）：
 *
 * - cli:status      —— 注册状态检测：`where tangoforge`（Win）/ `command -v tangoforge`（mac）
 *                     探测 PATH 可用性，并判断是否指向当前 App 分发的 CLI；
 * - cli:register    —— 注册：Win 追加 CLI 目录到用户 PATH；mac 创建 ~/bin/tangoforge
 *                     符号链接 + 注入 shell profile PATH（幂等）；
 * - cli:unregister  —— 卸载：Win 从用户 PATH 移除该目录；mac 删链接 + 移除 profile 行。
 *
 * CLI 目录：打包版 = resources/bin（extraResources 打入）；dev = 仓库根 bin/。
 */

/** 当前 App 分发 CLI 的目录（打包版 resources/bin；dev 仓库根 bin/） */
export function resolveCliDir(): string {
  if (app.isPackaged) {
    return join(process.resourcesPath, 'bin')
  }
  // dev：__dirname = app/out/main → 向上 3 级到仓库根（out/main → out → app → 仓库根）。
  return resolve(__dirname, '..', '..', '..', 'bin')
}

function cliName(): string {
  return process.platform === 'win32' ? 'tangoforge.exe' : 'tangoforge'
}

function cliPath(): string {
  return join(resolveCliDir(), cliName())
}

export interface CliStatus {
  /** CLI 是否已全局可用（PATH 可解析） */
  registered: boolean
  /** 解析到的 tangoforge 路径（未注册为 null） */
  path: string | null
  /** 是否指向当前 App 分发的 CLI（帮助判断注册的是否是"我们"） */
  ours: boolean
  /** 当前 App CLI 的实际路径（供 UI 展示） */
  cliPath: string
}

/** 探测 tangoforge 是否在 PATH 中（Win: where；mac/Linux: command -v）。 */
function findInPath(): string | null {
  try {
    if (process.platform === 'win32') {
      const out = execFileSync('where', ['tangoforge'], { encoding: 'utf8', timeout: 5000 })
      const line = out.split(/\r?\n/).find((l) => l.trim().length > 0)
      return line ? line.trim() : null
    }
    const out = execFileSync('/bin/sh', ['-c', 'command -v tangoforge || true'], {
      encoding: 'utf8',
      timeout: 5000,
    })
    const line = out.trim()
    return line ? line : null
  } catch {
    return null
  }
}

/** 解析符号链接得到真实路径（mac ~/bin/tangoforge → resources/bin/tangoforge）。 */
function realOf(p: string): string {
  try {
    return realpathSync(p)
  } catch {
    return p
  }
}

export function cliStatus(): CliStatus {
  const found = findInPath()
  const cli = cliPath()
  let registered = found !== null
  let path = found
  // mac/Linux 补充通道：非登录 shell 不加载 shell profile（command -v 探测不到
  // profile 注入的 PATH），但注册物（~/bin/tangoforge）存在即视为已注册——
  // 不论其指向当前 App 还是旧版本/dev 版（ours 单独判定，UI 据此提示更新）。
  if (!registered && process.platform !== 'win32') {
    const link = join(macBinDir(), 'tangoforge')
    try {
      if (existsSync(link)) {
        registered = true
        path = link
      }
    } catch {
      // 链接损坏等 → 视为未注册
    }
  }
  return {
    registered,
    path,
    ours: registered && path !== null && realOf(path) === realOf(cli),
    cliPath: cli,
  }
}

/**
 * 更新注册到当前版本：检测到 tangoforge 已注册但指向其他位置（旧版本/dev 版）时，
 * 由用户确认后把当前 App 的 CLI 更新为 PATH 解析目标（并保持优先）。
 */
function updateCli(): string {
  const cli = cliPath()
  if (!existsSync(cli)) return `未找到 CLI：${cli}（请确认 App 安装完整）`

  if (process.platform === 'win32') {
    const dir = resolveCliDir()
    // 移除解析到的旧目录（status.path 所在目录），再把当前目录置顶。
    const oldPath = findInPath()
    const current = getUserPath()
    const entries = current.split(';').filter((e) => {
      const t = e.trim()
      return t !== '' && t !== dir && (oldPath === null || t !== dirnameOf(oldPath))
    })
    entries.unshift(dir)
    setUserPath(entries.join(';'))
    return `已更新：${dir}（已置顶；新开终端生效）`
  }

  // mac / linux：重建链接指向当前 CLI + 确保全部 profile 注入。
  const binDir = macBinDir()
  mkdirSync(binDir, { recursive: true })
  const link = join(binDir, 'tangoforge')
  try {
    rmSync(link, { force: true })
    symlinkSync(cli, link)
  } catch (err) {
    return `更新符号链接失败：${err instanceof Error ? err.message : err}`
  }
  const touched = injectPathIntoRcs(binDir)
  return `已更新：${link}（PATH 已注入 ${touched.join('、')}，新开终端生效）`
}

/** 取路径所在目录（Windows 反斜杠兼容）。 */
function dirnameOf(p: string): string {
  return p.includes('\\') ? p.slice(0, p.lastIndexOf('\\')) : p.slice(0, p.lastIndexOf('/'))
}

/* ---------- Windows：用户 PATH 追加/移除（PowerShell，避免 setx 1024 截断） ---------- */

function getUserPath(): string {
  try {
    return execFileSync(
      'powershell',
      ['-NoProfile', '-Command', "[Environment]::GetEnvironmentVariable('Path','User')"],
      { encoding: 'utf8', timeout: 8000 },
    ).trim()
  } catch {
    return ''
  }
}

function setUserPath(pathValue: string): void {
  execFileSync(
    'powershell',
    [
      '-NoProfile',
      '-Command',
      `[Environment]::SetEnvironmentVariable('Path','${pathValue}','User')`,
    ],
    { encoding: 'utf8', timeout: 8000 },
  )
}

/* ---------- macOS：~/bin 符号链接 + shell profile PATH ---------- */

/**
 * 需要注入 PATH 的 shell profile 列表（macOS 默认登录 shell 为 zsh）：
 * .zshrc 总是写入（zsh 不加载 bash_profile）；.bash_profile/.bashrc 存在时一并写入，
 * 保证无论用户使用 zsh 还是 bash 的新终端都能生效。
 */
function macShellRcs(): string[] {
  const home = homedir()
  const rcs: string[] = [join(home, '.zshrc')]
  for (const name of ['.bash_profile', '.bashrc']) {
    const p = join(home, name)
    if (existsSync(p) && !rcs.includes(p)) rcs.push(p)
  }
  return rcs
}

/** 向 profile 幂等注入 PATH marker；返回实际写入的 rc 列表。 */
function injectPathIntoRcs(binDir: string): string[] {
  const marker = `export PATH="${binDir}:$PATH"`
  const touched: string[] = []
  for (const rc of macShellRcs()) {
    const existing = existsSync(rc) ? readFileSync(rc, 'utf8') : ''
    if (existing.includes(marker)) {
      touched.push(rc)
      continue
    }
    writeFileSync(rc, `\n# TangoForge CLI\n${marker}\n`, { flag: 'a' })
    touched.push(rc)
  }
  return touched
}

function macBinDir(): string {
  return join(homedir(), 'bin')
}

/* ---------- 注册 / 卸载 ---------- */

function registerCli(): string {
  const cli = cliPath()
  if (!existsSync(cli)) return `未找到 CLI：${cli}（请确认 App 安装完整）`

  if (process.platform === 'win32') {
    const dir = resolveCliDir()
    const current = getUserPath()
    const entries = current.split(';').filter((e) => e.trim() !== '')
    if (entries.includes(dir)) return 'CLI 已注册（无需重复操作）'
    entries.push(dir)
    setUserPath(entries.join(';'))
    return `已注册：${dir}（新开终端生效）`
  }

  // mac / linux：~/bin 链接 + 全部 shell profile PATH。
  const binDir = macBinDir()
  mkdirSync(binDir, { recursive: true })
  const link = join(binDir, 'tangoforge')
  try {
    rmSync(link, { force: true })
    symlinkSync(cli, link)
  } catch (err) {
    return `创建符号链接失败：${err instanceof Error ? err.message : err}`
  }
  const touched = injectPathIntoRcs(binDir)
  return `已注册：${link}（PATH 已注入 ${touched.join('、')}，新开终端生效）`
}

function unregisterCli(): string {
  if (process.platform === 'win32') {
    // 移除已注册的 tangoforge 路径（不限于当前 CLI 目录）：当前目录 +
    // 实际解析到的目录（可能是旧版本/dev 版注册的）。
    const dir = resolveCliDir()
    const oldPath = findInPath()
    const oldDir = oldPath ? dirnameOf(oldPath) : null
    const removed: string[] = []
    const current = getUserPath()
    const entries = current.split(';').filter((e) => {
      const t = e.trim()
      if (t === '') return false
      if (t === dir || (oldDir !== null && t === oldDir)) {
        if (!removed.includes(t)) removed.push(t)
        return false
      }
      return true
    })
    if (entries.join(';') === current) return '未在用户 PATH 中发现 tangoforge 目录，无需卸载'
    setUserPath(entries.join(';'))
    return `已卸载注册：${removed.join('、')}`
  }

  // mac / linux：卸载应移除已注册的 tangoforge 链接（不论其指向当前 CLI 还是
  // 旧版本/dev 版注册物）+ 从所有 shell profile 移除注入行。
  const link = join(macBinDir(), 'tangoforge')
  let removedLink = false
  try {
    if (existsSync(link)) {
      rmSync(link, { force: true })
      removedLink = true
    }
  } catch {
    removedLink = false
  }
  // 从所有 shell profile 移除注入的 PATH 行（幂等）。
  const marker = `export PATH="${macBinDir()}:$PATH"`
  try {
    for (const rc of macShellRcs()) {
      if (!existsSync(rc)) continue
      const lines = readFileSync(rc, 'utf8')
        .split('\n')
        .filter((l) => !l.includes(marker))
      writeFileSync(rc, lines.join('\n'))
    }
  } catch {
    // 忽略 profile 写失败（链接已删即可）
  }
  return removedLink ? `已卸载注册：${link}` : '未发现已注册的 CLI 链接'
}

export function registerCliIpc(): void {
  ipcMain.handle('cli:status', (): CliStatus => cliStatus())
  ipcMain.handle('cli:register', (): { ok: boolean; message: string } => {
    try {
      return { ok: true, message: registerCli() }
    } catch (err) {
      return { ok: false, message: `注册失败：${err instanceof Error ? err.message : err}` }
    }
  })
  ipcMain.handle('cli:unregister', (): { ok: boolean; message: string } => {
    try {
      return { ok: true, message: unregisterCli() }
    } catch (err) {
      return { ok: false, message: `卸载失败：${err instanceof Error ? err.message : err}` }
    }
  })
  ipcMain.handle('cli:update', (): { ok: boolean; message: string } => {
    try {
      return { ok: true, message: updateCli() }
    } catch (err) {
      return { ok: false, message: `更新失败：${err instanceof Error ? err.message : err}` }
    }
  })
}
