import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ZoomTransform } from 'd3'
import { PertView } from './pert-view'
import type { GraphData, StateMachineState } from '@/types/models'

const STATES: StateMachineState[] = [
  { Key: 'todo', Label: '待办', Color: '#999999' },
  { Key: 'doing', Label: '进行中', Color: '#2292d8' },
  { Key: 'done', Label: '已完成', Color: '#22c55e' },
]

function node(id: string, status = 'todo') {
  return {
    id,
    project_id: 1,
    parent_id: null,
    title: `任务 ${id}`,
    number: `TF-${id}`,
    description: '',
    status,
    priority: 0,
    tags: [],
    assignee: '',
    depends_on: [],
    archived_from: '',
    source_file: '',
    source_section: '',
    created_at: '2026-08-18T10:00:00+08:00',
    updated_at: '2026-08-18T10:00:00+08:00',
  }
}

const DATA: GraphData = {
  nodes: [node('ROOT'), node('A', 'doing'), node('B', 'done'), node('C'), node('SIDE')],
  edges: [
    { from: 'ROOT', to: 'A', type: 'dependency' },
    { from: 'A', to: 'B', type: 'dependency' },
    { from: 'B', to: 'C', type: 'dependency' },
    { from: 'SIDE', to: 'C', type: 'dependency' },
    { from: 'ROOT', to: 'C', type: 'parent' },
  ],
}

