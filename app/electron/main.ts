import { app, BrowserWindow, shell } from 'electron'
import { join } from 'path'
import { registerConfigIpc, registerDaemonIpc, registerDialogIpc } from './daemon'

// 安全基线（docs/TECHNICAL.md §4.4）：
// contextIsolation: true、nodeIntegration: false、渲染进程 sandbox；
// 渲染进程只能通过 preload 暴露的白名单 IPC API 与主进程通信。
function createWindow(): void {
  const win = new BrowserWindow({
    width: 1280,
    height: 800,
    show: false,
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  })

  win.on('ready-to-show', () => win.show())

  win.webContents.setWindowOpenHandler(({ url }) => {
    void shell.openExternal(url)
    return { action: 'deny' }
  })

  // 开发模式加载 electron-vite dev server；生产模式加载打包产物。
  if (!app.isPackaged && process.env['ELECTRON_RENDERER_URL']) {
    void win.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    void win.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

app.whenReady().then(() => {
  registerDaemonIpc()
  registerConfigIpc()
  registerDialogIpc()

  // App 启动时探活/拉起内嵌守护进程（docs/TECHNICAL.md §4.4）；不阻塞窗口创建。
  void import('./daemon').then(({ ensureDaemonRunning }) => ensureDaemonRunning())

  createWindow()

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
  })
})

app.on('window-all-closed', () => {
  // 守护进程常驻后台：App 退出不关闭守护进程（docs/REQUIREMENTS.md N6）。
  if (process.platform !== 'darwin') app.quit()
})
