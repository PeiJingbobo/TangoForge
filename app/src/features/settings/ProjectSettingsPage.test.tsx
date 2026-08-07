import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { ProjectSettingsPage } from './ProjectSettingsPage'
import { server } from '@/test/server'
import { DAEMON_BASE_URL } from '@/api/client'
import { useProjectStore } from '@/stores/project'
import { toast } from 'sonner'

const PROJECT_CONFIG = {
  StateMachine: {
    States: [
      { Key: 'todo', Label: '待办', Color: '#9aa0a6' },
      { Key: 'doing', Label: '进行中', Color: '#1a73e8' },
      { Key: 'done', Label: '已完成', Color: '#34a853' },
    ],
    Transitions: [
      { From: 'todo', To: ['doing', 'done'] },
      { From: 'doing', To: ['todo', 'done'] },
      { From: 'done', To: ['doing', 'todo'] },
    ],
  },
  Export: { TemplatePath: 'custom.tmpl' },
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('ProjectSettingsPage（TF-032 项目设置：状态机 + 导出 + YAML 兜底）', () => {
  beforeEach(() => {
    useProjectStore.setState({ project: '/data/projects/tangoforge' })
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/project-config`, () =>
        HttpResponse.json({ code: 0, data: PROJECT_CONFIG }),
      ),
    )
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('加载回显：状态行 / 导出模板路径 / YAML 原文（磁盘格式）', async () => {
    render(<ProjectSettingsPage />, { wrapper })
    const keyInputs = await screen.findAllByLabelText(/状态 \d+ key/)
    expect(keyInputs).toHaveLength(3)
    expect(keyInputs[0]).toHaveValue('todo')
    expect(await screen.findByLabelText(/状态 1 名称/)).toHaveValue('待办')
    expect(screen.getByLabelText('导出模板路径')).toHaveValue('custom.tmpl')
    const yaml = screen.getByLabelText('config.yaml 原文') as HTMLTextAreaElement
    expect(yaml.value).toContain('state_machine:')
    expect(yaml.value).toContain('template_path: custom.tmpl')
  })

  it('表单编辑：修改状态名 → 保存 → PUT 全量 DTO + toast', async () => {
    const toastSpy = vi.spyOn(toast, 'success').mockImplementation(() => '')
    let putBody: unknown = null
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/project-config`, async ({ request }) => {
        putBody = await request.json()
        return HttpResponse.json({ code: 0, data: PROJECT_CONFIG })
      }),
    )
    const user = userEvent.setup()
    render(<ProjectSettingsPage />, { wrapper })
    const labelInput = await screen.findByLabelText('状态 1 名称')
    await user.clear(labelInput)
    await user.type(labelInput, '待办（改）')
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(putBody).not.toBeNull())
    expect(putBody).toEqual(
      expect.objectContaining({
        StateMachine: expect.objectContaining({
          States: expect.arrayContaining([
            expect.objectContaining({ Key: 'todo', Label: '待办（改）' }),
          ]),
        }),
        Export: { TemplatePath: 'custom.tmpl' },
      }),
    )
    await waitFor(() => expect(toastSpy).toBeCalled())
    toastSpy.mockRestore()
  })

  it('导出模板路径：修改 → 保存 → PUT body 更新 TemplatePath', async () => {
    let putBody: unknown = null
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/project-config`, async ({ request }) => {
        putBody = await request.json()
        return HttpResponse.json({ code: 0, data: PROJECT_CONFIG })
      }),
    )
    const user = userEvent.setup()
    render(<ProjectSettingsPage />, { wrapper })
    const tmpl = await screen.findByLabelText('导出模板路径')
    await user.clear(tmpl)
    await user.type(tmpl, '.taskboard/gen.tmpl')
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(putBody).not.toBeNull())
    expect(putBody).toEqual(
      expect.objectContaining({ Export: { TemplatePath: '.taskboard/gen.tmpl' } }),
    )
  })

  it('YAML 原文编辑：手动修改 → 保存以 YAML 为准', async () => {
    let putBody: unknown = null
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/project-config`, async ({ request }) => {
        putBody = await request.json()
        return HttpResponse.json({ code: 0, data: PROJECT_CONFIG })
      }),
    )
    const user = userEvent.setup()
    render(<ProjectSettingsPage />, { wrapper })
    const yaml = (await screen.findByLabelText('config.yaml 原文')) as HTMLTextAreaElement
    const edited = yaml.value.replace('label: 待办', 'label: 待办区')
    await user.clear(yaml)
    await user.type(yaml, edited)
    expect(screen.getByText(/YAML 区已手动修改/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(putBody).not.toBeNull())
    expect(putBody).toEqual(
      expect.objectContaining({
        StateMachine: expect.objectContaining({
          States: expect.arrayContaining([expect.objectContaining({ Label: '待办区' })]),
        }),
      }),
    )
  })

  it('YAML 非法：保存时提示解析失败，不发 PUT', async () => {
    const toastError = vi.spyOn(toast, 'error').mockImplementation(() => '')
    let putCalled = false
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/project-config`, async () => {
        putCalled = true
        return HttpResponse.json({ code: 0, data: PROJECT_CONFIG })
      }),
    )
    const user = userEvent.setup()
    render(<ProjectSettingsPage />, { wrapper })
    const yaml = (await screen.findByLabelText('config.yaml 原文')) as HTMLTextAreaElement
    fireEvent.change(yaml, { target: { value: 'state_machine: [broken' } })
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(toastError).toBeCalled())
    expect(String(toastError.mock.calls[0]?.[0])).toContain('YAML 解析失败')
    expect(putCalled).toBe(false)
    toastError.mockRestore()
  })

  it('校验失败：PUT 422 STATUS_IN_USE → toast 提示并保留修改', async () => {
    const toastError = vi.spyOn(toast, 'error').mockImplementation(() => '')
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/project-config`, () =>
        HttpResponse.json(
          { code: 'STATUS_IN_USE', message: '状态 todo 有 2 个任务占用' },
          { status: 422 },
        ),
      ),
    )
    const user = userEvent.setup()
    render(<ProjectSettingsPage />, { wrapper })
    const labelInput = await screen.findByLabelText('状态 1 名称')
    await user.clear(labelInput)
    await user.type(labelInput, '待办占用中')
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(toastError).toBeCalled())
    expect(String(toastError.mock.calls[0]?.[0])).toContain('任务占用')
    // 保留修改：输入框仍为编辑值（供重试）
    expect(screen.getByLabelText('状态 1 名称')).toHaveValue('待办占用中')
    toastError.mockRestore()
  })

  it('删除状态：本地移除对应行与引用它的流转', async () => {
    const user = userEvent.setup()
    render(<ProjectSettingsPage />, { wrapper })
    await screen.findByLabelText('状态 1 key')
    await user.click(screen.getByRole('button', { name: '删除状态 todo' }))
    const keys = screen.getAllByLabelText(/状态 \d+ key/)
    expect(keys).toHaveLength(2)
    expect(keys[0]).toHaveValue('doing')
  })

  it('放弃修改：编辑后点放弃 → 恢复服务端值', async () => {
    const toastInfo = vi.spyOn(toast, 'info').mockImplementation(() => '')
    const user = userEvent.setup()
    render(<ProjectSettingsPage />, { wrapper })
    const labelInput = await screen.findByLabelText('状态 1 名称')
    await user.clear(labelInput)
    await user.type(labelInput, '未保存的修改')
    await user.click(screen.getByRole('button', { name: '放弃修改' }))
    expect(screen.getByLabelText('状态 1 名称')).toHaveValue('待办')
    expect(toastInfo).toBeCalled()
    toastInfo.mockRestore()
  })
})
