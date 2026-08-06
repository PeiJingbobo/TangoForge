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
import type { PermissionMap } from '@/types/models'

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

  it('渲染 16 项权限勾选', async () => {
    render(<PermissionsPanel />, { wrapper })
    await waitFor(() => expect(screen.getByLabelText('权限 task.read')).toBeInTheDocument())
    expect(screen.getAllByRole('checkbox')).toHaveLength(16)
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
  const SKILL = {
    name: 'tangoforge-usage',
    version: '1.0.0',
    description: 'TangoForge 使用指南',
    instructions: '# tangoforge-usage\n\n第一步：先调用 skill_info',
    content: 'full content',
    updated_at: '2026-08-06T10:00:00+08:00',
  }

  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tf' })
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/skills`, () =>
        HttpResponse.json({ code: 0, data: [SKILL] }),
      ),
      http.get(`${DAEMON_BASE_URL}/api/skills/tangoforge-usage`, () =>
        HttpResponse.json({ code: 0, data: SKILL }),
      ),
    )
  })

  it('渲染 Skill 列表', async () => {
    render(<SkillsPanel />, { wrapper })
    await waitFor(() => expect(screen.getByText('tangoforge-usage')).toBeInTheDocument())
    expect(screen.getByText(/v1.0.0/)).toBeInTheDocument()
  })

  it('点击查看详情（instructions 全文）', async () => {
    const user = userEvent.setup()
    render(<SkillsPanel />, { wrapper })
    await waitFor(() => expect(screen.getByText('tangoforge-usage')).toBeInTheDocument())
    await user.click(screen.getByText('tangoforge-usage'))
    await waitFor(() => expect(screen.getByText(/第一步：先调用 skill_info/)).toBeInTheDocument())
  })
})
