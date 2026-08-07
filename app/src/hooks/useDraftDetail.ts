import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { useProjectId } from '@/hooks/useProject'
import type { DraftDetail, ParsedTask } from '@/types/models'

/**
 * 草稿明细与编辑（审阅界面数据源）：
 * - GET /api/import/drafts/{id}：完整任务树（状态机 key/优先级/依赖标题引用）；
 * - PUT /api/import/drafts/{id}/tasks：整体更新任务树（后端校验，422 不落库）。
 */
export function useDraftDetail(draftId: string | null, project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: ['draft-detail', pid ?? '', draftId ?? ''],
    queryFn: () => apiRequest<DraftDetail>(`/api/import/drafts/${draftId}`, { project: pid }),
    enabled: !!pid && !!draftId,
  })
}

export function useUpdateDraftTasks(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ draftId, tasks }: { draftId: string; tasks: ParsedTask[] }) =>
      apiRequest<{ ok: boolean }>(`/api/import/drafts/${draftId}/tasks`, {
        project: pid,
        method: 'PUT',
        body: { tasks },
      }),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ['draft-detail', pid ?? '', vars.draftId] })
    },
  })
}
