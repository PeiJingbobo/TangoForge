import { describe, it, expect } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/server'
import { useProjects, useImportProject } from './useProjects'
import { useTasks, useCreateTask, useChangeStatus } from './useTasks'
import { useProjectStore } from '@/stores/project'
import { DAEMON_BASE_URL } from '@/api/client'
import { ApiError } from '@/api/client'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('useProjects', () => {
  it('加载项目列表', async () => {
    useProjectStore.setState({ project: null })
    const { result } = renderHook(() => useProjects(), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toHaveLength(1)
    expect(result.current.data?.[0].name).toBe('TangoForge')
  })

  it('导入项目 Mutation 成功', async () => {
    const { result } = renderHook(() => useImportProject(), { wrapper })
    result.current.mutate({ workdir: '/data/projects/demo' })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.name).toBe('demo')
  })
})

describe('useTasks', () => {
  it('列表数据（当前项目来自 store）', async () => {
    useProjectStore.setState({ project: '/data/projects/tangoforge' })
    const { result } = renderHook(() => useTasks(), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.tree).toHaveLength(1)
  })

  it('无项目时不发请求', async () => {
    useProjectStore.setState({ project: null })
    const { result } = renderHook(() => useTasks(), { wrapper })
    expect(result.current.isPending).toBe(true)
  })

  it('创建任务 Mutation 成功', async () => {
    useProjectStore.setState({ project: '/p' })
    const { result } = renderHook(() => useCreateTask(), { wrapper })
    result.current.mutate({ title: '新任务' })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.id).toBe('task-new')
  })

  it('非法状态流转：INVALID_TRANSITION 错误透传', async () => {
    server.use(
      http.patch(`${DAEMON_BASE_URL}/api/tasks/task-1`, () =>
        HttpResponse.json(
          { code: 'INVALID_TRANSITION', message: '非法流转：todo 不可直达 archived' },
          { status: 422 },
        ),
      ),
    )
    useProjectStore.setState({ project: '/p' })
    const { result } = renderHook(() => useChangeStatus(), { wrapper })
    result.current.mutate({ id: 'task-1', body: { status: 'archived' } })
    await waitFor(() => expect(result.current.isError).toBe(true))
    const err = result.current.error as ApiError
    expect(err.code).toBe('INVALID_TRANSITION')
  })
})
