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
    /** TF-046：向量嵌入配置（独立于 chat 的 llm.embedding 节） */
    embedding: {
      base_url: string
      api_key: string
      model: string
      api_kind: string
      timeout_sec: number
      max_tokens: number
    }
  }
  /** TF-052：知识库全局配置（docs/KNOWLEDGE-BASE.md §4.1） */
  knowledge: {
    enabled: boolean
    fsnotify: boolean
    startup_scan: boolean
    debounce_ms: number
    embed_concurrency: number
    max_index_size: number
    vector_search: boolean
    search_top_k: number
    search_threshold: number
    default_doc_dir: string
  }
}

/** PUT 请求体：字段省略 = 不修改；api_key/api_token 空 = 保留原值 */
export type ConfigPatch = Omit<Partial<GlobalConfigView>, 'llm' | 'knowledge'> & {
  llm?: Partial<Omit<GlobalConfigView['llm'], 'embedding'>> & {
    embedding?: Partial<GlobalConfigView['llm']['embedding']>
  }
  knowledge?: Partial<GlobalConfigView['knowledge']>
}

/** 默认 embedding 视图（后端未返回该节时的兜底，兼容旧 daemon） */
const DEFAULT_EMBEDDING = {
  base_url: '',
  api_key: '',
  model: '',
  api_kind: 'openai',
  timeout_sec: 60,
  max_tokens: 0,
}

/** 默认 knowledge 视图（后端未返回该节时的兜底，兼容旧 daemon） */
const DEFAULT_KNOWLEDGE = {
  enabled: true,
  fsnotify: true,
  startup_scan: true,
  debounce_ms: 30000,
  embed_concurrency: 1,
  max_index_size: 524288,
  vector_search: true,
  search_top_k: 10,
  search_threshold: 0.3,
  default_doc_dir: '',
}

/** 归一化配置视图：缺失的 llm.embedding / knowledge 节补默认值（旧 daemon 兼容，防止白屏） */
export function normalizeConfigView(data: GlobalConfigView): GlobalConfigView {
  const llm = data.llm ?? ({} as GlobalConfigView['llm'])
  return {
    ...data,
    llm: {
      ...llm,
      embedding: { ...DEFAULT_EMBEDDING, ...(llm.embedding ?? {}) },
    },
    knowledge: { ...DEFAULT_KNOWLEDGE, ...(data.knowledge ?? {}) },
  }
}

export function useConfig() {
  return useQuery({
    queryKey: ['global-config'],
    queryFn: async () => normalizeConfigView(await apiRequest<GlobalConfigView>('/api/config')),
    staleTime: 30_000,
  })
}

export function useUpdateConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (patch: ConfigPatch) =>
      apiRequest<GlobalConfigView>('/api/config', { method: 'PUT', body: patch }),
    onSuccess: (data) => {
      qc.setQueryData(['global-config'], normalizeConfigView(data))
    },
  })
}

/** 校验失败错误识别（422 CONFIG_INVALID → 前端回滚输入） */
export function isConfigInvalid(err: unknown): boolean {
  return err instanceof ApiError && err.code === 'CONFIG_INVALID'
}

/**
 * LLM 连接测试（TF-041 引导 Step 1）：POST /api/config/test（仅 UI）。
 * 用暂存配置测试连通性（未保存），成功后引导再 PUT /api/config 落盘。
 */
export function useTestLLM() {
  return useMutation({
    mutationFn: (cfg: { base_url?: string; api_key?: string; model?: string; api_kind?: string }) =>
      apiRequest<{ ok: boolean }>('/api/config/test', { method: 'POST', body: cfg }),
  })
}

/** 测试失败错误识别（422 LLM_TEST_FAILED） */
export function isLLMTestFailed(err: unknown): boolean {
  return err instanceof ApiError && err.code === 'LLM_TEST_FAILED'
}
