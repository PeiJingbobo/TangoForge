import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { useKanbanMutations } from './useKanban'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { toast } from 'sonner'

function mk(id: string, status: string) {
  return {
    id,
    project_id: 1,
    parent_id: null,
    title: id,
    number: '',
    description: '',
    status,
    priority: 0,
    tags: [],
    assignee: '',
    depends_on: [],
    archived_from: '',
    source_file: '',
    source_section: '',
    created_at: '2026-08-06T10:00:00+08:00',
    updated_at: '2026-08-06T10:00:00+08:00',
  }
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('useKanbanMutations（拖拽乐观更新/回滚）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tf' })
  })

  it('moveTask 成功：pending 先置位（卡片移动），成功后清除', async () => {
    const { result } = renderHook(() => useKanbanMutations(), { wrapper })
    const task = mk('a', 'todo')
    act(() => {
      result.current.moveTask('a', 'doing')
    })
    // 乐观：pending 立即可见
    expect(result.current.getEffectiveStatus(task)).toBe('doing')
    await waitFor(() => expect(result.current.isPending).toBe(false))
    // onSettled 清除 pending → 回到服务端状态
    await waitFor(() => expect(result.current.getEffectiveStatus(task)).toBe('todo'))
  })

  it('非法流转：PATCH 422 → toast 提示回滚 + pending 清除', async () => {
    const toastError = vi.spyOn(toast, 'error').mockImplementation(() => '')
    server.use(
      http.patch(`${DAEMON_BASE_URL}/api/tasks/:id`, () =>
        HttpResponse.json(
          { code: 'INVALID_TRANSITION', message: '非法流转', detail: 'todo → done 未定义' },
          { status: 422 },
        ),
      ),
    )
    const { result } = renderHook(() => useKanbanMutations(), { wrapper })
    act(() => {
      result.current.moveTask('a', 'done')
    })
    await waitFor(() => expect(toastError).toBeCalled())
    const message = toastError.mock.calls[0]?.[0]
    expect(String(message)).toContain('非法流转')
    // 回滚：pending 清除，卡片回到原列
    await waitFor(() => expect(result.current.getEffectiveStatus(mk('a', 'todo'))).toBe('todo'))
    toastError.mockRestore()
  })

  it('网络错误：toast 通用错误，pending 清除', async () => {
    const toastError = vi.spyOn(toast, 'error').mockImplementation(() => '')
    server.use(http.patch(`${DAEMON_BASE_URL}/api/tasks/:id`, () => HttpResponse.error()))
    const { result } = renderHook(() => useKanbanMutations(), { wrapper })
    act(() => {
      result.current.moveTask('a', 'doing')
    })
    await waitFor(() => expect(toastError).toBeCalled())
    await waitFor(() => expect(result.current.getEffectiveStatus(mk('a', 'todo'))).toBe('todo'))
    toastError.mockRestore()
  })
})
