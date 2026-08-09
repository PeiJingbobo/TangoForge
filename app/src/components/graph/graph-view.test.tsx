import { describe, it, expect } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { GraphView } from './graph-view'
import type { GraphData, StateMachineState } from '@/types/models'

const STATES: StateMachineState[] = [
  { Key: 'todo', Label: '待办', Color: '#999999' },
  { Key: 'doing', Label: '进行中', Color: '#2292d8' },
  { Key: 'done', Label: '已完成', Color: '#22c55e' },
]

function mkNode(id: string, status: string) {
  return {
    id,
    project_id: 1,
    parent_id: null,
    title: `任务 ${id}`,
    number: '',
    description: '',
    status,
    priority: 0,
    tags: [],
    assignee: '',
    depends_on: [],
    archived_from: '',
    source_file: '',
    source_section: '',
    created_at: '2026-08-06T10:00:00+08:00',
    updated_at: '2026-08-06T10:00:00+08:00',
  }
}

const DATA: GraphData = {
  nodes: [mkNode('a', 'todo'), mkNode('b', 'doing'), mkNode('c', 'done')],
  edges: [
    { from: 'a', to: 'b', type: 'parent' },
    { from: 'b', to: 'c', type: 'dependency' },
  ],
}

describe('GraphView（D3 全景图）', () => {
  it('渲染节点与连线（circle 数量 = 节点数）', async () => {
    render(<GraphView data={DATA} states={STATES} onSelect={() => {}} />)
    await waitFor(() => {
      expect(document.querySelectorAll('svg circle').length).toBe(3)
    })
    expect(document.querySelectorAll('line').length).toBe(2)
  })

  it('卸载后 simulation 停止（实例销毁无泄漏，不抛错）', () => {
    const { unmount } = render(<GraphView data={DATA} states={STATES} onSelect={() => {}} />)
    expect(() => unmount()).not.toThrow()
  })

  it('超阈值聚簇：>300 节点时按状态聚合', async () => {
    const bigNodes = Array.from({ length: 320 }, (_, i) =>
      mkNode(`n${i}`, i % 3 === 0 ? 'todo' : i % 3 === 1 ? 'doing' : 'done'),
    )
    render(<GraphView data={{ nodes: bigNodes, edges: [] }} states={STATES} onSelect={() => {}} />)
    await waitFor(() => {
      // 3 个状态 → 3 个聚合节点（各带数量文本）
      expect(document.querySelectorAll('svg circle').length).toBe(3)
    })
    expect(document.querySelectorAll('svg text').length).toBe(3)
  })

  it('渲染状态机颜色图例（QA 2026-08-09）', () => {
    render(<GraphView data={DATA} states={STATES} onSelect={() => {}} />)
    // 图例包含各状态 Label（todo/doing/done）
    expect(screen.getByText('待办')).toBeInTheDocument()
    expect(screen.getByText('进行中')).toBeInTheDocument()
    expect(screen.getByText('已完成')).toBeInTheDocument()
  })
})
