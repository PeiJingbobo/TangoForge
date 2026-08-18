import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import * as d3 from 'd3'
import { Maximize2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PertLegend } from '@/components/graph/pert-legend'
import { cn } from '@/lib/utils'
import {
  bezierPath,
  pertLayout,
  selectPertPrimaryRoot,
  tracePertEdge,
  tracePertNodes,
  type PertEdge,
} from '@/lib/pert-layout'
import type { GraphData, StateMachineState } from '@/types/models'

const CLICK_DELAY_MS = 220
const ARCHIVED_COLOR = '#9aa0a6'
const DEFAULT_VIEW_SCALE = 0.58
interface PertViewProps {
  data: GraphData
  states: StateMachineState[]
  onSelect: (taskId: string) => void
  onOpenFull?: (taskId: string) => void
  className?: string
}

export function PertView({ data, states, onSelect, onOpenFull, className }: PertViewProps) {
  const svgRef = useRef<SVGSVGElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const zoomRef = useRef<d3.ZoomBehavior<SVGSVGElement, unknown> | null>(null)
  const clickTimerRef = useRef<number | null>(null)
  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [selectedEdgeId, setSelectedEdgeId] = useState<string | null>(null)
  const [searchHighlightActive, setSearchHighlightActive] = useState(false)

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedSearch(search), 300)
    return () => window.clearTimeout(timer)
  }, [search])

  const colorMap = useMemo(() => {
    const colors: Record<string, string> = { archived: ARCHIVED_COLOR }
    states.forEach((state) => {
      colors[state.Key] = state.Color
    })
    return colors
  }, [states])

  const fallbackLayout = useMemo(() => pertLayout(data), [data])
  const [layout, setLayout] = useState(fallbackLayout)
  useEffect(() => {
    let active = true
    setLayout(fallbackLayout)
    if (fallbackLayout.hasCycle || fallbackLayout.nodes.length === 0) return () => undefined
    void import('@/lib/pert-graphviz-layout')
      .then(({ graphvizPertLayout }) => graphvizPertLayout(data))
      .then((nextLayout) => {
        if (active) setLayout(nextLayout)
      })
      .catch(() => {
        // Graphviz WASM 加载或布局失败时保留确定性的本地 Sugiyama 兜底。
      })
    return () => {
      active = false
    }
  }, [data, fallbackLayout])
  const nodeMap = useMemo(
    () => new Map(layout.nodes.map((node) => [node.id, node])),
    [layout.nodes],
  )
  const rootStyleMap = useMemo(() => {
    const rootIds = [...new Set(layout.nodes.map((node) => node.rootId))].sort()
    return new Map(
      rootIds.map((rootId, index) => [
        rootId,
        { color: rootLineColor(index), markerPrefix: `pert-arrow-${index}` },
      ]),
    )
  }, [layout.nodes])
  const selectedEdge = useMemo(
    () => layout.edges.find((edge) => edge.id === selectedEdgeId) ?? null,
    [layout.edges, selectedEdgeId],
  )
  const trace = useMemo(
    () => (selectedEdgeId ? tracePertEdge(layout.edges, selectedEdgeId) : null),
    [layout.edges, selectedEdgeId],
  )
  const matched = useMemo(() => {
    const query = debouncedSearch.trim().toLowerCase()
    if (!query) return null
    return new Set(
      layout.nodes
        .filter(
          (node) =>
            node.title.toLowerCase().includes(query) || node.number.toLowerCase().includes(query),
        )
        .map((node) => node.id),
    )
  }, [debouncedSearch, layout.nodes])
  const searchTrace = useMemo(
    () => (matched ? tracePertNodes(layout.edges, matched) : null),
    [layout.edges, matched],
  )

  useEffect(() => {
    setSearchHighlightActive(Boolean(matched?.size))
  }, [matched])

  const fitBounds = useCallback(
    (
      bounds: { minX: number; minY: number; maxX: number; maxY: number },
      maxScale: number,
      animate: boolean,
    ) => {
      const svg = svgRef.current
      const container = containerRef.current
      const zoom = zoomRef.current
      if (!svg || !container || !zoom) return
      const rect = container.getBoundingClientRect()
      if (rect.width === 0 || rect.height === 0) return
      const padding = 48
      const width = Math.max(1, bounds.maxX - bounds.minX)
      const height = Math.max(1, bounds.maxY - bounds.minY)
      const scale = Math.min(
        maxScale,
        Math.max(0.04, (rect.width - padding * 2) / width),
        Math.max(0.04, (rect.height - padding * 2) / height),
      )
      const centerX = (bounds.minX + bounds.maxX) / 2
      const centerY = (bounds.minY + bounds.maxY) / 2
      const transform = d3.zoomIdentity
        .translate(rect.width / 2 - centerX * scale, rect.height / 2 - centerY * scale)
        .scale(scale)
      const selection = d3.select(svg)
      if (animate) selection.transition().duration(260).call(zoom.transform, transform)
      else selection.call(zoom.transform, transform)
    },
    [],
  )

  const fitToScreen = useCallback(
    (animate = true) => {
      if (layout.width === 0 || layout.height === 0) return
      fitBounds({ minX: 0, minY: 0, maxX: layout.width, maxY: layout.height }, 1.15, animate)
    },
    [fitBounds, layout.height, layout.width],
  )

  const showDefaultViewport = useCallback(() => {
    const svg = svgRef.current
    const container = containerRef.current
    const zoom = zoomRef.current
    if (!svg || !container || !zoom || layout.width === 0 || layout.height === 0) return
    const rect = container.getBoundingClientRect()
    if (rect.width === 0 || rect.height === 0) return
    const padding = 72
    const overviewScale = Math.min(
      1,
      (rect.width - padding * 2) / layout.width,
      (rect.height - padding * 2) / layout.height,
    )
    // 大图首次加载保持可读尺度，超出视口的内容通过无限平移浏览；全图概览由按钮显式触发。
    const scale = Math.max(DEFAULT_VIEW_SCALE, overviewScale)
    // 超大层级图的包围盒中心可能完全没有节点。首次进入时锚定分支最多的主根，
    // 让用户立即看到层级入口；小图仍以完整节点包围盒中心展示。
    const isLargeGraph = overviewScale < DEFAULT_VIEW_SCALE
    const anchor = isLargeGraph ? selectPertPrimaryRoot(layout.nodes, layout.edges) : null
    const nodeBounds = anchor
      ? null
      : {
          minX: Math.min(...layout.nodes.map((node) => node.x - node.radius)),
          maxX: Math.max(...layout.nodes.map((node) => node.x + node.radius)),
          minY: Math.min(...layout.nodes.map((node) => node.y - node.radius - 16)),
          maxY: Math.max(...layout.nodes.map((node) => node.y + node.radius + 38)),
        }
    const centerX = anchor ? anchor.x : (nodeBounds!.minX + nodeBounds!.maxX) / 2
    const centerY = anchor ? anchor.y : (nodeBounds!.minY + nodeBounds!.maxY) / 2
    const transform = d3.zoomIdentity
      .translate(rect.width / 2 - centerX * scale, rect.height / 2 - centerY * scale)
      .scale(scale)
    d3.select(svg).call(zoom.transform, transform)
  }, [layout.edges, layout.height, layout.nodes, layout.width])

  const fitMatchedNodes = useCallback(
    (nodeIds: Set<string>, animate = true) => {
      const nodes = layout.nodes.filter((node) => nodeIds.has(node.id))
      if (nodes.length === 0) return
      const horizontalLabelSpace = 84
      const titleSpace = 38
      fitBounds(
        {
          minX: Math.min(...nodes.map((node) => node.x - node.radius - horizontalLabelSpace)),
          minY: Math.min(...nodes.map((node) => node.y - node.radius - 16)),
          maxX: Math.max(...nodes.map((node) => node.x + node.radius + horizontalLabelSpace)),
          maxY: Math.max(...nodes.map((node) => node.y + node.radius + titleSpace)),
        },
        1.65,
        animate,
      )
    },
    [fitBounds, layout.nodes],
  )

  useEffect(() => {
    const svg = svgRef.current
    if (!svg) return
    const zoom = d3
      .zoom<SVGSVGElement, unknown>()
      .extent((): [[number, number], [number, number]] => {
        const rect = containerRef.current?.getBoundingClientRect()
        return [
          [0, 0],
          [rect?.width ?? 0, rect?.height ?? 0],
        ]
      })
      .scaleExtent([0.04, 3])
      .clickDistance(4)
      .on('zoom', (event) => {
        d3.select(svg)
          .select<SVGGElement>('[data-role="pert-canvas"]')
          .attr('transform', event.transform)
        d3.select(svg)
          .select<SVGPatternElement>('#pert-grid')
          .attr('patternTransform', event.transform.toString())
      })
    d3.select(svg).call(zoom).on('dblclick.zoom', null)
    zoomRef.current = zoom
    return () => {
      d3.select(svg).on('.zoom', null)
    }
  }, [])

  useEffect(() => {
    const frame = window.requestAnimationFrame(showDefaultViewport)
    return () => window.cancelAnimationFrame(frame)
  }, [showDefaultViewport])

  useEffect(() => {
    if (!matched || matched.size === 0) return
    const frame = window.requestAnimationFrame(() => fitMatchedNodes(matched, false))
    return () => window.cancelAnimationFrame(frame)
  }, [fitMatchedNodes, matched])

  useEffect(() => {
    if (selectedEdgeId && !layout.edges.some((edge) => edge.id === selectedEdgeId))
      setSelectedEdgeId(null)
  }, [layout.edges, selectedEdgeId])

  const handleNodeClick = (id: string) => {
    if (clickTimerRef.current) window.clearTimeout(clickTimerRef.current)
    clickTimerRef.current = window.setTimeout(() => onSelect(id), CLICK_DELAY_MS)
  }
  const handleNodeDoubleClick = (id: string) => {
    if (clickTimerRef.current) window.clearTimeout(clickTimerRef.current)
    clickTimerRef.current = null
    onOpenFull?.(id)
  }

  const edgeState = (edge: PertEdge) => {
    if (trace) {
      if (edge.id === selectedEdgeId) return 'selected'
      return trace.edgeIds.has(edge.id) ? 'related' : 'dimmed'
    }
    if (searchHighlightActive && searchTrace)
      return searchTrace.edgeIds.has(edge.id) ? 'related' : 'dimmed'
    return 'normal'
  }

  const nodeDimmed = (id: string) => {
    if (trace && !trace.nodeIds.has(id)) return true
    if (searchHighlightActive && matched && !matched.has(id)) return true
    return false
  }

  return (
    <div className={cn('flex flex-col gap-2', className)}>
      <div className="flex flex-wrap items-center gap-3 px-1">
        <Input
          value={search}
          onChange={(event) => {
            setSearch(event.target.value)
            if (event.target.value.trim()) setSelectedEdgeId(null)
          }}
          placeholder="搜索任务编号 / 标题…"
          className="h-8 w-56"
          aria-label="搜索任务"
        />
        <Button variant="outline" size="sm" onClick={() => fitToScreen()}>
          <Maximize2 data-icon="inline-start" />
          适应屏幕
        </Button>
        {selectedEdge && (
          <Button variant="secondary" size="sm" onClick={() => setSelectedEdgeId(null)}>
            <X data-icon="inline-start" />
            清除路径
          </Button>
        )}
        <span className="text-caption text-muted-foreground">
          {data.nodes.length} 节点 · {layout.maxLayer + 1} 层 · 拖拽画布平移 · 滚轮缩放
        </span>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-2 px-1">
        <PertLegend states={states} />
        {selectedEdge && trace && (
          <p className="text-caption text-muted-foreground" role="status">
            已选择 {nodeMap.get(selectedEdge.from)?.number || '起点'} →{' '}
            {nodeMap.get(selectedEdge.to)?.number || '终点'}
            {' · '}上游 {trace.upstreamNodeIds.size} · 下游 {trace.downstreamNodeIds.size}
          </p>
        )}
      </div>

      {layout.hasCycle && (
        <div
          className="rounded-2xl border border-destructive/40 bg-destructive/10 px-4 py-3 text-body text-destructive"
          role="alert"
        >
          检测到循环依赖：{layout.cycle?.join(' ↔ ') ?? '未知'}。PERT
          图无法展示该依赖环，请先解除循环依赖。
        </div>
      )}

      <div
        ref={containerRef}
        className="relative h-[calc(100vh-15rem)] min-h-[620px] overflow-hidden rounded-2xl border border-divider bg-card"
        aria-label="PERT 任务图"
        data-viewport="infinite"
      >
        <svg
          ref={svgRef}
          className="size-full cursor-grab touch-none active:cursor-grabbing"
          onClick={(event) => {
            if (
              event.target === event.currentTarget ||
              (event.target as Element).closest('[data-role="canvas-background"]')
            ) {
              setSelectedEdgeId(null)
              setSearchHighlightActive(false)
            }
          }}
        >
          <defs>
            <pattern id="pert-grid" width="24" height="24" patternUnits="userSpaceOnUse">
              <circle cx="1" cy="1" r="1" fill="var(--border)" opacity="0.55" />
            </pattern>
            {[...rootStyleMap.entries()].flatMap(([rootId, style]) =>
              [
                { suffix: 'normal', opacity: 0.62 },
                { suffix: 'active', opacity: 1 },
                { suffix: 'dimmed', opacity: 0.08 },
              ].map((markerState) => (
                <marker
                  key={`${rootId}-${markerState.suffix}`}
                  id={`${style.markerPrefix}-${markerState.suffix}`}
                  markerWidth="10"
                  markerHeight="10"
                  refX="9"
                  refY="5"
                  orient="auto"
                  markerUnits="userSpaceOnUse"
                >
                  <path
                    d="M 0 0 L 10 5 L 0 10 z"
                    fill={style.color}
                    fillOpacity={markerState.opacity}
                  />
                </marker>
              )),
            )}
          </defs>
          <rect data-role="canvas-background" width="100%" height="100%" fill="url(#pert-grid)" />

          <g data-role="pert-canvas">
            {!layout.hasCycle && (
              <>
                <g data-role="pert-edges" fill="none">
                  {layout.edges.map((edge) => {
                    const state = edgeState(edge)
                    const path = bezierPath(edge.segments)
                    const rootStyle = rootStyleMap.get(edge.rootId) ?? {
                      color: rootLineColor(0),
                      markerPrefix: 'pert-arrow-0',
                    }
                    const markerState =
                      state === 'dimmed' ? 'dimmed' : state === 'normal' ? 'normal' : 'active'
                    return (
                      <g
                        key={edge.id}
                        data-edge-id={edge.id}
                        data-root-id={edge.rootId}
                        data-routed={edge.routed ? 'corridor' : 'direct'}
                        data-state={state}
                      >
                        <path
                          d={path}
                          stroke="transparent"
                          strokeWidth={16}
                          pointerEvents="stroke"
                          tabIndex={0}
                          className="cursor-default outline-none focus:outline-none focus-visible:outline-none"
                          style={{ outline: 'none' }}
                          role="button"
                          aria-label={`选择${edge.type === 'parent' ? '父子关系' : '依赖'} ${nodeMap.get(edge.from)?.number || edge.from} 到 ${nodeMap.get(edge.to)?.number || edge.to}`}
                          onClick={(event) => {
                            event.stopPropagation()
                            setSelectedEdgeId((current) => (current === edge.id ? null : edge.id))
                          }}
                          onKeyDown={(event) => {
                            if (event.key === 'Enter' || event.key === ' ') {
                              event.preventDefault()
                              setSelectedEdgeId((current) => (current === edge.id ? null : edge.id))
                            }
                          }}
                        />
                        <path
                          d={path}
                          data-role="visible-edge"
                          stroke={rootStyle.color}
                          strokeWidth={state === 'selected' ? 4.5 : state === 'related' ? 3.5 : 2.5}
                          strokeOpacity={state === 'dimmed' ? 0.08 : state === 'normal' ? 0.62 : 1}
                          markerEnd={`url(#${rootStyle.markerPrefix}-${markerState})`}
                          pointerEvents="none"
                        />
                      </g>
                    )
                  })}
                </g>

                <g data-role="pert-nodes">
                  {layout.nodes.map((node) => {
                    const color = colorMap[node.status] ?? ARCHIVED_COLOR
                    const isEndpoint =
                      selectedEdge?.from === node.id || selectedEdge?.to === node.id
                    const isUpstream = trace?.upstreamNodeIds.has(node.id) ?? false
                    const isDownstream = trace?.downstreamNodeIds.has(node.id) ?? false
                    const isSearchMatch = matched?.has(node.id) ?? false
                    const dimmed = nodeDimmed(node.id)
                    const label = node.number || compactTitle(node.title, 6)
                    return (
                      <g
                        key={node.id}
                        transform={`translate(${node.x}, ${node.y})`}
                        opacity={dimmed ? 0.16 : 1}
                        className="cursor-pointer outline-none"
                        role="button"
                        tabIndex={0}
                        aria-label={`${node.number} ${node.title}`}
                        onClick={(event) => {
                          event.stopPropagation()
                          handleNodeClick(node.id)
                        }}
                        onDoubleClick={(event) => {
                          event.stopPropagation()
                          handleNodeDoubleClick(node.id)
                        }}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter' || event.key === ' ') {
                            event.preventDefault()
                            onSelect(node.id)
                          }
                        }}
                      >
                        <title>
                          {node.number ? `${node.number} · ` : ''}
                          {node.title}
                        </title>
                        {isSearchMatch && (
                          <circle
                            data-role="search-result-marker"
                            r={node.radius + 11}
                            fill="none"
                            stroke="var(--primary)"
                            strokeWidth={2.5}
                            opacity={0.9}
                            vectorEffect="non-scaling-stroke"
                          />
                        )}
                        {isEndpoint && (
                          <circle
                            r={node.radius + 7}
                            fill="none"
                            stroke="var(--primary)"
                            strokeWidth={3}
                            opacity={0.35}
                          />
                        )}
                        <circle
                          r={node.radius}
                          fill={color}
                          fillOpacity={0.14}
                          stroke={isEndpoint ? 'var(--primary)' : color}
                          strokeWidth={
                            isEndpoint ? 3 : trace && (isUpstream || isDownstream) ? 2.5 : 2
                          }
                          vectorEffect="non-scaling-stroke"
                        />
                        <text
                          textAnchor="middle"
                          dominantBaseline="central"
                          fontSize={11}
                          fontWeight={650}
                          fill="var(--foreground)"
                          pointerEvents="none"
                        >
                          {compactTitle(label, 9)}
                        </text>
                        <text
                          y={node.radius + 19}
                          textAnchor="middle"
                          fontSize={11}
                          fontWeight={500}
                          fill="var(--foreground)"
                          pointerEvents="none"
                        >
                          {compactTitle(node.title, 14)}
                        </text>
                      </g>
                    )
                  })}
                </g>
              </>
            )}
          </g>
        </svg>
      </div>
    </div>
  )
}

function compactTitle(value: string, max: number) {
  return value.length > max ? `${value.slice(0, max - 1)}…` : value
}

function rootLineColor(index: number) {
  // 黄金角分布让任意数量的根任务都得到稳定且不重复的可辨色相。
  const hue = Math.round((index * 137.508 + 215) % 360)
  return `hsl(${hue} 72% 43%)`
}
