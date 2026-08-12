import { describe, it, expect } from 'vitest'
import { normalizeConfigView, type GlobalConfigView } from './useConfig'

const BASE: GlobalConfigView = {
  port: 19810,
  remote_access: false,
  api_token: '',
  llm: {
    base_url: 'https://api.deepseek.com',
    api_key: 'sk-****67',
    model: 'deepseek-chat',
    api_kind: 'openai',
    timeout_sec: 120,
    retries: 1,
    max_tokens: 4096,
    concurrency: 1,
    embedding: {
      base_url: '',
      api_key: '',
      model: '',
      api_kind: 'openai',
      timeout_sec: 60,
      max_tokens: 0,
    },
  },
  knowledge: {
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
  },
}

describe('normalizeConfigView（旧 daemon 兼容，TF-052 白屏修复）', () => {
  it('缺失 llm.embedding / knowledge 节时补默认值', () => {
    // 模拟旧 daemon：无 embedding / knowledge 字段。
    const old = {
      port: BASE.port,
      remote_access: BASE.remote_access,
      api_token: BASE.api_token,
      llm: {
        base_url: BASE.llm.base_url,
        api_key: BASE.llm.api_key,
        model: BASE.llm.model,
        api_kind: BASE.llm.api_kind,
        timeout_sec: BASE.llm.timeout_sec,
        retries: BASE.llm.retries,
        max_tokens: BASE.llm.max_tokens,
        concurrency: BASE.llm.concurrency,
      },
    } as unknown as GlobalConfigView
    const norm = normalizeConfigView(old)
    expect(norm.llm.embedding.model).toBe('')
    expect(norm.llm.embedding.api_kind).toBe('openai')
    expect(norm.llm.embedding.timeout_sec).toBe(60)
    expect(norm.knowledge.enabled).toBe(true)
    expect(norm.knowledge.fsnotify).toBe(true)
    expect(norm.knowledge.debounce_ms).toBe(30000)
    expect(norm.knowledge.max_index_size).toBe(524288)
    expect(norm.knowledge.search_top_k).toBe(10)
  })

  it('已有字段原样保留（不覆盖）', () => {
    const cfg: GlobalConfigView = {
      ...BASE,
      llm: {
        ...BASE.llm,
        embedding: {
          ...BASE.llm.embedding,
          model: 'qwen3-embedding:4b',
          api_kind: 'ollama',
          timeout_sec: 45,
        },
      },
      knowledge: { ...BASE.knowledge, fsnotify: false, debounce_ms: 15000, search_top_k: 8 },
    }
    const norm = normalizeConfigView(cfg)
    expect(norm.llm.embedding.model).toBe('qwen3-embedding:4b')
    expect(norm.llm.embedding.api_kind).toBe('ollama')
    expect(norm.llm.embedding.timeout_sec).toBe(45)
    expect(norm.knowledge.fsnotify).toBe(false)
    expect(norm.knowledge.debounce_ms).toBe(15000)
    expect(norm.knowledge.search_top_k).toBe(8)
  })

  it('部分缺失时合并默认（仅补缺）', () => {
    const cfg = JSON.parse(JSON.stringify(BASE)) as GlobalConfigView
    cfg.llm.embedding = { ...BASE.llm.embedding, model: 'nomic-embed-text' }
    // 缺 knowledge 整节（构造无 knowledge 的对象）。
    const partial = {
      ...cfg,
      knowledge: undefined,
    } as unknown as GlobalConfigView
    const norm = normalizeConfigView(partial)
    expect(norm.llm.embedding.model).toBe('nomic-embed-text')
    expect(norm.knowledge.vector_search).toBe(true)
    expect(norm.knowledge.search_threshold).toBe(0.3)
  })
})
