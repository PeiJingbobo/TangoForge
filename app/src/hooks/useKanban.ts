import { useCallback, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { apiRequest, ApiError } from '@/api/client'
import { useProjectId } from '@/hooks/useProject'
import type { Task } from '@/types/task'

/**
 * 看板拖拽状态流转（乐观更新 + 回滚）：
 * - moveTask：先本地记录 pending（卡片立即移动），再发 PATCH；
 * - 失败（如 INVALID_TRANSITION）：清除 pending 回滚 + toast；
 * - 成功后 invalidate tasks/graph，pending 随 onSettled 清除。
 */
export function useKanbanMutations(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  const [pending, setPending] = useState<Record<string, string>>({})

  const mutation = useMutation({
    mutationFn: (args: { id: string; to: string }) =>
      apiRequest<Task>(`/api/tasks/${args.id}`, {
        project: pid,
        method: 'PATCH',
        body: { status: args.to },
      }),
  })

  const moveTask = useCallback(
    (taskId: string, to: string) => {
      setPending((prev) => ({ ...prev, [taskId]: to }))
      mutation.mutate(
        { id: taskId, to },
        {
          onError: (err) => {
            // 回滚：清除本地覆盖（数据未变，卡片回到原列）
            setPending((prev) => {
              const next = { ...prev }
              delete next[taskId]
              return next
            })
            toast.error(err instanceof Error ? err.message : '状态流转失败', {
              description:
                err instanceof ApiError && err.code === 'INVALID_TRANSITION'
                  ? '已回滚到原状态'
                  : undefined,
            })
          },
          onSettled: () => {
            setPending((prev) => {
              const next = { ...prev }
              delete next[taskId]
              return next
            })
            qc.invalidateQueries({ queryKey: ['tasks', pid] })
            qc.invalidateQueries({ queryKey: ['graph', pid] })
          },
        },
      )
    },
    [mutation, pid, qc],
  )

  const getEffectiveStatus = useCallback((t: Task): string => pending[t.id] ?? t.status, [pending])

  return {
    pending,
    getEffectiveStatus,
    moveTask,
    isPending: mutation.isPending,
  }
}
