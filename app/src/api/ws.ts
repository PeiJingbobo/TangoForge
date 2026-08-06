import { DAEMON_BASE_URL } from '@/api/client'
import type { WSEvent } from '@/types/models'

export type SocketStatus = 'connecting' | 'open' | 'closed'

export interface EventSocketOptions {
  /** 项目工作目录（WS 仅支持 ?project= 查询参数） */
  project: string
  onEvent: (event: WSEvent) => void
  onStatusChange?: (status: SocketStatus) => void
  /** 指数退避重连：初始 ms，默认 1000 */
  baseDelayMs?: number
  /** 最大重连间隔 ms，默认 30_000 */
  maxDelayMs?: number
}

/**
 * WS 事件客户端：GET /ws/events?project=
 * - 指数退避自动重连（连接被服务端关闭/网络抖动）；
 * - 多项目切换 = 先 disconnect 再新实例（本项目不做跨项目复用）；
 * - disconnect() 幂等，置终止标志停止重连。
 */
export class EventSocket {
  private readonly project: string
  private readonly onEvent: (e: WSEvent) => void
  private readonly onStatusChange?: (status: SocketStatus) => void
  private readonly baseDelayMs: number
  private readonly maxDelayMs: number

  private ws: WebSocket | null = null
  private retryCount = 0
  private retryTimer: ReturnType<typeof setTimeout> | null = null
  private terminated = false
  private status: SocketStatus = 'closed'

  constructor(opts: EventSocketOptions) {
    this.project = opts.project
    this.onEvent = opts.onEvent
    this.onStatusChange = opts.onStatusChange
    this.baseDelayMs = opts.baseDelayMs ?? 1000
    this.maxDelayMs = opts.maxDelayMs ?? 30_000
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
      // 移除 onclose 处理器避免关闭时触发重连
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
    const url = `${DAEMON_BASE_URL.replace(/^http/, 'ws')}/ws/events?project=${encodeURIComponent(this.project)}`
    this.setStatus('connecting')
    const ws = new WebSocket(url)
    this.ws = ws

    ws.onopen = () => {
      if (this.terminated) return
      this.retryCount = 0
      this.setStatus('open')
    }
    ws.onmessage = (ev: MessageEvent<string>) => {
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

/** 便捷函数：连接即订阅，返回断开函数（适合 React useEffect 直接使用） */
export function connectEvents(opts: EventSocketOptions): () => void {
  const sock = new EventSocket(opts)
  sock.connect()
  return () => sock.disconnect()
}
