import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { qk } from '@/hooks/keys'
import { useProjectId } from '@/hooks/useProject'
import type { AuditQueryResult } from '@/types/models'

export interface AuditFilter {
  actor?: string
  action?: string
  page?: number
  size?: number
}

/** 审计日志查询（ts 倒序；filter 精确匹配） */
export function useAudit(filter?: AuditFilter, project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: [...qk.audit(pid ?? ''), filter ?? {}],
    queryFn: () =>
      apiRequest<AuditQueryResult>('/api/audit', {
        project: pid,
        query: {
          ...(filter?.actor ? { 'filter[actor]': filter.actor } : {}),
          ...(filter?.action ? { 'filter[action]': filter.action } : {}),
          ...(filter?.page ? { page: filter.page } : {}),
          ...(filter?.size ? { size: filter.size } : {}),
        },
      }),
    enabled: !!pid,
  })
}
