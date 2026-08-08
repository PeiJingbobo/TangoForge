import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { OnboardingWizard } from './OnboardingWizard'
import { useProjectStore } from '@/stores/project'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

/** 从 Step 0 一路「跳过/默认 → 下一步」走到欢迎页（Step 6/6）的公共步骤。 */
async function walkToWelcome(user: ReturnType<typeof userEvent.setup>) {
  const next = () => screen.getByRole('button', { name: /下一步/ })
  const skip = () => screen.getByRole('button', { name: /跳过此步/ })
  const proceed = async (stepLabel: string | RegExp) => {
    await waitFor(() => expect(next()).not.toBeDisabled())
    await user.click(next())
    await waitFor(() => expect(screen.getByText(stepLabel)).toBeInTheDocument())
  }

  // Step 1/6 确认目录：check registered → 放行。
  await waitFor(() => expect(next()).not.toBeDisabled())
  await user.click(next())
  await waitFor(() => expect(screen.getByText(/Step 2\/6/)).toBeInTheDocument())
  // Step 2/6 LLM：跳过 → 下一步。
  await user.click(skip())
  await proceed(/Step 3\/6/)
  // Step 3/6 导入草稿：跳过 → 下一步。
  await user.click(skip())
  await proceed(/Step 4\/6/)
  // Step 4/6 Agent 权限：保留默认并继续 → 下一步。
  await user.click(screen.getByRole('button', { name: /保留默认并继续/ }))
  await proceed(/Step 5\/6/)
  // Step 5/6 Skill：跳过 → 下一步。
  await user.click(skip())
  await proceed(/Step 6\/6/)
  // Step 6/6 欢迎页。
  await waitFor(() => expect(screen.getByText(/已完成全部引导设置/)).toBeInTheDocument())
}

describe('OnboardingWizard（TF-041 / TF-043）', () => {
  beforeEach(() => {
    localStorage.clear()
    useProjectStore.setState({ project: null })
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/projects/check`, () =>
        HttpResponse.json({
          code: 0,
          data: { registered: true, onboarded: false, has_meta: true, meta_valid: true },
        }),
      ),
      http.get(`${DAEMON_BASE_URL}/api/config`, () =>
        HttpResponse.json({
          code: 0,
          data: {
            llm: {
              base_url: 'https://api.deepseek.com',
              api_key: 'sk-x',
              model: 'deepseek-chat',
              api_kind: 'openai',
            },
          },
        }),
      ),
      http.get(`${DAEMON_BASE_URL}/api/permissions`, () =>
        HttpResponse.json({
          code: 0,
          data: {
            actions: {
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
              'graph.read': true,
              'skill.read': true,
              'skill.install': false,
              'state_machine.read': true,
              'state_machine.write': false,
              'audit.read': true,
              'permission.read': true,
            },
          },
        }),
      ),
      http.get(`${DAEMON_BASE_URL}/api/skills/packages`, () =>
        HttpResponse.json({ code: 0, data: [] }),
      ),
    )
  })
  afterEach(() => localStorage.clear())

  it('进入欢迎页即触发 onWelcome（TF-043 需求 2：走完引导=到达欢迎页）', async () => {
    const user = userEvent.setup()
    const onWelcome = vi.fn()
    render(
      <OnboardingWizard
        open
        workdir="/data/projects/tf"
        onOpenChange={() => {}}
        onComplete={() => {}}
        onWelcome={onWelcome}
      />,
      { wrapper },
    )
    await walkToWelcome(user)
    await waitFor(() => expect(onWelcome).toHaveBeenCalledWith('/data/projects/tf'))
  })

  it('欢迎页点「进入项目」→ onComplete + 关闭', async () => {
    const user = userEvent.setup()
    const onComplete = vi.fn()
    const onOpenChange = vi.fn()
    render(
      <OnboardingWizard
        open
        workdir="/data/projects/tf"
        onOpenChange={onOpenChange}
        onComplete={onComplete}
        onWelcome={() => {}}
      />,
      { wrapper },
    )
    await walkToWelcome(user)
    await user.click(screen.getByRole('button', { name: /进入项目/ }))
    expect(onComplete).toHaveBeenCalledWith('/data/projects/tf')
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})
