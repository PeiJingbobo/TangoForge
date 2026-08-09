import { app, BrowserWindow, clipboard, ipcMain, shell } from 'electron'
import { join } from 'path'
import { registerCliIpc } from './cli-register'
import { registerConfigIpc, registerDaemonIpc } from './daemon'

// 单实例锁（Electron 官方最佳实践）：防止多窗口/多实例并存时
// 旧主进程响应 IPC（token 缓存失效、WS 连接错乱等）。
if (!app.requestSingleInstanceLock()) {
  app.quit()
}

// 安全基线（docs/TECHNICAL.md §4.4）：
// contextIsolation: true、nodeIntegration: false、渲染进程 sandbox；
// 渲染进程只能通过 preload 暴露的白名单 IPC API 与主进程通信。
function createWindow(): void {
  const isMac = process.platform === 'darwin'
  const isWin = process.platform === 'win32'

  const win = new BrowserWindow({
    width: 1280,
    height: 800,
    show: false,
    // 无边框窗口（TF-038 自定义标题栏）：
    // - macOS：hiddenInset 隐藏标题栏但保留左上角原生红黄绿控制按钮；
    // - Windows：frame:false 无系统标题栏与按钮，由渲染进程自绘控制按钮；
    // - 其他平台（Linux/dev 预览）：保留系统标题栏。
    ...(isMac ? { titleBarStyle: 'hiddenInset' as const } : isWin ? { frame: false as const } : {}),
    webPreferences: {
      // preload 为 CJS（sandbox 兼容；见 electron.vite.config.ts output.format=cjs）
      preload: join(__dirname, '../preload/index.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  })

  win.on('ready-to-show', () => win.show())

  // 调试模式（scripts/dev-run.sh debug / TF_DEBUG=1）：打开渲染进程 DevTools，
  // 便于查看 UI 层（React/拖拽等）运行时错误与白屏原因。
  if (process.env.ELECTRON_DEBUG === '1') {
    win.webContents.openDevTools({ mode: 'detach' })
  }

  win.webContents.setWindowOpenHandler(({ url }) => {
    void shell.openExternal(url)
    return { action: 'deny' }
  })

  // 最大化状态变化 → 通知渲染进程（Windows 自绘按钮图标切换）。
  win.on('maximize', () => {
    win.webContents.send('window:maximized-change', true)
  })
  win.on('unmaximize', () => {
    win.webContents.send('window:maximized-change', false)
  })

  // 开发模式加载 electron-vite dev server；生产模式加载打包产物。
  if (!app.isPackaged && process.env['ELECTRON_RENDERER_URL']) {
    void win.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    void win.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

// 窗口控制 IPC（TF-038 自绘标题栏按钮：最小化 / 最大化切换 / 关闭）。
function registerWindowIpc(): void {
  const winOf = (e: Electron.IpcMainInvokeEvent): BrowserWindow | null =>
    BrowserWindow.fromWebContents(e.sender)

  ipcMain.handle('window:minimize', (e) => {
    winOf(e)?.minimize()
  })
  ipcMain.handle('window:toggleMaximize', (e) => {
    const win = winOf(e)
    if (!win) return
    if (win.isMaximized()) {
      win.unmaximize()
    } else {
      win.maximize()
    }
  })
  ipcMain.handle('window:close', (e) => {
    winOf(e)?.close()
  })
  ipcMain.handle('window:isMaximized', (e) => {
    return winOf(e)?.isMaximized() ?? false
  })
}

// 第二实例触发（点击 Dock 图标）时聚焦已有窗口而非新建。
app.on('second-instance', () => {
  const win = BrowserWindow.getAllWindows()[0]
  if (win) {
    if (win.isMinimized()) win.restore()
    win.focus()
  }
})

app.whenReady().then(() => {
  registerDaemonIpc()
  registerConfigIpc()
  registerCliIpc()
  // 剪贴板写（QA 2026-08-09：任务编号复制；file:// 下 navigator.clipboard 不可靠）
  ipcMain.handle('clipboard:writeText', (_e, text: string) => {
    clipboard.writeText(String(text ?? ''))
    return true
  })
  registerWindowIpc()

  // App 启动时探活/拉起内嵌守护进程（docs/TECHNICAL.md §4.4）；不阻塞窗口创建。
  void import('./daemon').then(({ ensureDaemonRunning }) => ensureDaemonRunning())

  createWindow()

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
  })
})

app.on('window-all-closed', () => {
  // 全部可见窗口关闭 → 完全退出 UI 进程（各平台一致，含 macOS；QA 2026-08-08）。
  // 守护进程（tangoforge-daemon）独立 detached 常驻，退出 UI 不关闭
  // （docs/REQUIREMENTS.md N6；需要时用 scripts/stop-daemon 手动停止）。
  app.quit()
})
