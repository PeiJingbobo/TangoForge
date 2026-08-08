import { useEffect, useMemo, useRef } from 'react'
import * as d3 from 'd3'
import { cn } from '@/lib/utils'
import type { GraphData, StateMachineState } from '@/types/models'

/**
 * 任务全景图（TF-028）：D3 力导向渲染。
 * - useRef 管理 d3 实例，卸载时 simulation.stop()（无泄漏）；
 * - 节点颜色映射项目状态机 Color（archived 灰）；
 * - 缩放/拖拽；节点 > CLUSTER_THRESHOLD 时按状态聚簇（超阈值前端聚簇）。
 */
const CLUSTER_THRESHOLD = 300

interface SimNode extends d3.SimulationNodeDatum {
  id: string
  label: string
  color: string
  isCluster: boolean
  count?: number
}

type SimLink = d3.SimulationLinkDatum<SimNode>

export interface GraphViewProps {
  data: GraphData
  states: StateMachineState[]
  onSelect: (taskId: string) => void
  className?: string
}

export function GraphView({ data, states, onSelect, className }: GraphViewProps) {
  const svgRef = useRef<SVGSVGElement>(null)
  const onSelectRef = useRef(onSelect)
  onSelectRef.current = onSelect

  const colorMap = useMemo(() => {
    const m: Record<string, string> = { archived: '#9aa0a6' }
    states.forEach((s) => {
      m[s.Key] = s.Color
    })
    return m
  }, [states])

  useEffect(() => {
    const svgEl = svgRef.current
    if (!svgEl) return
    const { nodes, edges } = data
    if (nodes.length === 0) return

    // 超阈值聚簇：按状态聚合为单节点（显示数量）
    const clustered = nodes.length > CLUSTER_THRESHOLD
    const simNodes: SimNode[] = clustered
      ? [...d3.group(nodes, (n) => n.status)].map(([status, group]) => ({
          id: `cluster:${status}`,
          label: status,
          color: colorMap[status] ?? '#9aa0a6',
          isCluster: true,
          count: group.length,
        }))
      : nodes.map((n) => ({
          id: n.id,
          // TF-040：节点标签带简短编号前缀（可读）
          label: n.number ? `${n.number} ${n.title}` : n.title,
          color: colorMap[n.status] ?? '#9aa0a6',
          isCluster: false,
        }))

    const simLinks: SimLink[] = clustered
      ? []
      : edges.map((e) => ({ source: e.from, target: e.to }))

    const width = svgEl.clientWidth || 860
    const height = svgEl.clientHeight || 520

    const svg = d3.select(svgEl)
    svg.selectAll('*').remove()
    const g = svg.append('g')

    const zoom = d3
      .zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.2, 3])
      .on('zoom', (ev) => g.attr('transform', ev.transform))
    svg.call(zoom)

    const simulation = d3
      .forceSimulation<SimNode>(simNodes)
      .force(
        'link',
        d3
          .forceLink<SimNode, SimLink>(simLinks)
          .id((d) => d.id)
          .distance(64)
          .strength(0.4),
      )
      .force('charge', d3.forceManyBody().strength(clustered ? -160 : -240))
      .force('center', d3.forceCenter(width / 2, height / 2))
      .force('collide', d3.forceCollide(clustered ? 42 : 15))

    const link = g
      .append('g')
      .selectAll('line')
      .data(simLinks)
      .join('line')
      .attr('stroke', 'var(--border)')
      .attr('stroke-opacity', 0.65)
      .attr('stroke-width', 1)

    // d3 v7 drag 泛型与 selection.call 签名不兼容（社区常见问题），断言包装
    const dragBehavior = d3
      .drag<Element, SimNode, SimNode>()
      .on('start', (ev, d) => {
        if (!ev.active) simulation.alphaTarget(0.3).restart()
        d.fx = d.x
        d.fy = d.y
      })
      .on('drag', (ev, d) => {
        d.fx = ev.x
        d.fy = ev.y
      })
      .on('end', (ev, d) => {
        if (!ev.active) simulation.alphaTarget(0)
        d.fx = null
        d.fy = null
      })

    const node = g
      .append('g')
      .selectAll('g')
      .data(simNodes)
      .join('g')
      .attr('cursor', 'pointer')
      .on('click', (_ev, d) => {
        if (!d.isCluster) onSelectRef.current(d.id)
      })
      .call(dragBehavior as unknown as (selection: unknown) => void)

    node
      .append('circle')
      .attr('r', (d) => (d.isCluster ? 28 : 7))
      .attr('fill', (d) => d.color)
      .attr('stroke', 'var(--card)')
      .attr('stroke-width', 1.5)

    if (clustered) {
      node
        .append('text')
        .text((d) => String(d.count ?? ''))
        .attr('text-anchor', 'middle')
        .attr('dy', 4)
        .attr('font-size', 12)
        .attr('font-weight', 700)
        .attr('fill', 'var(--card)')
    } else {
      node
        .append('text')
        .text((d) => d.label)
        .attr('x', 10)
        .attr('dy', 4)
        .attr('font-size', 10)
        .attr('fill', 'var(--muted-foreground)')
        .attr('pointer-events', 'none')
    }

    simulation.on('tick', () => {
      link
        .attr('x1', (d) => (d.source as SimNode).x ?? 0)
        .attr('y1', (d) => (d.source as SimNode).y ?? 0)
        .attr('x2', (d) => (d.target as SimNode).x ?? 0)
        .attr('y2', (d) => (d.target as SimNode).y ?? 0)
      node.attr('transform', (d) => `translate(${d.x ?? 0},${d.y ?? 0})`)
    })

    return () => {
      simulation.stop()
      svg.on('.zoom', null)
    }
  }, [data, colorMap])

  return (
    <svg
      ref={svgRef}
      className={cn('h-[520px] w-full rounded-2xl border border-divider bg-card', className)}
      aria-label="任务全景图"
      role="img"
    />
  )
}
