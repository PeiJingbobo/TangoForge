import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, ApiError } from '@/api/client'
import { qk } from '@/hooks/keys'
import { useProjectId } from '@/hooks/useProject'
import type { ProjectConfigDTO } from '@/types/models'

/**
 * 项目配置（设置页数据源，GET/PUT /api/project-config，TF-032）。
 * - useProjectConfig：读取项目 config.yaml（缺失回退默认），⚠️ PascalCase 字段；
 * - useUpdateProjectConfig：全量覆盖 state_machine + export（仅 UI 可写，
 *   后端校验失败 400/422 抛 ApiError，前端回滚输入）；
 * - PUT 成功后同步失效 state-machine 缓存（两端点共享状态机数据）。
 */
export function useProjectConfig(project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: qk.projectConfig(pid ?? ''),
    queryFn: () => apiRequest<ProjectConfigDTO>('/api/project-config', { project: pid }),
    enabled: !!pid,
    staleTime: 30_000,
  })
}

export function useUpdateProjectConfig(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (dto: ProjectConfigDTO) =>
      apiRequest<ProjectConfigDTO>('/api/project-config', {
        project: pid,
        method: 'PUT',
        body: dto,
      }),
    onSuccess: (data) => {
      qc.setQueryData(qk.projectConfig(pid ?? ''), data)
      qc.invalidateQueries({ queryKey: qk.stateMachine(pid ?? '') })
      qc.invalidateQueries({ queryKey: qk.tasks(pid ?? '') })
    },
  })
}

/** 校验失败错误识别（400 TASK_INVALID / 422 STATUS_IN_USE 等 → 前端回滚输入） */
export function isProjectConfigRejected(err: unknown): boolean {
  return (
    err instanceof ApiError &&
    (err.code === 'TASK_INVALID' ||
      err.code === 'STATUS_IN_USE' ||
      err.code === 'CONFIG_INVALID' ||
      err.code === 'CONFIG_SAVE_FAILED')
  )
}
