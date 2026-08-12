/**
 * 窗口 Chrome 工具：自绘标题栏高度（TF-038）。
 * 桌面 Electron 无边框窗口（mac hiddenInset / win frame:false）顶部有 h-9（36px）
 * 自绘标题栏 + 拖拽区；全高右侧抽屉（Sheet 门户到 body）头部会被其遮挡，
 * 需按平台预留顶部内边距。Web 预览 / 其他平台为 0（不渲染标题栏）。
 */
export function getTitleBarHeight(): number {
  const platform = typeof window !== 'undefined' ? window.tangoforge?.window?.platform : undefined
  return platform === 'darwin' || platform === 'win32' ? 36 : 0
}
