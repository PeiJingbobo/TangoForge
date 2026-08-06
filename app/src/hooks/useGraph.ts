import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { qk } from '@/hooks/keys'
import { useProjectId } from '@/hooks/useProject'
import type { GraphData } from '@/types/models'

/** 全景图全量数据（nodes=未归档任务，edges=parent/dependency） */
export function useGraph(project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: qk.graph(pid ?? ''),
    queryFn: () => apiRequest<GraphData>('/api/graph', { project: pid }),
    enabled: !!pid,
  })
}
