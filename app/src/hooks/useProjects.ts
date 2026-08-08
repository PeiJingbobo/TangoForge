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

/** 重命名项目（仅 UI；只改显示名称，不动磁盘/workdir） */
export function useRenameProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { id: number; name: string }) =>
      apiRequest<Project>(`/api/projects/${body.id}`, {
        method: 'PATCH',
        body: { name: body.name },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.projects })
    },
  })
}

/** 目录导入前置检查（TF-041 引导 Step 0；TF-043 加 onboarded）：
 *  {registered, onboarded, has_meta, meta_valid, meta_reason} */
export function useProjectCheck() {
  return useMutation({
    mutationFn: (workdir: string) =>
      apiRequest<ProjectCheckResult>('/api/projects/check', {
        method: 'POST',
        body: { workdir },
      }),
  })
}

export interface ProjectCheckResult {
  registered: boolean
  /** 引导是否已完成（TF-043：已注册且 hidden=0）——完成后选择目录可直接进入 */
  onboarded: boolean
  has_meta: boolean
  meta_valid: boolean
  meta_reason?: string
}

/** 标记项目引导完成（TF-043：暂时隐藏 → 列表可见；仅 UI） */
export function useCompleteOnboarding() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (workdir: string) =>
      apiRequest<Project>('/api/projects/complete', {
        method: 'POST',
        body: { workdir },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.projects })
    },
  })
}

/** 清空历史元数据（TF-041 引导：元数据过旧/损坏时重置；仅 UI） */
export function useResetProjectMetadata() {
  return useMutation({
    mutationFn: (workdir: string) =>
      apiRequest<{ reset: boolean }>('/api/projects/import/reset', {
        method: 'POST',
        body: { workdir },
      }),
  })
}
