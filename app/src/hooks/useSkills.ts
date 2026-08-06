import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { qk } from '@/hooks/keys'
import { useProjectId } from '@/hooks/useProject'
import type { Skill } from '@/types/models'

/** Skill 列表（每次查询前重扫 skills/ 目录） */
export function useSkills(project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: qk.skills(pid ?? ''),
    queryFn: () => apiRequest<Skill[]>('/api/skills', { project: pid }),
    enabled: !!pid,
  })
}

/** Skill 详情（instructions 全文） */
export function useSkillInfo(name: string | undefined, project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: [...qk.skills(pid ?? ''), name ?? ''],
    queryFn: () =>
      apiRequest<Skill>(`/api/skills/${encodeURIComponent(name ?? '')}`, { project: pid }),
    enabled: !!pid && !!name,
  })
}
