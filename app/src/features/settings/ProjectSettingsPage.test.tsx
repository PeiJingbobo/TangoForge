import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { ProjectSettingsPage } from './ProjectSettingsPage'
import { reorderStateRows } from './state-machine-utils'
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

/**
 * 等待编辑副本初始化完成（draft/targets/rowIds 由 effect 建立）。
 * 竞态背景：首次渲染（effect 前）rowIds 为空 → 状态行 key 为索引；
 * effect 后 key 变为 uuid → 整行重新挂载，先前获取的节点已脱离 DOM。
 * 以 YAML 区内容非空（effect 中写入）作为就绪信号，之后所有查询拿到的都是稳定节点。
 */
async function waitEditorReady() {
  const yaml = await screen.findByLabelText('config.yaml 原文')
  await waitFor(() => expect((yaml as HTMLTextAreaElement).value).toContain('state_machine:'))
}

describe('ProjectSettingsPage（TF-032 项目设置：派生流转规则 + 拖拽排序）', () => {
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

  it('加载回显：状态行（含行内流转目标标签）/ 导出模板路径 / YAML 原文', async () => {
    render(<ProjectSettingsPage />, { wrapper })
    await waitEditorReady()
    const keyInputs = await screen.findAllByLabelText(/状态 \d+ key/)
    expect(keyInputs).toHaveLength(3)
    expect(keyInputs[0]).toHaveValue('todo')
    expect(await screen.findByLabelText('状态 1 名称')).toHaveValue('待办')
    // 每个状态行内联「流转到」目标标签（无独立流转规则区）。
    const targetBadges = screen.getAllByLabelText(/流转目标/)
    expect(targetBadges.length).toBeGreaterThanOrEqual(9)
    expect(screen.queryByText('流转规则')).not.toBeInTheDocument()
    expect(screen.getByLabelText('导出模板路径')).toHaveValue('custom.tmpl')
    const yaml = screen.getByLabelText('config.yaml 原文') as HTMLTextAreaElement
    expect(yaml.value).toContain('state_machine:')
    expect(yaml.value).toContain('template_path: custom.tmpl')
  })

  it('点亮目标标签：点击「待办 流转目标 已完成」→ 保存 → Transitions 由 targets 生成', async () => {
    let putBody: unknown = null
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/project-config`, async ({ request }) => {
        putBody = await request.json()
        return HttpResponse.json({ code: 0, data: PROJECT_CONFIG })
      }),
    )
    const user = userEvent.setup()
    render(<ProjectSettingsPage />, { wrapper })
    await waitEditorReady()
    // 待办行的「已完成」目标默认点亮（todo→[doing,done]），点击取消。
    const todoBadge = await screen.findByLabelText('状态 待办 流转目标 已完成')
    const yamlBefore = (screen.getByLabelText('config.yaml 原文') as HTMLTextAreaElement).value
    fireEvent.click(todoBadge)
    await waitFor(() => {
      const yaml = screen.getByLabelText('config.yaml 原文') as HTMLTextAreaElement
      expect(yaml.value).not.toBe(yamlBefore)
    })
    await waitFor(() => expect(screen.getByRole('button', { name: '保存修改' })).toBeEnabled())
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(putBody).not.toBeNull())
    expect(putBody).toEqual(
      expect.objectContaining({
        StateMachine: expect.objectContaining({
          Transitions: expect.arrayContaining([
            expect.objectContaining({ From: 'todo', To: ['doing'] }), // done 已取消
            expect.objectContaining({ From: 'doing', To: ['todo', 'done'] }),
          ]),
        }),
      }),
    )
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
    await waitEditorReady()
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
    await waitEditorReady()
    const tmpl = await screen.findByLabelText('导出模板路径')
    await user.clear(tmpl)
    await user.type(tmpl, '.taskboard/gen.tmpl')
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(putBody).not.toBeNull())
    expect(putBody).toEqual(
      expect.objectContaining({ Export: { TemplatePath: '.taskboard/gen.tmpl' } }),
    )
  })

  it('YAML 原文编辑：手动修改 → 保存以 YAML 为准（States 更新，Transitions 重新生成）', async () => {
    let putBody: unknown = null
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/project-config`, async ({ request }) => {
        putBody = await request.json()
        return HttpResponse.json({ code: 0, data: PROJECT_CONFIG })
      }),
    )
    const user = userEvent.setup()
    render(<ProjectSettingsPage />, { wrapper })
    await waitEditorReady()
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
    // 长 YAML 逐字符输入 + 覆盖率模式下 CI 环境慢 → 放宽超时
  }, 15000)

  it('YAML 手写重复流转规则：保存后被重新生成覆盖（每状态一条 from 规则）', async () => {
    let putBody: unknown = null
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/project-config`, async ({ request }) => {
        putBody = await request.json()
        return HttpResponse.json({ code: 0, data: PROJECT_CONFIG })
      }),
    )
    const user = userEvent.setup()
    render(<ProjectSettingsPage />, { wrapper })
    await waitEditorReady()
    const yaml = (await screen.findByLabelText('config.yaml 原文')) as HTMLTextAreaElement
    // 手动塞入重复/乱序 transitions（同 from 两条 + 乱序 to）。
    const edited = yaml.value.replace(
      'transitions:\n      - from: todo',
      'transitions:\n      - from: todo\n      - from: todo\n      - from: doing',
    )
    await user.clear(yaml)
    await user.type(yaml, edited)
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(putBody).not.toBeNull())
    const sm = (putBody as { StateMachine: { Transitions: { From: string }[] } }).StateMachine
    // 每状态恰好一条 from 规则（重新生成覆盖手写）。
    const fromCounts = sm.Transitions.reduce<Record<string, number>>((acc, t) => {
      acc[t.From] = (acc[t.From] ?? 0) + 1
      return acc
    }, {})
    expect(fromCounts).toEqual({ todo: 1, doing: 1, done: 1 })
    // 长 YAML 逐字符输入 + 覆盖率模式下 CI 环境慢 → 放宽超时
  }, 15000)

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
    await waitEditorReady()
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
    await waitEditorReady()
    const labelInput = await screen.findByLabelText('状态 1 名称')
    await user.clear(labelInput)
    await user.type(labelInput, '待办占用中')
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(toastError).toBeCalled())
    expect(String(toastError.mock.calls[0]?.[0])).toContain('任务占用')
    // 422 失败后保留编辑值（CI 慢环境组件回填可能滞后 → waitFor）
    await waitFor(() => expect(screen.getByLabelText('状态 1 名称')).toHaveValue('待办占用中'))
    toastError.mockRestore()
  })

  it('删除状态：本地移除对应行并清理其它行的目标引用', async () => {
    const user = userEvent.setup()
    render(<ProjectSettingsPage />, { wrapper })
    await waitEditorReady()
    await screen.findByLabelText('状态 1 key')
    await user.click(screen.getByRole('button', { name: '删除状态 todo' }))
    const keys = screen.getAllByLabelText(/状态 \d+ key/)
    expect(keys).toHaveLength(2)
    expect(keys[0]).toHaveValue('doing')
    // 删除后保存：PUT Transitions 不再引用 todo。
    let putBody: unknown = null
    server.use(
      http.put(`${DAEMON_BASE_URL}/api/project-config`, async ({ request }) => {
        putBody = await request.json()
        return HttpResponse.json({ code: 0, data: PROJECT_CONFIG })
      }),
    )
    await user.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(putBody).not.toBeNull())
    const bodyStr = JSON.stringify(putBody)
    expect(bodyStr).not.toContain('"todo"')
  })

  it('放弃修改：编辑后点放弃 → 恢复服务端值', async () => {
    const toastInfo = vi.spyOn(toast, 'info').mockImplementation(() => '')
    const user = userEvent.setup()
    render(<ProjectSettingsPage />, { wrapper })
    await waitEditorReady()
    const labelInput = await screen.findByLabelText('状态 1 名称')
    await user.clear(labelInput)
    await user.type(labelInput, '未保存的修改')
    await user.click(screen.getByRole('button', { name: '放弃修改' }))
    expect(screen.getByLabelText('状态 1 名称')).toHaveValue('待办')
    expect(toastInfo).toBeCalled()
    toastInfo.mockRestore()
  })
})

describe('reorderStateRows（拖拽排序纯函数：states/targets/rowIds 同步重排）', () => {
  const states = [
    { Key: 'a', Label: 'A', Color: '#1' },
    { Key: 'b', Label: 'B', Color: '#2' },
    { Key: 'c', Label: 'C', Color: '#3' },
  ]
  const targets = [['b', 'c'], ['a'], []]
  const rowIds = ['r1', 'r2', 'r3']

  it('前移：索引 2 → 0，三者同步移动', () => {
    const next = reorderStateRows(states, targets, rowIds, 2, 0)
    expect(next.states.map((s) => s.Key)).toEqual(['c', 'a', 'b'])
    expect(next.targets).toEqual([[], ['b', 'c'], ['a']])
    expect(next.rowIds).toEqual(['r3', 'r1', 'r2'])
  })

  it('后移：索引 0 → 2', () => {
    const next = reorderStateRows(states, targets, rowIds, 0, 2)
    expect(next.states.map((s) => s.Key)).toEqual(['b', 'c', 'a'])
    expect(next.targets).toEqual([['a'], [], ['b', 'c']])
    expect(next.rowIds).toEqual(['r2', 'r3', 'r1'])
  })

  it('同索引：无变化', () => {
    const next = reorderStateRows(states, targets, rowIds, 1, 1)
    expect(next.states.map((s) => s.Key)).toEqual(['a', 'b', 'c'])
    expect(next.targets).toEqual(targets)
  })
})
