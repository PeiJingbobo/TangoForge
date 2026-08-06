import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { qk } from '@/hooks/keys'
import { useProjectId } from '@/hooks/useProject'
import type { StateMachineDTO } from '@/types/models'

/** 项目状态机（⚠️ PascalCase 字段） */
export function useStateMachine(project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: qk.stateMachine(pid ?? ''),
    queryFn: () => apiRequest<StateMachineDTO>('/api/state-machine', { project: pid }),
    enabled: !!pid,
  })
}

/** 更新状态机（全量覆盖；占用状态不可移除由后端校验） */
export function useUpdateStateMachine(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (dto: StateMachineDTO) =>
      apiRequest<StateMachineDTO>('/api/state-machine', { project: pid, method: 'PUT', body: dto }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['state-machine', pid] })
      qc.invalidateQueries({ queryKey: ['tasks', pid] })
    },
  })
}
