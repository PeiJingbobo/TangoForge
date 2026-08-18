import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { useKnowledgeDocuments, useKnowledgeQueue } from './useKnowledge'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('useKnowledgeDocuments refetchInterval 防御（TF-052 白屏修复）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tangoforge' })
  })

  it('无 data 时（切换库 query key 变化）不抛错', async () => {
    // 首次请求返回正常数据（无 indexing）→ 空闲兜底轮询（5s）不应抛错。
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/knowledge/documents`, () =>
        HttpResponse.json({
          code: 0,
          data: { items: [], total: 0, page: 0, size: 50 },
        }),
      ),
    )
    const { result, unmount } = renderHook(() => useKnowledgeDocuments(undefined), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    // 切换 filter（选中库）→ 新 query key，挂载期 data 为空/undefined。
    const { result: r2 } = renderHook(() => useKnowledgeDocuments({ kb_id: 1 }), { wrapper })
    await waitFor(() => expect(r2.current.isSuccess).toBe(true))
    unmount()
  })

  it('数据为 null items 时不抛错（refetchInterval 防御）', async () => {
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/knowledge/documents`, () =>
        HttpResponse.json({ code: 0, data: { items: null, total: 0, page: 0, size: 50 } }),
      ),
    )
    const { result } = renderHook(() => useKnowledgeDocuments({ kb_id: 1 }), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })
})

describe('知识库队列/文档轮询兜底（TF-053 修复：LLM 导入后实时刷新）', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    useProjectStore.setState({ project: '/data/projects/tangoforge' })
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('队列空闲（无活跃任务）时仍保留兜底轮询，后续入队任务可被拉取', async () => {
    let queueCalls = 0
    // 第 1 次返回空快照（模拟进入页面时队列为空）→ 之后轮询应继续，而不是永久停止。
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/knowledge/queue`, () => {
        queueCalls++
        if (queueCalls === 1) {
          return HttpResponse.json({
            code: 0,
            data: { workdir: 'x', pending: [], embedding: [], done: [], failed: [], canceled: [] },
          })
        }
        // 第 2 次起：出现排队任务（模拟 LLM/MCP 导入入队）。
        return HttpResponse.json({
          code: 0,
          data: {
            workdir: 'x',
            pending: [
              {
                doc_id: 'doc-new',
                path: 'docs/new.md',
                display_name: 'new.md',
                status: 'pending',
                enqueued_at: '2026-08-13T09:00:00+08:00',
              },
            ],
            embedding: [],
            done: [],
            failed: [],
            canceled: [],
          },
        })
      }),
    )
    const { result } = renderHook(() => useKnowledgeQueue(), { wrapper })
    // fake timers 下 flush 初始请求（MSW 响应为微任务，advanceTimersByTimeAsync 会 flush）。
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(result.current.isSuccess).toBe(true)
    expect(result.current.data?.pending).toHaveLength(0)
    expect(queueCalls).toBe(1)

    // 逐步推进兜底轮询（5s/步）：应重新拉取并看到新入队任务。
    // fake timers 下 React Query 的 interval 重调度/渲染刷新可能有延迟，循环推进至数据出现。
    let seenNew = false
    for (let i = 0; i < 6 && !seenNew; i++) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(5000)
      })
      seenNew = (result.current.data?.pending ?? []).length === 1
    }
    expect(seenNew).toBe(true)
    expect(queueCalls).toBeGreaterThanOrEqual(2)
  })

  it('文档列表无 indexing 时保留兜底轮询，文档状态可被刷新', async () => {
    let docCalls = 0
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/knowledge/documents`, () => {
        docCalls++
        if (docCalls === 1) {
          // 初始：一条 ok/未嵌入（embedded=0）文档 → 无 indexing → 旧逻辑会停轮。
          return HttpResponse.json({
            code: 0,
            data: {
              items: [
                {
                  id: 'doc-a',
                  path: 'docs/a.md',
                  display_name: 'a.md',
                  status: 'ok',
                  embedded: 0,
                  updated_at: '2026-08-13T09:00:00+08:00',
                },
              ],
              total: 1,
              page: 0,
              size: 50,
            },
          })
        }
        // 之后：该文档进入 indexing（模拟 LLM 导入后开始嵌入）。
        return HttpResponse.json({
          code: 0,
          data: {
            items: [
              {
                id: 'doc-a',
                path: 'docs/a.md',
                display_name: 'a.md',
                status: 'indexing',
                embedded: 0,
                updated_at: '2026-08-13T09:00:01+08:00',
              },
            ],
            total: 1,
            page: 0,
            size: 50,
          },
        })
      }),
    )
    const { result } = renderHook(() => useKnowledgeDocuments({}, '/data/projects/tangoforge'), {
      wrapper,
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(result.current.isSuccess).toBe(true)
    expect(result.current.data?.items?.[0]?.status).toBe('ok')
    expect(docCalls).toBe(1)

    // 逐步推进兜底轮询（5s/步）→ 应看到 indexing。
    let seenIndexing = false
    for (let i = 0; i < 6 && !seenIndexing; i++) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(5000)
      })
      seenIndexing = result.current.data?.items?.[0]?.status === 'indexing'
    }
    expect(seenIndexing).toBe(true)
    expect(docCalls).toBeGreaterThanOrEqual(2)
  })
})
