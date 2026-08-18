import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useEventInvalidator } from './useEvents'
import { subscribeEvents } from '@/api/ws'
import type { WSEvent } from '@/types/models'

// mock WS 订阅：捕获 handler 以便测试中手动触发事件。
vi.mock('@/api/ws', () => ({
  subscribeEvents: vi.fn(() => () => {}),
}))

const mockedSubscribe = vi.mocked(subscribeEvents)

function makeWrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

describe('useEventInvalidator knowledge 事件（TF-053 修复：无 knowledge. 前缀）', () => {
  const pid = '/data/projects/tf'
  let handlers: Array<(e: WSEvent) => void>

  beforeEach(() => {
    handlers = []
    mockedSubscribe.mockImplementation((_p, h) => {
      handlers.push(h)
      return () => {}
    })
  })

  const fire = (type: string, id?: string) => {
    const h = handlers[handlers.length - 1]
    expect(h).toBeDefined()
    h({ type, project: pid, data: id ? { id } : {}, ts: '2026-08-13T00:00:00+08:00' })
  }

  it('queue_updated 事件 → 失效 knowledge 与 tasks 查询', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const spy = vi.spyOn(qc, 'invalidateQueries')
    renderHook(() => useEventInvalidator(pid), { wrapper: makeWrapper(qc) })
    await waitFor(() => expect(handlers.length).toBe(1))

    fire('queue_updated', 'doc-x')
    expect(spy).toHaveBeenCalledWith({ queryKey: ['knowledge', pid] })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['tasks', pid] })
  })

  it('document_added / document_removed 等后端事件名同样触发失效', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const spy = vi.spyOn(qc, 'invalidateQueries')
    renderHook(() => useEventInvalidator(pid), { wrapper: makeWrapper(qc) })
    await waitFor(() => expect(handlers.length).toBe(1))

    for (const t of [
      'document_added',
      'document_removed',
      'document_relinked',
      'document_updated',
      'kb_created',
      'index_failed',
    ]) {
      fire(t, 'doc-x')
    }
    const knowledgeCalls = spy.mock.calls.filter((c) => c[0]?.queryKey?.[0] === 'knowledge')
    expect(knowledgeCalls.length).toBe(6)
  })

  it('带 knowledge. 前缀的事件仍兼容（历史/未来命名）', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const spy = vi.spyOn(qc, 'invalidateQueries')
    renderHook(() => useEventInvalidator(pid), { wrapper: makeWrapper(qc) })
    await waitFor(() => expect(handlers.length).toBe(1))

    fire('knowledge.queue_updated', 'doc-x')
    expect(spy).toHaveBeenCalledWith({ queryKey: ['knowledge', pid] })
  })

  it('task.* 事件仍只失效任务查询（不失效 knowledge）', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const spy = vi.spyOn(qc, 'invalidateQueries')
    renderHook(() => useEventInvalidator(pid), { wrapper: makeWrapper(qc) })
    await waitFor(() => expect(handlers.length).toBe(1))

    fire('task.updated', 'task-1')
    expect(spy).toHaveBeenCalledWith({ queryKey: ['tasks', pid] })
    const knowledgeCalls = spy.mock.calls.filter((c) => c[0]?.queryKey?.[0] === 'knowledge')
    expect(knowledgeCalls.length).toBe(0)
  })
})
