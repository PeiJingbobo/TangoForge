import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { qk } from '@/hooks/keys'
import { useProjectId } from '@/hooks/useProject'
import type { Task, TaskListFilter, TaskListResult } from '@/types/task'
import type { ChangeStatusInput, CreateTaskInput, UpdateTaskInput } from '@/types/task'

function buildTaskQuery(
  filter?: TaskListFilter,
): Record<string, string | number | boolean | undefined> {
  return {
    ...(filter?.status ? { 'filter[status]': filter.status } : {}),
    ...(filter?.q ? { q: filter.q } : {}),
    ...(filter?.page ? { page: filter.page } : {}),
    ...(filter?.size ? { size: filter.size } : {}),
  }
}

/** 任务列表（树形/分页，见 TaskListFilter） */
export function useTasks(filter?: TaskListFilter, project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: [...qk.tasks(pid ?? ''), filter ?? {}],
    queryFn: () =>
      apiRequest<TaskListResult>('/api/tasks', {
        project: pid,
        query: buildTaskQuery(filter),
      }),
    enabled: !!pid,
  })
}

/** 单任务 */
export function useTask(id?: string, project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: qk.task(pid ?? '', id ?? ''),
    queryFn: () => apiRequest<Task>(`/api/tasks/${id}`, { project: pid }),
    enabled: !!pid && !!id,
  })
}

function useInvalidateTask(pid: string | undefined) {
  const qc = useQueryClient()
  const invalidate = (id?: string) => {
    qc.invalidateQueries({ queryKey: ['tasks', pid] })
    qc.invalidateQueries({ queryKey: ['graph', pid] })
    if (id) qc.invalidateQueries({ queryKey: ['tasks', pid, id] })
  }
  return { invalidate }
}

/** 创建任务 */
export function useCreateTask(project?: string) {
  const pid = useProjectId(project)
  const { invalidate } = useInvalidateTask(pid)
  return useMutation({
    mutationFn: (input: CreateTaskInput) =>
      apiRequest<Task>('/api/tasks', { project: pid, method: 'POST', body: input }),
    onSuccess: (t) => invalidate(t.id),
  })
}

/** 更新任务（可含 status → 服务端拆分为 ChangeStatus 调用，需 task.update_status 权限） */
export function useUpdateTask(project?: string) {
  const pid = useProjectId(project)
  const { invalidate } = useInvalidateTask(pid)
  return useMutation({
    mutationFn: (args: { id: string; body: UpdateTaskInput & { status?: string | null } }) =>
      apiRequest<Task>(`/api/tasks/${args.id}`, {
        project: pid,
        method: 'PATCH',
        body: args.body,
      }),
    onSuccess: (t) => invalidate(t.id),
  })
}

/** 仅状态流转（同 useUpdateTask 的 status 语义，供看板拖拽等场景显式表达） */
export function useChangeStatus(project?: string) {
  const pid = useProjectId(project)
  const { invalidate } = useInvalidateTask(pid)
  return useMutation({
    mutationFn: (args: { id: string; body: ChangeStatusInput }) =>
      apiRequest<Task>(`/api/tasks/${args.id}`, {
        project: pid,
        method: 'PATCH',
        body: args.body,
      }),
    onSuccess: (t) => invalidate(t.id),
  })
}

/** 归档（返回依赖数/子任务清空提示） */
export function useArchiveTask(project?: string) {
  const pid = useProjectId(project)
  const { invalidate } = useInvalidateTask(pid)
  return useMutation({
    mutationFn: (id: string) =>
      apiRequest<{ task: Task; dependent_count: number; children_cleared: number }>(
        `/api/tasks/${id}/archive`,
        { project: pid, method: 'POST' },
      ),
    onSuccess: (r) => invalidate(r.task.id),
  })
}

/** 还原（fallback_todo 缺省 false；archived_from 状态失效时回退 todo） */
export function useRestoreTask(project?: string) {
  const pid = useProjectId(project)
  const { invalidate } = useInvalidateTask(pid)
  return useMutation({
    mutationFn: (args: { id: string; fallbackTodo?: boolean }) =>
      apiRequest<Task>(`/api/tasks/${args.id}/restore`, {
        project: pid,
        method: 'POST',
        body: { fallback_todo: args.fallbackTodo },
      }),
    onSuccess: (t) => invalidate(t.id),
  })
}

/** 物理删除（仅回收站 archived 任务） */
export function useDeleteTask(project?: string) {
  const pid = useProjectId(project)
  const { invalidate } = useInvalidateTask(pid)
  return useMutation({
    mutationFn: (id: string) =>
      apiRequest<Task>(`/api/tasks/${id}`, { project: pid, method: 'DELETE' }),
    onSuccess: (t) => invalidate(t.id),
  })
}
