import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { qk } from '@/hooks/keys'
import { useProjectId } from '@/hooks/useProject'
import type { HostStatus, SkillInstallResult, SkillPackage } from '@/types/models'

/** 技能包列表（内置 + 全局技能库） */
export function useSkillPackages(project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: qk.skills(pid ?? ''),
    queryFn: () => apiRequest<SkillPackage[]>('/api/skills/packages', { project: pid }),
    enabled: !!pid,
  })
}

/** 技能包详情（SKILL.md 全文） */
export function useSkillPackageInfo(name: string | undefined, project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: [...qk.skills(pid ?? ''), 'package', name ?? ''],
    queryFn: () =>
      apiRequest<SkillPackage>(`/api/skills/packages/${encodeURIComponent(name ?? '')}`, {
        project: pid,
      }),
    enabled: !!pid && !!name,
  })
}

/** 宿主安装状态矩阵（missing/current/stale） */
export function useSkillStatus(project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: [...qk.skills(pid ?? ''), 'status'],
    queryFn: () => apiRequest<HostStatus[]>('/api/skills/status', { project: pid }),
    enabled: !!pid,
  })
}

/** 安装技能包到宿主位置（批量） */
export function useSkillInstall(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { host: string; packages: string[] }) =>
      apiRequest<SkillInstallResult[]>('/api/skills/install', {
        method: 'POST',
        project: pid,
        body,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: [...qk.skills(pid ?? ''), 'status'] })
    },
  })
}

/** 卸载技能包（宿主位置移除） */
export function useSkillUninstall(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { host: string; packages: string[] }) =>
      apiRequest<SkillInstallResult[]>('/api/skills/uninstall', {
        method: 'POST',
        project: pid,
        body,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: [...qk.skills(pid ?? ''), 'status'] })
    },
  })
}

/** 写入自定义技能包（全局技能库，仅 UI） */
export function useSkillPackageWrite(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { name: string; content: string }) =>
      apiRequest<SkillPackage>(`/api/skills/packages/${encodeURIComponent(body.name)}`, {
        method: 'PUT',
        project: pid,
        body: { content: body.content },
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.skills(pid ?? '') })
    },
  })
}

/** 全局默认 Skill 模板（QA-S4，全局设置页编辑；端点豁免 X-Project） */
export function useSkillTemplate() {
  return useQuery({
    queryKey: ['skill-template'],
    queryFn: () => apiRequest<{ content: string }>('/api/skill-template'),
  })
}

/** 写入自定义 Skill 模板（仅 UI；端点豁免 X-Project） */
export function useSkillTemplateWrite() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (content: string) =>
      apiRequest<{ ok: boolean }>('/api/skill-template', {
        method: 'PUT',
        body: { content },
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['skill-template'] })
    },
  })
}
