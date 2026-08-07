import { useMutation, useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { useProjectId } from '@/hooks/useProject'
import { qk } from '@/hooks/keys'
import type { RenderOptions, RenderResult, TemplateGenerateResult } from '@/types/models'

/** 导出 Markdown（template_mode: default|llm；target: overwrite|copy） */
export function useExportMarkdown(project?: string) {
  const pid = useProjectId(project)
  return useMutation({
    mutationFn: (opts: RenderOptions) =>
      apiRequest<RenderResult>('/api/export', { project: pid, method: 'POST', body: opts }),
  })
}

/** LLM 生成导出模板（用户提供示例文档） */
export function useGenerateTemplate(project?: string) {
  const pid = useProjectId(project)
  return useMutation({
    mutationFn: (example: string) =>
      apiRequest<TemplateGenerateResult>('/api/export/template/generate', {
        project: pid,
        method: 'POST',
        body: { example },
      }),
  })
}

/** 当前导出模板内容（TF-038 导出对话框预览）：mode=default|llm；llm 未生成 → 抛错 */
export function useExportTemplate(mode: 'default' | 'llm', project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: qk.exportTemplate(pid ?? '', mode),
    queryFn: () =>
      apiRequest<{ template: string; mode: string }>(`/api/export/template?mode=${mode}`, {
        project: pid,
      }),
    enabled: !!pid,
    retry: false,
  })
}
