import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { PermissionsPanel } from './PermissionsPanel'
import { SkillsPanel } from '@/features/skills/SkillsPanel'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { toast } from 'sonner'
import type { PermissionMap, SkillPackage, HostStatus } from '@/types/models'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

const ACTIONS: PermissionMap = {
  'project.read': true,
  'task.read': true,
  'task.create': false,
  'task.update': false,
  'task.update_status': false,
  'task.delete': false,
  'task.restore': false,
  'import.run': false,
  'import.confirm': false,
  'export.run': false,
  'graph.read': false,
  'skill.read': false,
  'skill.install': false,
  'state_machine.read': false,
  'state_machine.write': false,
  'audit.read': false,
  'permission.read': false,
}

describe('PermissionsPanel', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tf' })
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/permissions`, () =>
        HttpResponse.json({ code: 0, data: { actions: ACTIONS } }),
      ),
    )
  })

  it('渲染 17 项权限（中文 label + switch）', async () => {
    render(<PermissionsPanel />, { wrapper })
    await waitFor(() => expect(screen.getByLabelText('权限 task.read')).toBeInTheDocument())
    expect(screen.getAllByRole('switch')).toHaveLength(17)
    // 中文 label 展示（含域标题「任务」与动作「查看任务」）。
    expect(screen.getByText('查看任务')).toBeInTheDocument()
    expect(screen.getByText('任务')).toBeInTheDocument()
  })

  it('勾选变更 → 保存（PUT 全量覆盖）', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/permissions`, () =>
        HttpResponse.json({
          code: 0,
          data: { actions: { ...ACTIONS, 'task.create': true } },
        }),
      ),
    )
    const user = userEvent.setup()
    render(<PermissionsPanel />, { wrapper })
    await waitFor(() => expect(screen.getByLabelText('权限 task.create')).toBeInTheDocument())
    await user.click(screen.getByLabelText('权限 task.create'))
    await user.click(screen.getByRole('button', { name: '保存权限' }))
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
  })
})

