import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, ApiError } from '@/api/client'

/**
 * 全局配置（首选项页数据源，GET/PUT /api/config；仅 UI 可读写）。
 * - getConfig：脱敏视图（api_key/api_token 掩码，仅用于展示已配置状态）；
 * - updateConfig：全量覆盖，后端校验（422 CONFIG_INVALID 不落盘，前端回滚输入）。
 */
export interface GlobalConfigView {
  port: number
  remote_access: boolean
  api_token: string
  llm: {
    base_url: string
    api_key: string
    model: string
    api_kind: string
    timeout_sec: number
    retries: number
    max_tokens: number
    concurrency: number
  }
}

/** PUT 请求体：字段省略 = 不修改；api_key/api_token 空 = 保留原值 */
export type ConfigPatch = Omit<Partial<GlobalConfigView>, 'llm'> & {
  llm?: Partial<GlobalConfigView['llm']>
}

export function useConfig() {
  return useQuery({
    queryKey: ['global-config'],
    queryFn: () => apiRequest<GlobalConfigView>('/api/config'),
    staleTime: 30_000,
  })
}

export function useUpdateConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (patch: ConfigPatch) =>
      apiRequest<GlobalConfigView>('/api/config', { method: 'PUT', body: patch }),
    onSuccess: (data) => {
      qc.setQueryData(['global-config'], data)
    },
  })
}

/** 校验失败错误识别（422 CONFIG_INVALID → 前端回滚输入） */
export function isConfigInvalid(err: unknown): boolean {
  return err instanceof ApiError && err.code === 'CONFIG_INVALID'
}
