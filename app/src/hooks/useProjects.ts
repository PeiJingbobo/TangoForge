import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { qk } from '@/hooks/keys'
import type { Project, ProjectImportRequest } from '@/types/models'

/** 项目列表（豁免 X-Project） */
export function useProjects() {
  return useQuery({
    queryKey: qk.projects,
    queryFn: () => apiRequest<Project[]>('/api/projects'),
  })
}

/** 导入项目：workdir 已初始化则注册复用，否则引导初始化（TF-024 处理引导流） */
export function useImportProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: ProjectImportRequest) =>
      apiRequest<Project>('/api/projects/import', { method: 'POST', body: req }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.projects })
    },
  })
}

/** 移除项目注册（仅 UI；不删除磁盘数据） */
export function useRemoveProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      apiRequest<{ removed: boolean }>(`/api/projects/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.projects })
    },
  })
}
