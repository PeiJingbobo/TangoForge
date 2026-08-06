import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { qk } from '@/hooks/keys'
import { useProjectId } from '@/hooks/useProject'
import type { ImportConfirmResult, ImportDraft, ParseInput } from '@/types/models'

/** 草稿列表（pending，created_at 倒序） */
export function useDrafts(project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: qk.drafts(pid ?? ''),
    queryFn: () => apiRequest<ImportDraft[]>('/api/import/drafts', { project: pid }),
    enabled: !!pid,
  })
}

/** 提交 Markdown 解析（四形态取一，见 ParseInput；LLM 失败整次不落库） */
export function useImportTasks(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: ParseInput) =>
      apiRequest<ImportDraft>('/api/import', { project: pid, method: 'POST', body: input }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['drafts', pid] })
    },
  })
}

/** 确认草稿（文件级全量覆盖入库） */
export function useConfirmDraft(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (draftId: string) =>
      apiRequest<ImportConfirmResult>(`/api/import/drafts/${draftId}/confirm`, {
        project: pid,
        method: 'POST',
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['drafts', pid] })
      qc.invalidateQueries({ queryKey: ['tasks', pid] })
      qc.invalidateQueries({ queryKey: ['graph', pid] })
    },
  })
}

/** 丢弃草稿 */
export function useDiscardDraft(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (draftId: string) =>
      apiRequest<{ ok: boolean }>(`/api/import/drafts/${draftId}`, {
        project: pid,
        method: 'DELETE',
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['drafts', pid] })
    },
  })
}
