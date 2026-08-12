import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { SettingsPage } from './SettingsPage'
import { ProjectSettingsPage } from './ProjectSettingsPage'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { useProjectStore } from '@/stores/project'
import { toast } from 'sonner'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

const GLOBAL_CONFIG = {
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
      model: 'qwen3-embedding:4b',
      api_kind: 'ollama',
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

describe('SettingsPage 知识库 tab（TF-052）', () => {
  beforeEach(() => {
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/config`, () =>
        HttpResponse.json({ code: 0, data: GLOBAL_CONFIG }),
      ),
    )
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('展示知识库配置（向量搜索已配置模型 → 可用）', async () => {
    render(<SettingsPage />, { wrapper })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('tab', { name: '知识库' }))
    expect(screen.getByText('启用知识库')).toBeInTheDocument()
    expect(screen.getByLabelText('Embedding 模型')).toHaveValue('qwen3-embedding:4b')
    expect(screen.getByText('向量搜索')).toBeInTheDocument()
    // 已配置模型 → 无置灰提示。
    expect(screen.queryByText(/需配置 Embedding 模型/)).not.toBeInTheDocument()
  })

  it('修改防抖窗口 → PUT /api/config knowledge 节', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    let putBody: unknown = null
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/config`, async ({ request }) => {
        putBody = await request.json()
        return HttpResponse.json({ code: 0, data: GLOBAL_CONFIG })
      }),
    )
    const user = userEvent.setup()
    render(<SettingsPage />, { wrapper })
    await user.click(await screen.findByRole('tab', { name: '知识库' }))
    const debounce = await screen.findByLabelText('防抖窗口（毫秒）')
    await user.clear(debounce)
    await user.type(debounce, '15000')
    await waitFor(() => expect(toastSpy).toBeCalled(), { timeout: 2000 })
    const body = putBody as { knowledge?: { debounce_ms?: number } }
    expect(body.knowledge?.debounce_ms).toBe(15000)
    toastSpy.mockRestore()
  })
})

describe('SettingsPage 知识库 tab 未配置模型（QA-K23）', () => {
  beforeEach(() => {
    const noModel = {
      ...GLOBAL_CONFIG,
      llm: { ...GLOBAL_CONFIG.llm, embedding: { ...GLOBAL_CONFIG.llm.embedding, model: '' } },
    }
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/config`, () =>
        HttpResponse.json({ code: 0, data: noModel }),
      ),
    )
  })

  it('未配置 Embedding 模型 → 向量搜索置灰并提示', async () => {
    render(<SettingsPage />, { wrapper })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('tab', { name: '知识库' }))
    await waitFor(() => expect(screen.getByText(/需配置 Embedding 模型/)).toBeInTheDocument())
    const switches = screen.getAllByRole('switch')
    // 向量搜索开关 disabled。
    const vs = switches[switches.length - 1]
    expect(vs).toBeDisabled()
  })
})

describe('ProjectSettingsPage 知识库节（TF-052）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tangoforge' })
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/project-config`, () =>
        HttpResponse.json({
          code: 0,
          data: {
            StateMachine: {
              States: [
                { Key: 'todo', Label: '待办', Color: '#9aa0a6' },
                { Key: 'done', Label: '完成', Color: '#34a853' },
              ],
              Transitions: [{ From: 'todo', To: ['done'] }],
            },
            Export: { TemplatePath: '' },
            Knowledge: { DefaultDocDir: 'kb_files' },
          },
        }),
      ),
    )
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('回显 default_doc_dir 并保存 PUT knowledge 节', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    let putBody: unknown = null
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/project-config`, async ({ request }) => {
        putBody = await request.json()
        return HttpResponse.json({
          code: 0,
          data: {
            StateMachine: {
              States: [
                { Key: 'todo', Label: '待办', Color: '#9aa0a6' },
                { Key: 'done', Label: '完成', Color: '#34a853' },
              ],
              Transitions: [{ From: 'todo', To: ['done'] }],
            },
            Export: { TemplatePath: '' },
            Knowledge: { DefaultDocDir: 'kb_files' },
          },
        })
      }),
    )
    const user = userEvent.setup()
    render(<ProjectSettingsPage />, { wrapper })
    const input = await screen.findByLabelText('知识库默认文档目录')
    expect(input).toHaveValue('kb_files')
    // 修改并保存。
    await user.clear(input)
    await user.type(input, 'knowledge-docs')
    await user.click(screen.getByRole('button', { name: /保存修改/ }))
    await waitFor(() => expect(toastSpy).toBeCalled(), { timeout: 3000 })
    const body = putBody as { Knowledge?: { DefaultDocDir?: string } }
    expect(body.Knowledge?.DefaultDocDir).toBe('knowledge-docs')
    toastSpy.mockRestore()
  })
})
