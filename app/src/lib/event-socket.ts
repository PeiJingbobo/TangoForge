import type { WSEvent } from '../types/models'

export type SocketStatus = 'connecting' | 'open' | 'closed'

/**
 * 最小 WebSocket 形态（兼容浏览器 DOM 与 Node undici/ws 两套全局类型）：
 * 主进程（tsconfig.node 无 DOM lib）与渲染进程（DOM lib）均可编译。
 * Node 环境（Electron 主进程）无全局 WebSocket，由 createSocket 注入 ws 实现。
 */
export interface RawSocket {
  onopen: (() => void) | null
  onmessage: ((ev: { data: string }) => void) | null
  onclose: (() => void) | null
  onerror: (() => void) | null
  close: () => void
}

export interface EventSocketOptions {
  /** 完整 WS 地址（含项目查询参数），由调用方组装 */
  url: string
  onEvent: (event: WSEvent) => void
  onStatusChange?: (status: SocketStatus) => void
  /** 指数退避重连：初始 ms，默认 1000 */
  baseDelayMs?: number
  /** 最大重连间隔 ms，默认 30_000 */
  maxDelayMs?: number
  /**
   * Socket 工厂（可选）：默认使用全局 WebSocket（浏览器/测试环境）。
   * Electron 主进程（Node 20 无全局 WebSocket）必须注入基于 ws 包的实现。
   */
  createSocket?: (url: string) => RawSocket
}

/**
 * WS 事件客户端（Electron 主进程持有，docs/TECHNICAL.md §4.4）：
 * - 指数退避自动重连（连接被服务端关闭/网络抖动）；
 * - 项目切换 = 先 disconnect 再新实例（调用方负责，本项目不做跨项目复用）；
 * - disconnect() 幂等，置终止标志停止重连。
 * 纯逻辑（无 electron/Node 依赖），可单测（FakeWebSocket）。
 */
export class EventSocket {
  private readonly url: string
  private readonly onEvent: (e: WSEvent) => void
  private readonly onStatusChange?: (status: SocketStatus) => void
  private readonly baseDelayMs: number
  private readonly maxDelayMs: number
  private readonly createSocket: (url: string) => RawSocket

  private ws: RawSocket | null = null
  private retryCount = 0
  private retryTimer: ReturnType<typeof setTimeout> | null = null
  private terminated = false
  private status: SocketStatus = 'closed'

  constructor(opts: EventSocketOptions) {
    this.url = opts.url
    this.onEvent = opts.onEvent
    this.onStatusChange = opts.onStatusChange
    this.baseDelayMs = opts.baseDelayMs ?? 1000
    this.maxDelayMs = opts.maxDelayMs ?? 30_000
    this.createSocket = opts.createSocket ?? ((u) => new WebSocket(u) as unknown as RawSocket)
  }

  connect(): void {
    this.terminated = false
    this.open()
  }

  disconnect(): void {
    this.terminated = true
    if (this.retryTimer) clearTimeout(this.retryTimer)
    this.retryTimer = null
    if (this.ws) {
      this.ws.onclose = null
      this.ws.close()
      this.ws = null
    }
    this.setStatus('closed')
  }

  get currentStatus(): SocketStatus {
    return this.status
  }

  private open(): void {
    if (this.terminated) return
    this.setStatus('connecting')
    const ws = this.createSocket(this.url)
    this.ws = ws

    ws.onopen = () => {
      if (this.terminated) return
      this.retryCount = 0
      this.setStatus('open')
    }
    ws.onmessage = (ev) => {
      if (this.terminated) return
      try {
        const event = JSON.parse(ev.data) as WSEvent
        this.onEvent(event)
      } catch {
        // 忽略无法解析的消息
      }
    }
    ws.onclose = () => {
      if (this.terminated) return
      this.ws = null
      this.setStatus('closed')
      this.scheduleReconnect()
    }
    ws.onerror = () => {
      // onclose 随后触发，统一走重连逻辑
    }
  }

  private scheduleReconnect(): void {
    if (this.terminated || this.retryTimer) return
    const delay = Math.min(this.baseDelayMs * 2 ** this.retryCount, this.maxDelayMs)
    this.retryCount += 1
    this.retryTimer = setTimeout(() => {
      this.retryTimer = null
      this.open()
    }, delay)
  }

  private setStatus(s: SocketStatus): void {
    if (this.status !== s) {
      this.status = s
      this.onStatusChange?.(s)
    }
  }
}
