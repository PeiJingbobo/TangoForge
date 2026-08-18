import { describe, expect, it } from 'vitest'
import { graphvizPertLayout } from '@/lib/pert-graphviz-layout'
import { bezierPath, routeCollisionCount, routeCrossingCount } from '@/lib/pert-layout'
import type { PertBezierSegment } from '@/lib/pert-layout'
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

const tangentAlignment = (left: PertBezierSegment, right: PertBezierSegment) => {
  const leftTangent = {
    x: left[3].x - left[2].x,
    y: left[3].y - left[2].y,
  }
  const rightTangent = {
    x: right[1].x - right[0].x,
    y: right[1].y - right[0].y,
  }
  const leftLength = Math.hypot(leftTangent.x, leftTangent.y)
  const rightLength = Math.hypot(rightTangent.x, rightTangent.y)
  return {
    cosine:
      (leftTangent.x * rightTangent.x + leftTangent.y * rightTangent.y) /
      (leftLength * rightLength),
    cross:
      Math.abs(leftTangent.x * rightTangent.y - leftTangent.y * rightTangent.x) /
      (leftLength * rightLength),
  }
}

describe('graphvizPertLayout', () => {
  it('可渲染 ARGUS 规模的单根层级任务，不丢失父子节点和边', async () => {
    const children = Array.from({ length: 122 }, (_, index) => `CHILD-${index + 1}`)
    const layout = await graphvizPertLayout(
      graph(
        ['ROOT', ...children],
        children.map((child) => parent('ROOT', child)),
      ),
    )

    expect(layout.hasCycle).toBe(false)
    expect(layout.nodes).toHaveLength(123)
    expect(layout.edges).toHaveLength(122)
    expect(layout.edges.every((edge) => edge.type === 'parent')).toBe(true)
    expect(layout.nodes.find((node) => node.id === 'ROOT')?.layer).toBe(0)
    expect(
      layout.nodes.filter((node) => node.id !== 'ROOT').every((node) => node.layer === 1),
    ).toBe(true)
  })

  it('输出平滑三次贝塞尔、共享源端点并避开所有非端点节点', async () => {
    const layout = await graphvizPertLayout(
      graph(
        ['A', 'B', 'C', 'D', 'E'],
        [dep('A', 'B'), dep('A', 'C'), dep('B', 'D'), dep('C', 'D'), dep('A', 'E')],
      ),
    )
    const sourceEdges = layout.edges.filter((edge) => edge.from === 'A')

    expect(sourceEdges).toHaveLength(3)
    expect(new Set(sourceEdges.map((edge) => JSON.stringify(edge.segments[0][0]))).size).toBe(1)
    expect(sourceEdges.every((edge) => bezierPath(edge.segments).includes(' C '))).toBe(true)
    expect(sourceEdges.every((edge) => !bezierPath(edge.segments).includes(' L '))).toBe(true)
    for (const edge of layout.edges) {
      const obstacles = layout.nodes.filter((node) => node.id !== edge.from && node.id !== edge.to)
      expect(routeCollisionCount(edge.segments, obstacles, 0)).toBe(0)
      for (let index = 1; index < edge.segments.length; index += 1) {
        const alignment = tangentAlignment(edge.segments[index - 1], edge.segments[index])
        expect(alignment.cosine).toBeGreaterThan(0.999)
        expect(alignment.cross).toBeLessThan(0.001)
      }
    }
  })

  it('通过层内换序消除可避免交叉，并为同层节点保留宽松间距', async () => {
    const layout = await graphvizPertLayout(
      graph(['A', 'B', 'C', 'D'], [dep('A', 'D'), dep('B', 'C')]),
    )
    const targets = layout.nodes.filter((node) => node.layer === 1).sort((a, b) => a.y - b.y)

    expect(routeCrossingCount(layout.edges.map((edge) => edge.segments))).toBe(0)
    expect(targets[1].y - targets[0].y).toBeGreaterThan(100)
    expect(layout.width).toBeGreaterThan(1400)
  })
})
