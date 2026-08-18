import { describe, expect, it } from 'vitest'
import {
  assignVerticalCoordinates,
  bezierPath,
  obstacleAvoidingBezierRoute,
  pertEdgeId,
  pertLayout,
  routeCollisionCount,
  routeCrossingCount,
  routeOverlapPenalty,
  tracePertEdge,
  tracePertNodes,
} from '@/lib/pert-layout'
import type { GraphData, GraphEdge } from '@/types/models'
import type { Task } from '@/types/task'

const task = (id: string): Task => ({
  id,
  project_id: 1,
  parent_id: null,
  title: `任务 ${id}`,
  number: `TF-${id}`,
  description: '',
  status: 'todo',
  priority: 0,
  tags: [],
  assignee: '',
  depends_on: [],
  archived_from: '',
  source_file: '',
  source_section: '',
  created_at: '2026-08-18T10:00:00+08:00',
  updated_at: '2026-08-18T10:00:00+08:00',
})
const dep = (from: string, to: string): GraphEdge => ({ from, to, type: 'dependency' })
const parent = (from: string, to: string): GraphEdge => ({ from, to, type: 'parent' })
const graph = (ids: string[], edges: GraphEdge[]): GraphData => ({ nodes: ids.map(task), edges })

describe('pertLayout 分层和路由', () => {
  it('链式依赖严格从左到右分层', () => {
    const layout = pertLayout(
      graph(['A', 'B', 'C', 'D'], [dep('A', 'B'), dep('B', 'C'), dep('C', 'D')]),
    )
    expect(Object.fromEntries(layout.nodes.map((node) => [node.id, node.layer]))).toEqual({
      A: 0,
      B: 1,
      C: 2,
      D: 3,
    })
    expect(layout.nodes.every((node) => node.radius === 32)).toBe(true)
  })

  it('菱形依赖把 B/C 放在同层且节点不重叠', () => {
    const layout = pertLayout(
      graph(['A', 'B', 'C', 'D'], [dep('A', 'B'), dep('A', 'C'), dep('B', 'D'), dep('C', 'D')]),
    )
    expect(layout.nodes.find((node) => node.id === 'B')?.layer).toBe(1)
    expect(layout.nodes.find((node) => node.id === 'C')?.layer).toBe(1)
    expect(layout.nodes.find((node) => node.id === 'D')?.layer).toBe(2)
    const layerOneY = layout.nodes.filter((node) => node.layer === 1).map((node) => node.y)
    expect(new Set(layerOneY).size).toBe(2)
  })

  it('末端单依赖节点在纵向上贴近唯一父节点', () => {
    const layout = pertLayout(
      graph(
        ['ROOT-A', 'ROOT-B', 'ROOT-C', 'A1', 'B1', 'C1', 'END'],
        [dep('ROOT-A', 'A1'), dep('ROOT-B', 'B1'), dep('ROOT-C', 'C1'), dep('A1', 'END')],
      ),
    )
    const positions = new Map(layout.nodes.map((node) => [node.id, node]))
    expect(Math.abs(positions.get('END')!.y - positions.get('A1')!.y)).toBeLessThan(0.001)
  })

  it('纵向松弛按父子位置重排并保持层内最小间距', () => {
    const layers = [
      ['ROOT-A', 'ROOT-B'],
      ['A', 'B'],
    ]
    const preds = new Map<string, string[]>([
      ['ROOT-A', []],
      ['ROOT-B', []],
      ['A', ['ROOT-B']],
      ['B', ['ROOT-A']],
    ])
    const succs = new Map<string, string[]>([
      ['ROOT-A', ['B']],
      ['ROOT-B', ['A']],
      ['A', []],
      ['B', []],
    ])
    const y = assignVerticalCoordinates(layers, preds, succs, 200, 100)
    for (const layer of layers) {
      for (let index = 1; index < layer.length; index++) {
        expect(y.get(layer[index])! - y.get(layer[index - 1])!).toBeGreaterThanOrEqual(200)
      }
    }
    expect(layers[1]).toEqual(['B', 'A'])
  })

  it('通过层内换序消除可避免的连接线交叉', () => {
    const layout = pertLayout(graph(['A', 'B', 'C', 'D'], [dep('A', 'D'), dep('B', 'C')]))
    const positions = new Map(layout.nodes.map((node) => [node.id, node]))
    const sourceDelta = positions.get('A')!.y - positions.get('B')!.y
    const targetDelta = positions.get('D')!.y - positions.get('C')!.y
    expect(sourceDelta * targetDelta).toBeGreaterThan(0)
  })

  it('同源多边共享完全相同的初始端点与贝塞尔初始切线', () => {
    const layout = pertLayout(graph(['A', 'B', 'C'], [dep('A', 'B'), dep('A', 'C')]))
    const [first, second] = layout.edges.filter((edge) => edge.from === 'A')
    expect(first.segments[0][0]).toEqual(second.segments[0][0])
    expect(first.segments[0][1]).toEqual(second.segments[0][1])
    expect(first.segments[0][0].y).toBe(first.segments[0][1].y)
    expect(first.segments[0][1].x).toBeGreaterThan(first.segments[0][0].x)
  })

  it('依赖使用三次贝塞尔曲线且节点具有确定性错位', () => {
    const layout = pertLayout(graph(['A', 'B', 'C'], [dep('A', 'B'), dep('A', 'C')]))
    const routed = layout.edges.find((edge) => edge.to === 'C')!
    expect(routed.segments[0]).toHaveLength(4)
    expect(bezierPath(routed.segments)).toMatch(/^M .* C /)
    const sameLayer = layout.nodes.filter((node) => node.layer === 1)
    expect(sameLayer[0].x).not.toBe(sameLayer[1].x)
  })

  it('扩大节点间距并确保所有路径避开非端点节点排斥区', () => {
    const layout = pertLayout(
      graph(
        ['A', 'B', 'C', 'D', 'E', 'F', 'G'],
        [
          dep('A', 'D'),
          dep('A', 'E'),
          dep('B', 'D'),
          dep('B', 'F'),
          dep('C', 'E'),
          dep('D', 'G'),
          dep('E', 'G'),
          dep('F', 'G'),
          dep('A', 'G'),
        ],
      ),
    )
    const nodes = new Map(layout.nodes.map((node) => [node.id, node]))
    const sameLayer = layout.nodes.filter((node) => node.layer === 1).sort((a, b) => a.y - b.y)
    expect(sameLayer[1].y - sameLayer[0].y).toBeGreaterThan(150)
    for (const edge of layout.edges) {
      const obstacles = layout.nodes.filter((node) => node.id !== edge.from && node.id !== edge.to)
      expect(routeCollisionCount(edge.segments, obstacles)).toBe(0)
      expect(nodes.get(edge.to)!.x).toBeGreaterThan(nodes.get(edge.from)!.x)
    }
    expect(layout.edges.find((edge) => edge.id === pertEdgeId('A', 'G'))?.routed).toBe(true)
  })

  it('直线路径遇到节点时优先以单段宽缓曲线避让', () => {
    const obstacle = {
      id: 'BLOCK',
      title: '阻挡节点',
      number: 'TF-BLOCK',
      status: 'todo',
      layer: 1,
      indexInLayer: 0,
      x: 220,
      y: 100,
      radius: 32,
      predIds: [],
      rootId: 'BLOCK',
    }
    const segments = obstacleAvoidingBezierRoute(
      { x: 0, y: 100 },
      { x: 440, y: 100 },
      { x: 50, y: 100 },
      [obstacle],
    )
    expect(segments).toHaveLength(1)
    expect(routeCollisionCount(segments, [obstacle])).toBe(0)
    expect(bezierPath(segments).match(/ C /g)).toHaveLength(1)
  })

  it('重叠候选会分配不同曲率并显著降低路径近距离重合', () => {
    const start = { x: 0, y: 120 }
    const end = { x: 900, y: 420 }
    const branch = { x: 50, y: 120 }
    const first = obstacleAvoidingBezierRoute(start, end, branch, [], 50, 20, true)
    const second = obstacleAvoidingBezierRoute(start, end, branch, [], 50, 20, true, [first])
    const identicalPenalty = routeOverlapPenalty(first, [first])
    const separatedPenalty = routeOverlapPenalty(second, [first])

    expect(bezierPath(second)).not.toBe(bezierPath(first))
    expect(separatedPenalty).toBeLessThan(identicalPenalty * 0.25)
  })

  it('长边会绕到内部节点与既有路径外侧，并保持分段贝塞尔切线连续', () => {
    const occupied = [100, 160, 220, 280, 340].map(
      (y): Parameters<typeof routeOverlapPenalty>[1][number] => [
        [
          { x: 120, y },
          { x: 360, y },
          { x: 640, y },
          { x: 880, y },
        ],
      ],
    )
    const obstacles = [100, 160, 220, 280, 340].map((y, index) => ({
      id: `BLOCK-${index}`,
      title: '阻挡节点',
      number: `TF-BLOCK-${index}`,
      status: 'todo',
      layer: 1,
      indexInLayer: index,
      x: 500,
      y,
      radius: 32,
      predIds: [],
      rootId: `BLOCK-${index}`,
    }))
    const route = obstacleAvoidingBezierRoute(
      { x: 0, y: 220 },
      { x: 1000, y: 220 },
      { x: 50, y: 220 },
      obstacles,
      50,
      20,
      true,
      occupied,
    )
    const routeY = route.flat().map((point) => point.y)

    expect(routeCollisionCount(route, obstacles)).toBe(0)
    expect(Math.min(...routeY) < 82 || Math.max(...routeY) > 358).toBe(true)
    for (let index = 0; index < route.length - 1; index++) {
      const current = route[index]
      const next = route[index + 1]
      const join = current[3]
      const incoming = { x: join.x - current[2].x, y: join.y - current[2].y }
      const outgoing = { x: next[1].x - join.x, y: next[1].y - join.y }
      expect(join).toEqual(next[0])
      expect(Math.abs(incoming.x * outgoing.y - incoming.y * outgoing.x)).toBeLessThan(0.001)
      expect(incoming.x * outgoing.x + incoming.y * outgoing.y).toBeGreaterThan(0)
    }
  })

  it('不同根任务拥有不同谱系，后代出边继承根谱系', () => {
    const layout = pertLayout(
      graph(
        ['ROOT-A', 'A1', 'A2', 'ROOT-B', 'B1'],
        [dep('ROOT-A', 'A1'), dep('A1', 'A2'), dep('ROOT-B', 'B1')],
      ),
    )
    const rootAEdges = layout.edges.filter((edge) => edge.from === 'ROOT-A' || edge.from === 'A1')
    expect(new Set(rootAEdges.map((edge) => edge.rootId))).toEqual(new Set(['ROOT-A']))
    expect(layout.edges.find((edge) => edge.from === 'ROOT-B')?.rootId).toBe('ROOT-B')
  })

  it('默认同时布局 parent 与 dependency，重复关系优先依赖语义并忽略悬空边', () => {
    const layout = pertLayout(
      graph(['A', 'B'], [parent('A', 'B'), dep('A', 'B'), dep('A', 'B'), dep('ghost', 'B')]),
    )
    expect(layout.edges).toHaveLength(1)
    expect(layout.edges[0].id).toBe(pertEdgeId('A', 'B'))
    expect(layout.edges[0].type).toBe('dependency')
  })

  it('纯父子任务树按层级从左向右展示', () => {
    const layout = pertLayout(
      graph(
        ['ROOT', 'CHILD-A', 'CHILD-B', 'GRANDCHILD'],
        [parent('ROOT', 'CHILD-A'), parent('ROOT', 'CHILD-B'), parent('CHILD-A', 'GRANDCHILD')],
      ),
    )
    expect(Object.fromEntries(layout.nodes.map((node) => [node.id, node.layer]))).toEqual({
      ROOT: 0,
      'CHILD-A': 1,
      'CHILD-B': 1,
      GRANDCHILD: 2,
    })
    expect(layout.edges).toHaveLength(3)
    expect(layout.edges.every((edge) => edge.type === 'parent')).toBe(true)
  })

  it('空图返回稳定的空布局', () => {
    expect(pertLayout({ nodes: [], edges: [] })).toMatchObject({
      nodes: [],
      edges: [],
      width: 0,
      height: 0,
      hasCycle: false,
    })
  })

  it('循环依赖被拒绝渲染', () => {
    const layout = pertLayout(graph(['A', 'B', 'C'], [dep('A', 'B'), dep('B', 'C'), dep('C', 'A')]))
    expect(layout.hasCycle).toBe(true)
    expect(new Set(layout.cycle)).toEqual(new Set(['A', 'B', 'C']))
  })
})