describe('SkillsPanel', () => {
  const PKG: SkillPackage = {
    name: 'taskboard-basic',
    version: '1.0.0',
    description: 'TangoForge 使用指南',
    hosts: ['.claude/skills', '.cursor/skills', '.github/skills', 'user-claude', 'user-codebuddy'],
    when_to_use: '需要管理任务时',
    instructions: '# taskboard-basic\n\n使用 task_read',
    content: 'full content',
    source: 'builtin',
    updated_at: '',
  }

  const STATUS: HostStatus[] = [
    {
      key: '.claude/skills',
      label: '.claude/skills（Claude Code）',
      scope: 'project',
      installed: [{ name: 'taskboard-basic', version: '1.0.0', state: 'current' }],
    },
    {
      key: '.cursor/skills',
      label: '.cursor/skills（Cursor）',
      scope: 'project',
      installed: [{ name: 'taskboard-basic', version: '', state: 'missing' }],
    },
    {
      key: '.github/skills',
      label: '.github/skills（GitHub Copilot）',
      scope: 'project',
      installed: [{ name: 'taskboard-basic', version: '', state: 'missing' }],
    },
    {
      key: 'user-claude',
      label: '~/.claude/skills（Claude 全局）',
      scope: 'user',
      installed: [{ name: 'taskboard-basic', version: '', state: 'missing' }],
    },
    {
      key: 'user-codebuddy',
      label: '~/.workbuddy/skills（WorkBuddy 全局）',
      scope: 'user',
      installed: [{ name: 'taskboard-basic', version: '', state: 'missing' }],
    },
  ]

  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tf' })
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/skills/packages`, () =>
        HttpResponse.json({ code: 0, data: [PKG] }),
      ),
      http.get(`${DAEMON_BASE_URL}/api/skills/status`, () =>
        HttpResponse.json({ code: 0, data: STATUS }),
      ),
    )
  })

  it('渲染技能库 + 安装状态矩阵', async () => {
    render(<SkillsPanel />, { wrapper })
    await waitFor(() => expect(screen.getAllByText('taskboard-basic').length).toBeGreaterThan(0))
    // 安装向导宿主选项（目录型 .claude/skills 等，无 .md 宿主）。
    expect(screen.getAllByText('.claude/skills（Claude Code）').length).toBeGreaterThan(0)
    expect(screen.queryByText('AGENTS.md')).not.toBeInTheDocument()
    // 状态矩阵 current 徽章。
    expect(screen.getAllByText('已安装').length).toBeGreaterThan(0)
    expect(screen.getAllByText('未安装').length).toBeGreaterThan(0)
  })

  it('安装向导：多选宿主 + 勾选包 → 批量安装到每个宿主（POST install × N）', async () => {
    const user = userEvent.setup()
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
    render(<SkillsPanel />, { wrapper })
    await waitFor(() => expect(screen.getAllByText('taskboard-basic').length).toBeGreaterThan(0))
    // 安装向导区多选两个宿主（.claude/skills + .cursor/skills Badge）。
    await user.click(screen.getAllByRole('button', { name: /\.claude\/skills/ })[0])
    await user.click(screen.getAllByRole('button', { name: /\.cursor\/skills/ })[0])
    // 安装向导区勾选技能包（首个 taskboard-basic Badge）。
    await user.click(screen.getAllByRole('button', { name: /taskboard-basic/ })[0])
    await user.click(screen.getByRole('button', { name: /安装到 2 个宿主/ }))
    await waitFor(() => expect(installBodies.length).toBe(2))
    expect(installBodies).toEqual([
      { host: '.claude/skills', packages: ['taskboard-basic'] },
      { host: '.cursor/skills', packages: ['taskboard-basic'] },
    ])
  })

  it('卸载需二次确认（Dialog）', async () => {
    const user = userEvent.setup()
    let uninstallCalled = false
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/skills/uninstall`, async () => {
        uninstallCalled = true
        return HttpResponse.json({
          code: 0,
          data: [
            { name: 'taskboard-basic', host: '.claude/skills', action: 'uninstall', ok: true },
          ],
        })
      }),
    )
    render(<SkillsPanel />, { wrapper })
    await waitFor(() => expect(screen.getAllByText('已安装').length).toBeGreaterThan(0))
    // .claude/skills 行有「卸载」按钮（表内有多个卸载，取第一个）。
    const uninstallBtns = screen.getAllByRole('button', { name: '卸载' })
    await user.click(uninstallBtns[0])
    await waitFor(() => expect(screen.getByText('确认卸载技能包')).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: '确认卸载' }))
    await waitFor(() => expect(uninstallCalled).toBe(true))
  })

  it('AGENTS.md 提示词复制（中英切换 + 变量替换）', async () => {
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
    )
    render(<SkillsPanel />, { wrapper })
    await waitFor(() => expect(screen.getByText(/放入 AGENTS.md 的推荐提示词/)).toBeInTheDocument())

    // 中文版本：验证变量替换（project_dir / daemon_port / skills_list / state_machine_list）
    const zhBtn = screen.getByRole('button', { name: '中文' })
    await user.click(zhBtn)
    // 技能库在滚动容器底部，按钮可能不在视口内，使用 hidden 绕过可访问性过滤
    const copyBtn1 = screen.getByRole('button', { name: /复制/, hidden: true })
    await user.click(copyBtn1)
    await waitFor(() => expect(clipboard).toBeCalled())
    const zhText = clipboard.mock.calls[0][0]
    expect(zhText).toContain('## TangoForge 任务管理')
    expect(zhText).toContain('默认端口为 `19810`')
    expect(zhText).toContain('project=/data/projects/tf')
    expect(zhText).toContain('taskboard-basic')
    expect(zhText).toContain('todo(待办)')
    expect(zhText).toContain('doing(进行中)')
    expect(zhText).toContain('done(已完成)')
    expect(zhText).not.toContain('{{')
    clipboard.mockClear()

    // 英文版本：验证变量替换
    const enBtn = screen.getByRole('button', { name: 'English' })
    await user.click(enBtn)
    const copyBtn2 = screen.getByRole('button', { name: /复制/, hidden: true })
    await user.click(copyBtn2)
    await waitFor(() => expect(clipboard).toBeCalled())
    const enText = clipboard.mock.calls[0][0]
    expect(enText).toContain('## TangoForge Task Management')
    expect(enText).toContain('port `19810`')
    expect(enText).toContain('project=/data/projects/tf')
    expect(enText).toContain('taskboard-basic')
    expect(enText).not.toContain('{{')
    clipboard.mockRestore()
  })
})
