import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { qk } from '@/hooks/keys'
import { useProjectId } from '@/hooks/useProject'
import type { PermissionMap } from '@/types/models'

/** 权限表（16 action，缺失行视为 false） */
export function usePermissions(project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: qk.permissions(pid ?? ''),
    queryFn: () => apiRequest<{ actions: PermissionMap }>('/api/permissions', { project: pid }),
    enabled: !!pid,
  })
}

/** 保存权限（全量覆盖；仅 UI 身份可写，接口层已双重校验） */
export function useSavePermissions(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (actions: PermissionMap) =>
      apiRequest<{ actions: PermissionMap }>('/api/permissions', {
        project: pid,
        method: 'PUT',
        body: { actions },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['permissions', pid] })
    },
  })
}
