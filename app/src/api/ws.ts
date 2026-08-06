import type { WSEvent } from '@/types/models'

/**
 * WS 事件订阅（Electron 最佳实践：连接由主进程持有，渲染进程只订阅）。
 * 经 preload 白名单 window.tangoforge.events：
 * - setProject 通知主进程切换 WS 连接（断开旧项目 → 连新项目）；
 * - on 订阅主进程推送的 daemon:event（返回取消订阅函数）。
 * Web / 测试环境（无 window.tangoforge）→ no-op（返回空清理函数）。
 */
export function subscribeEvents(project: string, onEvent: (event: WSEvent) => void): () => void {
  const events = window.tangoforge?.events
  if (!events) return () => {}
  void events.setProject(project)
  return events.on((raw) => {
    onEvent(raw as WSEvent)
  })
}
