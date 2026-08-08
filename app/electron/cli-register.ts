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
  // bash_profile 注入的 PATH），但注册物（~/bin/tangoforge → 当前 CLI）存在即为
  // 已注册（新开终端生效）。
  if (!registered && process.platform !== 'win32') {
    const link = join(macBinDir(), 'tangoforge')
    try {
      if (existsSync(link) && realOf(link) === realOf(cli)) {
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

function macShellRc(): string {
  const home = homedir()
  if (existsSync(join(home, '.bash_profile'))) return join(home, '.bash_profile')
  if (existsSync(join(home, '.bashrc'))) return join(home, '.bashrc')
  return join(home, '.zshrc')
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

  // mac / linux：~/bin 链接 + profile PATH。
  const binDir = macBinDir()
  mkdirSync(binDir, { recursive: true })
  const link = join(binDir, 'tangoforge')
  try {
    rmSync(link, { force: true })
    symlinkSync(cli, link)
  } catch (err) {
    return `创建符号链接失败：${err instanceof Error ? err.message : err}`
  }
  const rc = macShellRc()
  const marker = `export PATH="${binDir}:$PATH"`
  if (existsSync(rc) && readFileSync(rc, 'utf8').includes(marker)) {
    return `已注册：${link}（PATH 已注入 ${rc}）`
  }
  writeFileSync(rc, `\n# TangoForge CLI\n${marker}\n`, { flag: 'a' })
  return `已注册：${link}（PATH 已注入 ${rc}，新开终端生效）`
}

function unregisterCli(): string {
  if (process.platform === 'win32') {
    const dir = resolveCliDir()
    const current = getUserPath()
    const entries = current.split(';').filter((e) => e.trim() !== '' && e.trim() !== dir)
    if (entries.join(';') === current) return '未在用户 PATH 中发现该 CLI 目录，无需卸载'
    setUserPath(entries.join(';'))
    return `已卸载注册：${dir}`
  }

  const link = join(macBinDir(), 'tangoforge')
  let removedLink = false
  try {
    if (existsSync(link) && realOf(link) === realOf(cliPath())) {
      rmSync(link, { force: true })
      removedLink = true
    }
  } catch {
    removedLink = false
  }
  // 移除 profile 中注入的行（幂等）。
  const rc = macShellRc()
  const marker = `export PATH="${macBinDir()}:$PATH"`
  try {
    if (existsSync(rc)) {
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
}
