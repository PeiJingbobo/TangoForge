/**
 * LLM 平台快捷字典（TF-041 引导流程 Step 1）：
 * 提供主流大模型平台的 base_url / api_kind（协议兼容类型）/ 可用模型数组，
 * 用户在引导中选择平台后自动填充，仅需输入 APIKey（或切「自定义」手动填）。
 */

export interface LLMProvider {
  key: string
  name: string
  baseUrl: string
  /** openai / anthropic / responses（对应后端 LLMConfig.APIKind） */
  apiKind: 'openai' | 'anthropic' | 'responses'
  /** 可选模型列表（placeholder 级，与平台当前模型对齐） */
  models: string[]
  /** 默认选中模型（models[0] 兜底） */
  defaultModel?: string
  /** 是否需要 baseUrl 覆盖提示（如 OpenAI 兼容网关） */
  note?: string
}

export const LLM_PROVIDERS: LLMProvider[] = [
  {
    key: 'deepseek',
    name: 'DeepSeek',
    baseUrl: 'https://api.deepseek.com/v1',
    apiKind: 'openai',
    models: ['deepseek-chat', 'deepseek-reasoner'],
  },
  {
    key: 'openai',
    name: 'OpenAI',
    baseUrl: 'https://api.openai.com/v1',
    apiKind: 'openai',
    models: ['gpt-4o', 'gpt-4o-mini'],
  },
  {
    key: 'anthropic',
    name: 'Anthropic Claude',
    baseUrl: 'https://api.anthropic.com',
    apiKind: 'anthropic',
    models: ['claude-sonnet-4-20250514', 'claude-haiku-4-5-20251001'],
  },
  {
    key: 'moonshot',
    name: 'Kimi（Moonshot）',
    baseUrl: 'https://api.moonshot.cn/v1',
    apiKind: 'openai',
    models: ['moonshot-v1-8k', 'moonshot-v1-32k', 'moonshot-v1-128k'],
  },
  {
    key: 'zhipu',
    name: '智谱 GLM',
    baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    apiKind: 'openai',
    models: ['glm-4-plus', 'glm-4-air'],
  },
  {
    key: 'qwen',
    name: '通义千问',
    baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    apiKind: 'openai',
    models: ['qwen-plus', 'qwen-max', 'qwen-turbo'],
  },
  {
    key: 'custom',
    name: '自定义（OpenAI 兼容）',
    baseUrl: '',
    apiKind: 'openai',
    models: [],
    note: '手动填写 base_url / 模型 / APIKey；仅支持 OpenAI 兼容协议。',
  },
]

export function providerByKey(key: string): LLMProvider | undefined {
  return LLM_PROVIDERS.find((p) => p.key === key)
}