describe('PertView 工作流画布', () => {
  it('以圆形节点渲染并只展示 dependency 边', () => {
    render(<PertView data={DATA} states={STATES} onSelect={() => {}} />)
    expect(document.querySelectorAll('[data-role="pert-nodes"] > g')).toHaveLength(5)
    expect(document.querySelectorAll('[data-role="pert-nodes"] > g > circle')).toHaveLength(5)
    expect(document.querySelectorAll('[data-role="pert-edges"] > g')).toHaveLength(4)
  })

  it('每条边有透明宽命中层，可通过鼠标选中', () => {
    render(<PertView data={DATA} states={STATES} onSelect={() => {}} />)
    const edge = screen.getByRole('button', { name: '选择依赖 TF-A 到 TF-B' })
    expect(edge).toHaveAttribute('stroke', 'transparent')
    expect(edge).toHaveAttribute('stroke-width', '16')
    expect(edge).toHaveStyle({ outline: 'none' })
    fireEvent.click(edge)
    expect(screen.getByRole('status')).toHaveTextContent('已选择 TF-A → TF-B')
    expect(document.querySelector('[data-edge-id="A->B"]')).toHaveAttribute(
      'data-state',
      'selected',
    )
  })

  it('连接线为加粗贝塞尔曲线，箭头随画布缩放且根谱系颜色稳定', () => {
    render(<PertView data={DATA} states={STATES} onSelect={() => {}} />)
    const rootEdge = document.querySelector('[data-edge-id="ROOT->A"] [data-role="visible-edge"]')!
    const descendantEdge = document.querySelector(
      '[data-edge-id="A->B"] [data-role="visible-edge"]',
    )!
    const otherRootEdge = document.querySelector(
      '[data-edge-id="SIDE->C"] [data-role="visible-edge"]',
    )!
    expect(rootEdge.getAttribute('d')).toContain(' C ')
    expect(rootEdge).toHaveAttribute('stroke-width', '2.5')
    expect(rootEdge).not.toHaveAttribute('vector-effect')
    expect(rootEdge.getAttribute('stroke')).toBe(descendantEdge.getAttribute('stroke'))
    expect(rootEdge.getAttribute('stroke')).not.toBe(otherRootEdge.getAttribute('stroke'))
    expect(document.querySelector('marker')).toHaveAttribute('markerUnits', 'userSpaceOnUse')
  })

  it('画布默认使用抓手，连接线命中区域 hover 时使用普通箭头', () => {
    render(<PertView data={DATA} states={STATES} onSelect={() => {}} />)

    expect(screen.getByLabelText('PERT 任务图').querySelector('svg')).toHaveClass('cursor-grab')
    expect(screen.getByRole('button', { name: '选择依赖 TF-A 到 TF-B' })).toHaveClass(
      'cursor-default',
    )
  })

  it('选中边后高亮上游和下游，旁路节点与边降噪', () => {
    render(<PertView data={DATA} states={STATES} onSelect={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: '选择依赖 TF-A 到 TF-B' }))
    expect(document.querySelector('[data-edge-id="ROOT->A"]')).toHaveAttribute(
      'data-state',
      'related',
    )
    expect(document.querySelector('[data-edge-id="B->C"]')).toHaveAttribute('data-state', 'related')
    expect(document.querySelector('[data-edge-id="SIDE->C"]')).toHaveAttribute(
      'data-state',
      'dimmed',
    )
    const dimmedEdge = document.querySelector(
      '[data-edge-id="SIDE->C"] [data-role="visible-edge"]',
    )!
    const relatedEdge = document.querySelector(
      '[data-edge-id="ROOT->A"] [data-role="visible-edge"]',
    )!
    expect(dimmedEdge.getAttribute('marker-end')).toContain('-dimmed)')
    expect(relatedEdge.getAttribute('marker-end')).toContain('-active)')
    const dimmedMarkerId = dimmedEdge.getAttribute('marker-end')!.match(/#([^)]*)/)![1]
    expect(document.querySelector(`#${dimmedMarkerId} path`)).toHaveAttribute(
      'fill-opacity',
      '0.08',
    )
    expect(screen.getByRole('button', { name: 'TF-SIDE 任务 SIDE' })).toHaveAttribute(
      'opacity',
      '0.16',
    )
  })

  it('边支持键盘选择与再次取消', () => {
    render(<PertView data={DATA} states={STATES} onSelect={() => {}} />)
    const edge = screen.getByRole('button', { name: '选择依赖 TF-A 到 TF-B' })
    fireEvent.keyDown(edge, { key: 'Enter' })
    expect(screen.getByRole('status')).toBeInTheDocument()
    fireEvent.keyDown(edge, { key: ' ' })
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('单击和双击圆形节点保持原有任务导航行为', async () => {
    const onSelect = vi.fn()
    const onOpenFull = vi.fn()
    render(<PertView data={DATA} states={STATES} onSelect={onSelect} onOpenFull={onOpenFull} />)
    const task = screen.getByRole('button', { name: 'TF-A 任务 A' })
    fireEvent.click(task)
    await waitFor(() => expect(onSelect).toHaveBeenCalledWith('A'), { timeout: 1000 })

    onSelect.mockClear()
    fireEvent.click(task)
    fireEvent.dblClick(task)
    expect(onOpenFull).toHaveBeenCalledWith('A')
    await new Promise((resolve) => setTimeout(resolve, 300))
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('绑定无限视口式缩放平移行为', () => {
    render(<PertView data={DATA} states={STATES} onSelect={() => {}} />)
    const svg = document.querySelector<SVGSVGElement>('[aria-label="PERT 任务图"] > svg')!
    expect(svg).toHaveClass('touch-none')
    expect((svg as SVGSVGElement & { __zoom?: unknown }).__zoom).toBeDefined()
    expect(screen.getByLabelText('PERT 任务图')).toHaveAttribute('data-viewport', 'infinite')
    expect(screen.getByLabelText('PERT 任务图')).toHaveClass('h-[calc(100vh-15rem)]')
  })

  it('大图首次加载保持节点可读尺度，点阵随无限视口同步平移缩放', async () => {
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      top: 0,
      left: 0,
      right: 1200,
      bottom: 620,
      width: 1200,
      height: 620,
      toJSON: () => ({}),
    })
    try {
      render(<PertView data={DATA} states={STATES} onSelect={() => {}} />)
      const svg = document.querySelector<SVGSVGElement>('[aria-label="PERT 任务图"] > svg')!
      await waitFor(() =>
        expect((svg as SVGSVGElement & { __zoom: ZoomTransform }).__zoom.k).toBeCloseTo(0.58),
      )
      expect(document.querySelector('#pert-grid')).toHaveAttribute(
        'patternTransform',
        expect.stringContaining('scale(0.58)'),
      )
    } finally {
      rectSpy.mockRestore()
    }
  })

  it('搜索后只高亮命中节点的上游和下游路径', async () => {
    render(<PertView data={DATA} states={STATES} onSelect={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: '选择依赖 TF-SIDE 到 TF-C' }))
    expect(screen.getByRole('status')).toBeInTheDocument()
    fireEvent.change(screen.getByRole('textbox', { name: '搜索任务' }), {
      target: { value: 'TF-B' },
    })
    expect(screen.queryByRole('status')).not.toBeInTheDocument()

    await waitFor(
      () =>
        expect(document.querySelector('[data-edge-id="ROOT->A"]')).toHaveAttribute(
          'data-state',
          'related',
        ),
      { timeout: 1000 },
    )
    expect(document.querySelector('[data-edge-id="A->B"]')).toHaveAttribute('data-state', 'related')
    expect(document.querySelector('[data-edge-id="B->C"]')).toHaveAttribute('data-state', 'related')
    expect(document.querySelector('[data-edge-id="SIDE->C"]')).toHaveAttribute(
      'data-state',
      'dimmed',
    )
    expect(
      document.querySelector('[data-edge-id="SIDE->C"] [data-role="visible-edge"]'),
    ).toHaveAttribute('stroke-opacity', '0.08')
    const matchedNode = screen.getByRole('button', { name: 'TF-B 任务 B' })
    const marker = matchedNode.querySelector('[data-role="search-result-marker"]')!
    const nodeBody = matchedNode.querySelector('circle:not([data-role])')!
    expect(marker).toHaveAttribute('fill', 'none')
    expect(marker).toHaveAttribute('stroke', 'var(--primary)')
    expect(Number(marker.getAttribute('r')) - Number(nodeBody.getAttribute('r'))).toBe(11)
    expect(document.querySelectorAll('[data-role="search-result-marker"]')).toHaveLength(1)
  })

  it('点击画布背景清除搜索高亮但保留查询和结果标记', async () => {
    render(<PertView data={DATA} states={STATES} onSelect={() => {}} />)
    const input = screen.getByRole('textbox', { name: '搜索任务' })
    fireEvent.change(input, { target: { value: 'TF-B' } })
    await waitFor(
      () =>
        expect(document.querySelector('[data-edge-id="SIDE->C"]')).toHaveAttribute(
          'data-state',
          'dimmed',
        ),
      { timeout: 1000 },
    )

    fireEvent.click(document.querySelector('[data-role="canvas-background"]')!)

    expect(document.querySelector('[data-edge-id="ROOT->A"]')).toHaveAttribute(
      'data-state',
      'normal',
    )
    expect(document.querySelector('[data-edge-id="SIDE->C"]')).toHaveAttribute(
      'data-state',
      'normal',
    )
    expect(screen.getByRole('button', { name: 'TF-SIDE 任务 SIDE' })).toHaveAttribute(
      'opacity',
      '1',
    )
    expect(input).toHaveValue('TF-B')
    expect(
      screen
        .getByRole('button', { name: 'TF-B 任务 B' })
        .querySelector('[data-role="search-result-marker"]'),
    ).toBeInTheDocument()
  })

  it('搜索完成后自动缩放并居中展示命中节点', async () => {
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      top: 0,
      left: 0,
      right: 1200,
      bottom: 620,
      width: 1200,
      height: 620,
      toJSON: () => ({}),
    })
    try {
      render(<PertView data={DATA} states={STATES} onSelect={() => {}} />)
      fireEvent.change(screen.getByRole('textbox', { name: '搜索任务' }), {
        target: { value: 'TF-B' },
      })
      const svg = document.querySelector<SVGSVGElement>('[aria-label="PERT 任务图"] > svg')!
      await waitFor(
        () => expect((svg as SVGSVGElement & { __zoom: ZoomTransform }).__zoom.k).toBeCloseTo(1.65),
        { timeout: 1000 },
      )
      const nodeElement = screen.getByRole('button', { name: 'TF-B 任务 B' })
      const [, rawX, rawY] = nodeElement
        .getAttribute('transform')!
        .match(/translate\(([-\d.]+), ([-\d.]+)\)/)!
      const viewportPoint = (svg as SVGSVGElement & { __zoom: ZoomTransform }).__zoom.apply([
        Number(rawX),
        Number(rawY),
      ])
      expect(viewportPoint[0]).toBeCloseTo(600)
      expect(viewportPoint[1]).toBeGreaterThan(250)
      expect(viewportPoint[1]).toBeLessThan(330)
    } finally {
      rectSpy.mockRestore()
    }
  })

  it('循环依赖显示错误且不渲染脏图', () => {
    const cycle: GraphData = {
      nodes: [node('A'), node('B')],
      edges: [
        { from: 'A', to: 'B', type: 'dependency' },
        { from: 'B', to: 'A', type: 'dependency' },
      ],
    }
    render(<PertView data={cycle} states={STATES} onSelect={() => {}} />)
    expect(screen.getByRole('alert')).toHaveTextContent('循环依赖')
    expect(document.querySelectorAll('[data-role="pert-nodes"] > g')).toHaveLength(0)
  })
})