describe('bezierPath', () => {
  it('输出由画布一同缩放的三次贝塞尔 SVG 路径', () => {
    expect(
      bezierPath([
        [
          { x: 0, y: 0 },
          { x: 20, y: 0 },
          { x: 30, y: 40 },
          { x: 50, y: 40 },
        ],
      ]),
    ).toBe('M 0 0 C 20 0 30 40 50 40')
  })

  it('区分内部交叉与互不相交的平滑路径', () => {
    const rising = [
      [
        { x: 0, y: 0 },
        { x: 30, y: 0 },
        { x: 70, y: 100 },
        { x: 100, y: 100 },
      ],
    ] as Parameters<typeof routeCrossingCount>[0][number]
    const falling = [
      [
        { x: 0, y: 100 },
        { x: 30, y: 100 },
        { x: 70, y: 0 },
        { x: 100, y: 0 },
      ],
    ] as Parameters<typeof routeCrossingCount>[0][number]
    const parallel = rising.map((segment) =>
      segment.map((point) => ({ x: point.x, y: point.y + 160 })),
    ) as Parameters<typeof routeCrossingCount>[0][number]
    expect(routeCrossingCount([rising, falling])).toBe(1)
    expect(routeCrossingCount([rising, parallel])).toBe(0)
  })
})

