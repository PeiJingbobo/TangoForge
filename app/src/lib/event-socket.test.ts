import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { EventSocket, type SocketStatus } from './event-socket'
import type { WSEvent } from '@/types/models'

/** 最小 Fake WebSocket（jsdom 无原生 WebSocket） */
class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  static closed: FakeWebSocket[] = []

  url: string
  readyState = 0
  onopen: (() => void) | null = null
  onmessage: ((ev: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null

  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }
  close(): void {
    this.readyState = 3
    FakeWebSocket.closed.push(this)
  }
  simulateOpen(): void {
    this.readyState = 1
    this.onopen?.()
  }
  simulateMessage(data: string): void {
    this.onmessage?.({ data })
  }
  simulateClose(): void {
    this.readyState = 3
    this.onclose?.()
  }
}

describe('EventSocket（主进程 WS 客户端，mock WebSocket）', () => {
  const onEvent = vi.fn()

  beforeEach(() => {
    vi.stubGlobal('WebSocket', FakeWebSocket)
    FakeWebSocket.instances = []
    FakeWebSocket.closed = []
    onEvent.mockReset()
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('连接：使用调用方传入的完整 URL，open 后状态 open', () => {
    const statuses: SocketStatus[] = []
    const sock = new EventSocket({
      url: 'ws://127.0.0.1:19810/ws/events?project=%2Fp1',
      onEvent,
      onStatusChange: (s) => statuses.push(s),
    })
    sock.connect()
    expect(FakeWebSocket.instances).toHaveLength(1)
    expect(FakeWebSocket.instances[0].url).toContain('/ws/events?project=%2Fp1')
    FakeWebSocket.instances[0].simulateOpen()
    expect(statuses).toEqual(['connecting', 'open'])
    sock.disconnect()
  })

  it('消息解析：合法事件回调，非法 JSON 忽略', () => {
    const sock = new EventSocket({ url: 'ws://x/ws/events', onEvent })
    sock.connect()
    const ws = FakeWebSocket.instances[0]
    ws.simulateMessage(
      JSON.stringify({
        type: 'task.created',
        project: '/p1',
        data: { id: 'x' },
        ts: '2026-08-06T00:00:00+08:00',
      }),
    )
    expect(onEvent).toHaveBeenCalledTimes(1)
    expect((onEvent.mock.calls[0][0] as WSEvent).type).toBe('task.created')
    ws.simulateMessage('not-json{')
    expect(onEvent).toHaveBeenCalledTimes(1)
    sock.disconnect()
  })

  it('断线重连：指数退避，首次 1s 后重连', () => {
    const sock = new EventSocket({ url: 'ws://x/ws/events', onEvent, baseDelayMs: 1000 })
    sock.connect()
    FakeWebSocket.instances[0].simulateClose()
    expect(FakeWebSocket.instances).toHaveLength(1) // 尚未重连
    vi.advanceTimersByTime(1000)
    expect(FakeWebSocket.instances).toHaveLength(2) // 已重连
    sock.disconnect()
  })

  it('重连退避递增：第二次 2s，封顶 maxDelay', () => {
    const sock = new EventSocket({
      url: 'ws://x/ws/events',
      onEvent,
      baseDelayMs: 1000,
      maxDelayMs: 4000,
    })
    sock.connect()
    FakeWebSocket.instances[0].simulateClose()
    vi.advanceTimersByTime(1000)
    FakeWebSocket.instances[1].simulateClose()
    vi.advanceTimersByTime(2000) // 第二次 2s
    expect(FakeWebSocket.instances).toHaveLength(3)
    sock.disconnect()
  })

  it('disconnect 后不重连、不接收事件', () => {
    const sock = new EventSocket({ url: 'ws://x/ws/events', onEvent })
    sock.connect()
    sock.disconnect()
    FakeWebSocket.instances[0].simulateClose()
    vi.advanceTimersByTime(5000)
    expect(FakeWebSocket.instances).toHaveLength(1)
    expect(onEvent).not.toHaveBeenCalled()
  })

  it('项目切换语义：调用方 disconnect 旧实例 + 新实例（不共享连接）', () => {
    const sockA = new EventSocket({ url: 'ws://x?project=A', onEvent })
    sockA.connect()
    const sockB = new EventSocket({ url: 'ws://x?project=B', onEvent })
    sockB.connect()
    sockA.disconnect()
    sockB.disconnect()
    expect(FakeWebSocket.closed).toHaveLength(2)
  })
})
