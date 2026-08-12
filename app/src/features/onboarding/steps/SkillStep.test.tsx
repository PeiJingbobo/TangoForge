import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { SkillStep } from './SkillStep'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

const PKG = {
  name: 'taskboard-basic',
  version: '1.0.0',
  description: 'TangoForge 使用指南',
  hosts: ['.claude/skills', '.cursor/skills', '.github/skills', 'user-claude', 'user-codebuddy'],
  when_to_use: '需要管理任务时',
  instructions: '# taskboard-basic',
  content: 'content',
  source: 'builtin',
  updated_at: '',
}

describe('SkillStep（TF-041 Step 4：多宿主安装）', () => {
  beforeEach(() => {
    localStorage.clear()
    useProjectStore.setState({ project: null })
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/skills/packages`, () =>
        HttpResponse.json({ code: 0, data: [PKG] }),
      ),
    )
  })
  afterEach(() => localStorage.clear())

  it('默认选中 .claude/skills；多选宿主 → 安装到全部选中宿主（POST × N）', async () => {
    const user = userEvent.setup()
    const onReady = vi.fn()
    const installBodies: unknown[] = []
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/skills/install`, async ({ request }) => {
        installBodies.push(await request.json())
        return HttpResponse.json({
          code: 0,
          data: [
            {
              name: 'taskboard-basic',
              host: '.claude/skills',
              action: 'install',
              version: '1.0.0',
              ok: true,
            },
          ],
        })
      }),
    )
    render(<SkillStep workdir="/data/projects/tf" onReady={onReady} />, { wrapper })
    // 包加载后默认勾选内置包；默认宿主已选中 .claude/skills。
    await waitFor(() => expect(screen.getByText('taskboard-basic')).toBeInTheDocument())
    // 追加选择 .cursor/skills → 2 个宿主。
    await user.click(screen.getByRole('button', { name: /\.cursor\/skills/ }))
    await user.click(screen.getByRole('button', { name: /安装到 2 个宿主/ }))
    await waitFor(() => expect(installBodies.length).toBe(2))
    expect(installBodies).toEqual([
      { host: '.claude/skills', packages: ['taskboard-basic'] },
      { host: '.cursor/skills', packages: ['taskboard-basic'] },
    ])
    await waitFor(() => expect(onReady).toHaveBeenCalledWith(true))
  })

  it('取消宿主选择后按钮文案变为「不安装并继续」', async () => {
    const user = userEvent.setup()
    render(<SkillStep workdir="/data/projects/tf" onReady={vi.fn()} />, { wrapper })
    await waitFor(() => expect(screen.getByText('taskboard-basic')).toBeInTheDocument())
    // 取消默认选中的 .claude/skills → 0 个宿主。
    await user.click(screen.getAllByRole('button', { name: /\.claude\/skills/ })[0])
    expect(screen.getByRole('button', { name: /不安装并继续/ })).toBeInTheDocument()
  })

  it('AGENTS.md 推荐提示词：复制含变量替换的实时配置', async () => {
    const user = userEvent.setup()
    const clipboard = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/config`, () =>
        HttpResponse.json({
          code: 0,
          data: {
            port: 19810,
            remote_access: false,
            api_token: '',
            llm: {
              base_url: '',
              api_key: '',
              model: '',
              api_kind: 'openai',
              timeout_sec: 30,
              retries: 3,
              max_tokens: 4096,
              concurrency: 5,
            },
          },
        }),
      ),
      http.get(`${DAEMON_BASE_URL}/api/state-machine`, () =>
        HttpResponse.json({
          code: 0,
          data: {
            States: [
              { Key: 'todo', Label: '待办', Color: '#6b7280' },
              { Key: 'doing', Label: '进行中', Color: '#2563eb' },
              { Key: 'done', Label: '已完成', Color: '#16a34a' },
            ],
            Transitions: [],
          },
        }),
      ),
      http.get(`${DAEMON_BASE_URL}/api/skills/status`, () =>
        HttpResponse.json({
          code: 0,
          data: [
            {
              key: '.claude/skills',
              label: '.claude/skills（Claude Code）',
              scope: 'project',
              installed: [{ name: 'taskboard-basic', version: '1.0.0', state: 'current' }],
            },
          ],
        }),
      ),
    )
    render(<SkillStep workdir="/data/projects/tf" onReady={vi.fn()} />, { wrapper })
    await waitFor(() => expect(screen.getByText(/放入 AGENTS.md 的推荐提示词/)).toBeInTheDocument())

    // 中文复制
    await user.click(screen.getByRole('button', { name: '中文' }))
    await user.click(screen.getByRole('button', { name: /复制/, hidden: true }))
    await waitFor(() => expect(clipboard).toBeCalled())
    const zhText = clipboard.mock.calls[0][0]
    expect(zhText).toContain('## TangoForge 任务管理')
    expect(zhText).toContain('默认端口为 `19810`')
    expect(zhText).toContain('project=/data/projects/tf')
    expect(zhText).toContain('taskboard-basic')
    expect(zhText).toContain('todo(待办)')
    expect(zhText).not.toContain('{{')
    clipboard.mockClear()

    // 英文复制
    await user.click(screen.getByRole('button', { name: 'English' }))
    await user.click(screen.getByRole('button', { name: /复制/, hidden: true }))
    await waitFor(() => expect(clipboard).toBeCalled())
    const enText = clipboard.mock.calls[0][0]
    expect(enText).toContain('## TangoForge Task Management')
    expect(enText).toContain('port `19810`')
    expect(enText).toContain('project=/data/projects/tf')
    expect(enText).not.toContain('{{')
    clipboard.mockRestore()
  })
})
