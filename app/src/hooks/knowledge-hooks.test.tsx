import { describe, it, expect, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { useKnowledgeDocuments } from './useKnowledge'
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
    // 首次请求返回正常数据（无 indexing）→ 轮询应为 false。
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
