import { useMutation } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { useProjectId } from '@/hooks/useProject'
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