describe('tracePertEdge 路径高亮', () => {
  const layout = pertLayout(
    graph(
      ['ROOT', 'A', 'B', 'C', 'D', 'SIDE'],
      [dep('ROOT', 'A'), dep('A', 'B'), dep('B', 'C'), dep('C', 'D'), dep('SIDE', 'D')],
    ),
  )

  it('选择 B→C 后包含源端全部上游与目标端全部下游', () => {
    const trace = tracePertEdge(layout.edges, pertEdgeId('B', 'C'))!
    expect(trace.upstreamNodeIds).toEqual(new Set(['B', 'A', 'ROOT']))
    expect(trace.downstreamNodeIds).toEqual(new Set(['C', 'D']))
    expect(trace.edgeIds).toEqual(
      new Set([
        pertEdgeId('ROOT', 'A'),
        pertEdgeId('A', 'B'),
        pertEdgeId('B', 'C'),
        pertEdgeId('C', 'D'),
      ]),
    )
    expect(trace.nodeIds.has('SIDE')).toBe(false)
  })

  it('不存在的边返回 null', () => {
    expect(tracePertEdge(layout.edges, 'missing')).toBeNull()
  })
})

describe('tracePertNodes 搜索路径高亮', () => {
  const layout = pertLayout(
    graph(
      ['ROOT', 'A', 'B', 'C', 'D', 'SIDE'],
      [dep('ROOT', 'A'), dep('A', 'B'), dep('B', 'C'), dep('C', 'D'), dep('SIDE', 'D')],
    ),
  )

  it('只包含命中节点的上游溯源和下游可达边，不吸附后代的旁路依赖', () => {
    const trace = tracePertNodes(layout.edges, new Set(['B']))!
    expect(trace.upstreamNodeIds).toEqual(new Set(['B', 'A', 'ROOT']))
    expect(trace.downstreamNodeIds).toEqual(new Set(['B', 'C', 'D']))
    expect(trace.edgeIds).toEqual(
      new Set([
        pertEdgeId('ROOT', 'A'),
        pertEdgeId('A', 'B'),
        pertEdgeId('B', 'C'),
        pertEdgeId('C', 'D'),
      ]),
    )
    expect(trace.edgeIds.has(pertEdgeId('SIDE', 'D'))).toBe(false)
  })

  it('多个搜索结果合并各自相关路径，空结果不产生追踪', () => {
    const trace = tracePertNodes(layout.edges, new Set(['A', 'SIDE']))!
    expect(trace.edgeIds.has(pertEdgeId('ROOT', 'A'))).toBe(true)
    expect(trace.edgeIds.has(pertEdgeId('SIDE', 'D'))).toBe(true)
    expect(tracePertNodes(layout.edges, new Set())).toBeNull()
  })
})
