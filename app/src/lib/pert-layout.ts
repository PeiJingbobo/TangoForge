/**
 * PERT 分层布局与依赖路径追踪。
 *
 * 最长路径分层保证依赖从左向右；barycenter 排序降低层间交叉；
 * 确定性错位打散节点的网格感，三次贝塞尔曲线保持分支自然、可复现。
 */
import type { GraphData, GraphEdge } from '@/types/models'

export const PERT_DEFAULTS = {
  nodeDiameter: 64,
  layerGap: 720,
  rowGap: 460,
  canvasPadding: 220,
  branchLength: 112,
  scatterX: 90,
  scatterY: 0,
  obstacleClearance: 32,
  edgeClearance: 24,
  routeLaneGap: 64,
  routeLaneCount: 8,
  barycenterIterations: 48,
  verticalIterations: 24,
} as const

export interface PertLayoutOptions {
  nodeDiameter?: number
  layerGap?: number
  rowGap?: number
  canvasPadding?: number
  branchLength?: number
  scatterX?: number
  scatterY?: number
  obstacleClearance?: number
  edgeClearance?: number
  routeLaneGap?: number
  routeLaneCount?: number
  barycenterIterations?: number
  verticalIterations?: number
  edgeFilter?: (edge: GraphEdge) => boolean
}

export interface PertPoint {
  x: number
  y: number
}

export type PertBezierSegment = [PertPoint, PertPoint, PertPoint, PertPoint]

export interface PertNode {
  id: string
  title: string
  number: string
  status: string
  layer: number
  indexInLayer: number
  x: number
  y: number
  radius: number
  predIds: string[]
  rootId: string
}

export interface PertEdge {
  id: string
  from: string
  to: string
  type: GraphEdge['type']
  /** 一段或多段三次贝塞尔；同源边共享第一段起点与第一控制点。 */
  segments: PertBezierSegment[]
  rootId: string
  routed: boolean
}

export interface PertLayout {
  nodes: PertNode[]
  edges: PertEdge[]
  layers: string[][]
  maxLayer: number
  cycle: string[] | null
  hasCycle: boolean
  width: number
  height: number
  /** Proper-layer graph 中相邻层线段的组合交叉数，用于布局质量回归。 */
  layerCrossings: number
}

export interface PertTrace {
  nodeIds: Set<string>
  edgeIds: Set<string>
  upstreamNodeIds: Set<string>
  downstreamNodeIds: Set<string>
}

export const pertEdgeId = (from: string, to: string) => `${from}->${to}`

export function selectPertPrimaryRoot(nodes: PertNode[], edges: PertEdge[]): PertNode | null {
  if (nodes.length === 0) return null
  const outgoing = new Map<string, number>()
  for (const edge of edges) outgoing.set(edge.from, (outgoing.get(edge.from) ?? 0) + 1)
  return (
    nodes
      .slice()
      .sort(
        (a, b) =>
          (outgoing.get(b.id) ?? 0) - (outgoing.get(a.id) ?? 0) ||
          a.layer - b.layer ||
          a.indexInLayer - b.indexInLayer ||
          a.id.localeCompare(b.id),
      )[0] ?? null
  )
}

export function bezierPath(segments: PertBezierSegment[]): string {
  if (segments.length === 0) return ''
  const parts = [`M ${segments[0][0].x} ${segments[0][0].y}`]
  for (const [, control1, control2, end] of segments) {
    parts.push(`C ${control1.x} ${control1.y} ${control2.x} ${control2.y} ${end.x} ${end.y}`)
  }
  return parts.join(' ')
}

export function pertLayout(data: GraphData, options: PertLayoutOptions = {}): PertLayout {
  const {
    nodeDiameter = PERT_DEFAULTS.nodeDiameter,
    layerGap = PERT_DEFAULTS.layerGap,
    rowGap = PERT_DEFAULTS.rowGap,
    canvasPadding = PERT_DEFAULTS.canvasPadding,
    branchLength = PERT_DEFAULTS.branchLength,
    scatterX = PERT_DEFAULTS.scatterX,
    scatterY = PERT_DEFAULTS.scatterY,
    obstacleClearance = PERT_DEFAULTS.obstacleClearance,
    edgeClearance = PERT_DEFAULTS.edgeClearance,
    routeLaneGap = PERT_DEFAULTS.routeLaneGap,
    routeLaneCount = PERT_DEFAULTS.routeLaneCount,
    barycenterIterations = PERT_DEFAULTS.barycenterIterations,
    verticalIterations = PERT_DEFAULTS.verticalIterations,
    edgeFilter = (edge) => edge.type === 'dependency' || edge.type === 'parent',
  } = options

  const empty: PertLayout = {
    nodes: [],
    edges: [],
    layers: [],
    maxLayer: 0,
    cycle: null,
    hasCycle: false,
    width: 0,
    height: 0,
    layerCrossings: 0,
  }
  if (data.nodes.length === 0) return empty

  const ids = new Set(data.nodes.map((node) => node.id))
  const graphEdgesById = new Map<string, GraphEdge>()
  for (const edge of data.edges.filter(edgeFilter)) {
    const id = pertEdgeId(edge.from, edge.to)
    if (!ids.has(edge.from) || !ids.has(edge.to)) continue
    const existing = graphEdgesById.get(id)
    // 同一对任务同时具有父子和依赖语义时只画一条线，并优先保留更具体的依赖语义。
    if (!existing || (existing.type === 'parent' && edge.type === 'dependency'))
      graphEdgesById.set(id, edge)
  }
  const graphEdges = [...graphEdgesById.values()]

  const preds = new Map<string, string[]>(data.nodes.map((node) => [node.id, []]))
  const succs = new Map<string, string[]>(data.nodes.map((node) => [node.id, []]))
  for (const edge of graphEdges) {
    preds.get(edge.to)?.push(edge.from)
    succs.get(edge.from)?.push(edge.to)
  }

  const inDegree = new Map(data.nodes.map((node) => [node.id, preds.get(node.id)?.length ?? 0]))
  const queue = data.nodes
    .filter((node) => inDegree.get(node.id) === 0)
    .map((node) => node.id)
    .sort()
  const topo: string[] = []
  while (queue.length > 0) {
    const id = queue.shift()!
    topo.push(id)
    for (const next of succs.get(id) ?? []) {
      const degree = (inDegree.get(next) ?? 0) - 1
      inDegree.set(next, degree)
      if (degree === 0) {
        queue.push(next)
        queue.sort()
      }
    }
  }

  const hasCycle = topo.length !== data.nodes.length
  const cycle = hasCycle
    ? detectCycle(
        succs,
        data.nodes.map((node) => node.id),
      )
    : null
  const layerOf = new Map(data.nodes.map((node) => [node.id, 0]))
  for (const id of topo) {
    const layer = Math.max(0, ...(preds.get(id) ?? []).map((pred) => (layerOf.get(pred) ?? 0) + 1))
    layerOf.set(id, layer)
  }

  const maxLayer = Math.max(0, ...layerOf.values())
  const layers = Array.from({ length: maxLayer + 1 }, () => [] as string[])
  for (const node of data.nodes) layers[layerOf.get(node.id) ?? 0].push(node.id)
  layers.forEach((layer) => layer.sort())

  // Sugiyama proper-layer graph：跨多层长边在每一中间层插入虚拟通道点，
  // 让长边参与逐层交叉最小化，而不是布局完成后才横跨既有节点。
  const expandedLayers = layers.map((layer) => layer.slice())
  const expandedPreds = new Map<string, string[]>(data.nodes.map((node) => [node.id, []]))
  const expandedSuccs = new Map<string, string[]>(data.nodes.map((node) => [node.id, []]))
  const edgeWaypoints = new Map<string, string[]>()
  for (const edge of graphEdges) {
    const edgeId = pertEdgeId(edge.from, edge.to)
    const sourceLayer = layerOf.get(edge.from) ?? 0
    const targetLayer = layerOf.get(edge.to) ?? sourceLayer
    const chain = [edge.from]
    const waypoints: string[] = []
    for (let layer = sourceLayer + 1; layer < targetLayer; layer++) {
      const dummyId = `__pert_route__${edgeId}:${layer}`
      waypoints.push(dummyId)
      chain.push(dummyId)
      expandedLayers[layer].push(dummyId)
      expandedPreds.set(dummyId, [])
      expandedSuccs.set(dummyId, [])
    }
    chain.push(edge.to)
    edgeWaypoints.set(edgeId, waypoints)
    for (let index = 0; index < chain.length - 1; index++) {
      expandedSuccs.set(chain[index], [
        ...(expandedSuccs.get(chain[index]) ?? []),
        chain[index + 1],
      ])
      expandedPreds.set(chain[index + 1], [
        ...(expandedPreds.get(chain[index + 1]) ?? []),
        chain[index],
      ])
    }
  }
  expandedLayers.forEach((layer) => layer.sort())

  const order = new Map<string, number>()
  const refreshOrder = () =>
    expandedLayers.forEach((layer) => layer.forEach((id, index) => order.set(id, index)))
  const reorder = (layer: string[], neighbors: (id: string) => string[]) => {
    const previous = new Map(layer.map((id, index) => [id, index]))
    const score = (id: string) => {
      const values = neighbors(id)
        .map((neighbor) => order.get(neighbor))
        .filter((n): n is number => n !== undefined)
      if (values.length === 0) return previous.get(id) ?? 0
      values.sort((a, b) => a - b)
      const middle = Math.floor(values.length / 2)
      return values.length % 2 === 0 ? (values[middle - 1] + values[middle]) / 2 : values[middle]
    }
    layer.sort((a, b) => score(a) - score(b) || (previous.get(a) ?? 0) - (previous.get(b) ?? 0))
  }

  refreshOrder()
  for (let iteration = 0; iteration < Math.max(1, barycenterIterations); iteration++) {
    for (let layer = 1; layer <= maxLayer; layer++) {
      reorder(expandedLayers[layer], (id) => expandedPreds.get(id) ?? [])
      refreshOrder()
    }
    for (let layer = maxLayer - 1; layer >= 0; layer--) {
      reorder(expandedLayers[layer], (id) => expandedSuccs.get(id) ?? [])
      refreshOrder()
    }
  }
  minimizeAdjacentLayerCrossings(expandedLayers, expandedSuccs, 16)
  refreshOrder()
  const layerCrossings = expandedLayers
    .slice(0, -1)
    .reduce(
      (total, layer, index) =>
        total + countCrossingsBetweenLayers(layer, expandedLayers[index + 1], expandedSuccs),
      0,
    )
  layers.forEach((layer, index) => {
    layer.splice(0, layer.length, ...expandedLayers[index].filter((id) => ids.has(id)))
  })

  // 在合流节点上选取排序最靠前的上游谱系作为主谱系；之后的所有出边继承该颜色。
  const rootOf = new Map<string, string>()
  for (const id of topo) {
    const parents = (preds.get(id) ?? [])
      .slice()
      .sort((a, b) => (order.get(a) ?? 0) - (order.get(b) ?? 0) || a.localeCompare(b))
    rootOf.set(id, parents.length === 0 ? id : (rootOf.get(parents[0]) ?? parents[0]))
  }
  for (const node of data.nodes) {
    if (!rootOf.has(node.id)) rootOf.set(node.id, node.id)
  }

  const radius = nodeDiameter / 2
  const layerPitch = nodeDiameter + layerGap
  const rowPitch = nodeDiameter + rowGap
  const nodeById = new Map(data.nodes.map((node) => [node.id, node]))
  const nodes: PertNode[] = []
  const verticalY = assignLayerItemCoordinates(
    expandedLayers,
    expandedPreds,
    expandedSuccs,
    ids,
    rowPitch,
    rowPitch / 2,
    routeLaneGap,
    canvasPadding + radius,
    verticalIterations,
  )
  for (let layer = 0; layer < layers.length; layer++) {
    layers[layer].forEach((id, indexInLayer) => {
      const node = nodeById.get(id)!
      nodes.push({
        id,
        title: node.title,
        number: node.number,
        status: node.status,
        layer,
        indexInLayer,
        x: canvasPadding + radius + layer * layerPitch + deterministicOffset(`${id}:x`, scatterX),
        y: (verticalY.get(id) ?? canvasPadding + radius) + deterministicOffset(`${id}:y`, scatterY),
        radius,
        predIds: preds.get(id) ?? [],
        rootId: rootOf.get(id) ?? id,
      })
    })
  }

  const position = new Map(nodes.map((node) => [node.id, node]))
  const routingOrder = graphEdges
    .slice()
    .sort((a, b) => pertEdgeId(a.from, a.to).localeCompare(pertEdgeId(b.from, b.to)))
  const occupiedRoutes: PertBezierSegment[][] = []
  const routedById = new Map<string, PertEdge>()
  for (const edge of routingOrder) {
    const source = position.get(edge.from)!
    const target = position.get(edge.to)!
    const start = { x: source.x + radius, y: source.y }
    const sourceBranch = { x: start.x + branchLength, y: source.y }
    const end = { x: target.x - radius, y: target.y }
    const obstacles = nodes.filter((node) => node.id !== source.id && node.id !== target.id)
    const waypointIds = edgeWaypoints.get(pertEdgeId(edge.from, edge.to)) ?? []
    const waypoints = waypointIds.map((id) => ({
      x: canvasPadding + radius + (Number(id.slice(id.lastIndexOf(':') + 1)) || 0) * layerPitch,
      y: verticalY.get(id) ?? (source.y + target.y) / 2,
    }))
    const layeredRoute = layeredSplineRoute([start, ...waypoints, end], sourceBranch, branchLength)
    const segments =
      routeCollisionCount(layeredRoute, obstacles, obstacleClearance) === 0
        ? layeredRoute
        : obstacleAvoidingBezierRoute(
            start,
            end,
            sourceBranch,
            obstacles,
            branchLength,
            obstacleClearance,
            true,
            occupiedRoutes,
            edgeClearance,
            routeLaneGap,
            routeLaneCount,
          )
    const routed = {
      id: pertEdgeId(edge.from, edge.to),
      from: edge.from,
      to: edge.to,
      type: edge.type,
      segments,
      rootId: source.rootId,
      routed: segments.length > 1 || target.layer - source.layer > 1,
    }
    routedById.set(routed.id, routed)
    occupiedRoutes.push(segments)
  }
  const edges = graphEdges.map((edge) => routedById.get(pertEdgeId(edge.from, edge.to))!)

  const routeControlPoints = edges.flatMap((edge) => edge.segments.flat())
  let routePoints = edges.flatMap((edge) => sampleBezierRoute(edge.segments, 24))
  const minimumContentY = Math.min(
    ...nodes.map((node) => node.y - node.radius - 24),
    ...routePoints.map((point) => point.y),
  )
  const verticalShift = Math.max(0, canvasPadding - minimumContentY)
  if (verticalShift > 0) {
    nodes.forEach((node) => {
      node.y += verticalShift
    })
    new Set(routeControlPoints).forEach((point) => {
      point.y += verticalShift
    })
    routePoints = edges.flatMap((edge) => sampleBezierRoute(edge.segments, 24))
  }
  const maxX = Math.max(
    ...nodes.map((node) => node.x + node.radius),
    ...routePoints.map((point) => point.x),
  )
  const maxY = Math.max(
    ...nodes.map((node) => node.y + node.radius + 24),
    ...routePoints.map((point) => point.y),
  )
  return {
    nodes,
    edges,
    layers,
    maxLayer,
    cycle,
    hasCycle,
    width: maxX + canvasPadding,
    height: maxY + canvasPadding,
    layerCrossings,
  }
}

/** 选择边后追踪源端全部上游、目标端全部下游，以及实际遍历到的依赖边。 */
export function tracePertEdge(edges: PertEdge[], selectedEdgeId: string): PertTrace | null {
  const selected = edges.find((edge) => edge.id === selectedEdgeId)
  if (!selected) return null
  const { incoming, outgoing } = buildPertAdjacency(edges)
  const upstreamNodeIds = new Set<string>([selected.from])
  const downstreamNodeIds = new Set<string>([selected.to])
  const edgeIds = new Set<string>([selected.id])
  walkPertEdges([selected.from], incoming, (edge) => edge.from, upstreamNodeIds, edgeIds)
  walkPertEdges([selected.to], outgoing, (edge) => edge.to, downstreamNodeIds, edgeIds)
  return {
    nodeIds: new Set([...upstreamNodeIds, ...downstreamNodeIds]),
    edgeIds,
    upstreamNodeIds,
    downstreamNodeIds,
  }
}

/** 搜索节点后追踪每个命中的完整上游溯源与下游可达路径。 */
export function tracePertNodes(edges: PertEdge[], matchedNodeIds: Set<string>): PertTrace | null {
  if (matchedNodeIds.size === 0) return null
  const { incoming, outgoing } = buildPertAdjacency(edges)
  const starts = [...matchedNodeIds]
  const upstreamNodeIds = new Set(starts)
  const downstreamNodeIds = new Set(starts)
  const edgeIds = new Set<string>()
  walkPertEdges(starts, incoming, (edge) => edge.from, upstreamNodeIds, edgeIds)
  walkPertEdges(starts, outgoing, (edge) => edge.to, downstreamNodeIds, edgeIds)
  return {
    nodeIds: new Set([...upstreamNodeIds, ...downstreamNodeIds]),
    edgeIds,
    upstreamNodeIds,
    downstreamNodeIds,
  }
}

function buildPertAdjacency(edges: PertEdge[]) {
  const incoming = new Map<string, PertEdge[]>()
  const outgoing = new Map<string, PertEdge[]>()
  for (const edge of edges) {
    incoming.set(edge.to, [...(incoming.get(edge.to) ?? []), edge])
    outgoing.set(edge.from, [...(outgoing.get(edge.from) ?? []), edge])
  }
  return { incoming, outgoing }
}

function walkPertEdges(
  starts: string[],
  adjacency: Map<string, PertEdge[]>,
  next: (edge: PertEdge) => string,
  nodeIds: Set<string>,
  edgeIds: Set<string>,
) {
  const queue = [...starts]
  while (queue.length > 0) {
    const id = queue.shift()!
    for (const edge of adjacency.get(id) ?? []) {
      edgeIds.add(edge.id)
      const nextId = next(edge)
      if (nodeIds.has(nextId)) continue
      nodeIds.add(nextId)
      queue.push(nextId)
    }
  }
}

function detectCycle(succs: Map<string, string[]>, nodeIds: string[]): string[] | null {
  const color = new Map<string, 0 | 1 | 2>(nodeIds.map((id) => [id, 0]))
  const path: string[] = []
  const visit = (id: string): string[] | null => {
    color.set(id, 1)
    path.push(id)
    for (const next of succs.get(id) ?? []) {
      if (color.get(next) === 1) return path.slice(path.indexOf(next))
      if (color.get(next) === 0) {
        const cycle = visit(next)
        if (cycle) return cycle
      }
    }
    path.pop()
    color.set(id, 2)
    return null
  }
  for (const id of nodeIds) {
    if (color.get(id) === 0) {
      const cycle = visit(id)
      if (cycle) return cycle
    }
  }
  return null
}

/** 在 barycenter 扫描后反复交换相邻通道；只接受能减少相邻层交叉数的交换。 */
function minimizeAdjacentLayerCrossings(
  layers: string[][],
  succs: Map<string, string[]>,
  maxPasses: number,
) {
  const localScore = (layerIndex: number) =>
    (layerIndex > 0
      ? countCrossingsBetweenLayers(layers[layerIndex - 1], layers[layerIndex], succs)
      : 0) +
    (layerIndex < layers.length - 1
      ? countCrossingsBetweenLayers(layers[layerIndex], layers[layerIndex + 1], succs)
      : 0)
  for (let pass = 0; pass < maxPasses; pass++) {
    let improved = false
    const layerIndexes = Array.from({ length: Math.max(0, layers.length - 2) }, (_, i) => i + 1)
    if (pass % 2 === 1) layerIndexes.reverse()
    for (const layerIndex of layerIndexes) {
      const layer = layers[layerIndex]
      const indexes = Array.from({ length: Math.max(0, layer.length - 1) }, (_, i) => i)
      if (pass % 2 === 1) indexes.reverse()
      for (const index of indexes) {
        const before = localScore(layerIndex)
        ;[layer[index], layer[index + 1]] = [layer[index + 1], layer[index]]
        const after = localScore(layerIndex)
        if (after < before) improved = true
        else [layer[index], layer[index + 1]] = [layer[index + 1], layer[index]]
      }
    }
    if (!improved) break
  }
}

function countCrossingsBetweenLayers(
  left: string[],
  right: string[],
  succs: Map<string, string[]>,
): number {
  const rightOrder = new Map(right.map((id, index) => [id, index]))
  const edgeOrders = left.flatMap((id, sourceIndex) =>
    (succs.get(id) ?? [])
      .map((target) => rightOrder.get(target))
      .filter((targetIndex): targetIndex is number => targetIndex !== undefined)
      .map((targetIndex) => ({ sourceIndex, targetIndex })),
  )
  let crossings = 0
  for (let first = 0; first < edgeOrders.length; first++) {
    for (let second = first + 1; second < edgeOrders.length; second++) {
      const a = edgeOrders[first]
      const b = edgeOrders[second]
      if (a.sourceIndex === b.sourceIndex || a.targetIndex === b.targetIndex) continue
      if ((a.sourceIndex - b.sourceIndex) * (a.targetIndex - b.targetIndex) < 0) crossings++
    }
  }
  return crossings
}

/**
 * 为真实节点与长边虚拟通道共同分配纵向坐标。节点-节点、节点-通道、通道-通道
 * 使用不同最小间距，节点无需对齐，但任何路径在穿过层中心时都有独立安全空间。
 */
function assignLayerItemCoordinates(
  layers: string[][],
  preds: Map<string, string[]>,
  succs: Map<string, string[]>,
  realIds: Set<string>,
  realGap: number,
  realRouteGap: number,
  routeGap: number,
  minimumY: number,
  iterations: number,
): Map<string, number> {
  const y = new Map<string, number>()
  const gapsFor = (layer: string[]) =>
    layer.slice(1).map((id, index) => {
      const previous = layer[index]
      if (realIds.has(previous) && realIds.has(id)) return realGap
      if (realIds.has(previous) || realIds.has(id)) return realRouteGap
      return routeGap
    })
  layers.forEach((layer) => {
    const gaps = gapsFor(layer)
    let cursor = minimumY
    layer.forEach((id, index) => {
      if (index > 0) cursor += gaps[index - 1]
      y.set(id, cursor)
    })
  })

  const relax = (layer: string[], neighbors: Map<string, string[]>, weight: number) => {
    const desired = layer.map((id) => {
      const adjacent = (neighbors.get(id) ?? [])
        .map((neighbor) => y.get(neighbor))
        .filter((value): value is number => value !== undefined)
      const current = y.get(id) ?? minimumY
      return adjacent.length === 0 ? current : current * (1 - weight) + median(adjacent) * weight
    })
    const projected = projectOrderedPositionsWithGaps(desired, gapsFor(layer))
    layer.forEach((id, index) => y.set(id, projected[index]))
  }

  for (let iteration = 0; iteration < Math.max(1, iterations); iteration++) {
    for (let layer = 1; layer < layers.length; layer++) relax(layers[layer], preds, 0.78)
    for (let layer = layers.length - 2; layer >= 0; layer--) relax(layers[layer], succs, 0.42)
  }
  for (let layer = 1; layer < layers.length; layer++) relax(layers[layer], preds, 0.88)

  const currentMinimum = Math.min(...y.values())
  const shift = currentMinimum < minimumY ? minimumY - currentMinimum : 0
  if (shift > 0) for (const [id, value] of y) y.set(id, value + shift)
  return y
}

function projectOrderedPositionsWithGaps(desired: number[], gaps: number[]): number[] {
  if (desired.length <= 1) return desired.slice()
  const offsets = new Array<number>(desired.length).fill(0)
  for (let index = 1; index < desired.length; index++) {
    offsets[index] = offsets[index - 1] + (gaps[index - 1] ?? 0)
  }
  const adjusted = desired.map((value, index) => value - offsets[index])
  const projected = projectOrderedPositions(adjusted, 0)
  return projected.map((value, index) => value + offsets[index])
}

/** 每一层间使用一段水平切线三次贝塞尔；连接点两侧切线共线且曲线不产生纵向过冲。 */
function layeredSplineRoute(
  points: PertPoint[],
  sourceBranch: PertPoint,
  branchLength: number,
): PertBezierSegment[] {
  const segments: PertBezierSegment[] = []
  for (let index = 0; index < points.length - 1; index++) {
    const start = points[index]
    const end = points[index + 1]
    const distance = Math.max(1, end.x - start.x)
    const handle = Math.min(branchLength * 1.8, distance * 0.38)
    const control1 = index === 0 ? sourceBranch : { x: start.x + handle, y: start.y }
    const control2 = { x: end.x - handle, y: end.y }
    segments.push([start, control1, control2, end])
  }
  return segments
}

/**
 * Sugiyama 风格的纵向坐标松弛：层内顺序保持不变，坐标则尽量贴近相邻父子节点。
 * 与“每层独立居中”不同，单前驱末端节点会自然落在父节点附近。
 */
export function assignVerticalCoordinates(
  layers: string[][],
  preds: Map<string, string[]>,
  succs: Map<string, string[]>,
  minimumGap: number,
  minimumY: number,
  iterations: number = PERT_DEFAULTS.verticalIterations,
): Map<string, number> {
  const y = new Map<string, number>()
  layers.forEach((layer) => {
    layer.forEach((id, index) => y.set(id, minimumY + index * minimumGap))
  })

  const relaxLayer = (
    layer: string[],
    neighbors: Map<string, string[]>,
    neighborWeight: number,
  ) => {
    const previousOrder = new Map(layer.map((id, index) => [id, index]))
    const desiredById = new Map(
      layer.map((id) => {
        const adjacent = (neighbors.get(id) ?? [])
          .map((neighbor) => y.get(neighbor))
          .filter((value): value is number => value !== undefined)
        const current = y.get(id) ?? minimumY
        if (adjacent.length === 0) return [id, current] as const
        const target = median(adjacent)
        return [id, current * (1 - neighborWeight) + target * neighborWeight] as const
      }),
    )
    layer.sort(
      (a, b) =>
        (desiredById.get(a) ?? minimumY) - (desiredById.get(b) ?? minimumY) ||
        (previousOrder.get(a) ?? 0) - (previousOrder.get(b) ?? 0),
    )
    const desired = layer.map((id) => desiredById.get(id) ?? minimumY)
    const projected = projectOrderedPositions(desired, minimumGap)
    layer.forEach((id, index) => y.set(id, projected[index]))
  }

  for (let iteration = 0; iteration < Math.max(1, iterations); iteration++) {
    for (let layer = 1; layer < layers.length; layer++) {
      relaxLayer(layers[layer], preds, 0.82)
    }
    for (let layer = layers.length - 2; layer >= 0; layer--) {
      relaxLayer(layers[layer], succs, 0.38)
    }
  }

  // 最终以前驱为主收敛，避免末端层再次被反向松弛拉离唯一父节点。
  for (let layer = 1; layer < layers.length; layer++) {
    relaxLayer(layers[layer], preds, 1)
  }

  const currentMinimum = Math.min(...y.values())
  const shift = currentMinimum < minimumY ? minimumY - currentMinimum : 0
  if (shift > 0) {
    for (const [id, value] of y) y.set(id, value + shift)
  }
  return y
}

function projectOrderedPositions(desired: number[], minimumGap: number): number[] {
  if (desired.length <= 1) return desired.slice()
  // 将最小间距约束转换为单调回归，再用 PAVA 求平方误差最小的投影。
  const adjusted = desired.map((value, index) => value - index * minimumGap)
  const blocks: Array<{ start: number; end: number; sum: number; count: number }> = []
  adjusted.forEach((value, index) => {
    blocks.push({ start: index, end: index, sum: value, count: 1 })
    while (blocks.length > 1) {
      const current = blocks[blocks.length - 1]
      const previous = blocks[blocks.length - 2]
      if (previous.sum / previous.count <= current.sum / current.count) break
      blocks.splice(blocks.length - 2, 2, {
        start: previous.start,
        end: current.end,
        sum: previous.sum + current.sum,
        count: previous.count + current.count,
      })
    }
  })
  const projected = new Array<number>(desired.length)
  for (const block of blocks) {
    const mean = block.sum / block.count
    for (let index = block.start; index <= block.end; index++) {
      projected[index] = mean + index * minimumGap
    }
  }
  return projected
}

function median(values: number[]): number {
  const sorted = values.slice().sort((a, b) => a - b)
  const middle = Math.floor(sorted.length / 2)
  return sorted.length % 2 === 0 ? (sorted[middle - 1] + sorted[middle]) / 2 : sorted[middle]
}

function deterministicOffset(value: string, amplitude: number): number {
  if (amplitude === 0) return 0
  let hash = 2166136261
  for (let index = 0; index < value.length; index++) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return ((hash >>> 0) / 0xffffffff) * amplitude * 2 - amplitude
}

export function obstacleAvoidingBezierRoute(
  start: PertPoint,
  end: PertPoint,
  sourceBranch: PertPoint,
  obstacles: PertNode[],
  branchLength: number = PERT_DEFAULTS.branchLength,
  clearance: number = PERT_DEFAULTS.obstacleClearance,
  forceCorridor: boolean = false,
  occupiedRoutes: PertBezierSegment[][] = [],
  edgeClearance: number = PERT_DEFAULTS.edgeClearance,
  laneGap: number = PERT_DEFAULTS.routeLaneGap,
  laneCount: number = PERT_DEFAULTS.routeLaneCount,
): PertBezierSegment[] {
  const targetControlDistance = Math.min(
    branchLength * 0.8,
    Math.max(branchLength * 0.35, (end.x - start.x) * 0.1),
  )
  const direct: PertBezierSegment[] = [
    [start, sourceBranch, { x: end.x - targetControlDistance, y: end.y }, end],
  ]
  const averageY = (start.y + end.y) / 2
  const relevant = obstacles.filter(
    (node) => node.x + node.radius >= start.x && node.x - node.radius <= end.x,
  )
  const internalLanes = new Set<number>([averageY, start.y, end.y])
  for (const node of relevant) {
    const exclusion = node.radius + clearance + edgeClearance
    internalLanes.add(node.y - exclusion)
    internalLanes.add(node.y + exclusion)
  }
  for (let step = 1; step <= Math.max(4, Math.ceil(laneCount / 2)); step++) {
    internalLanes.add(averageY - laneGap * step)
    internalLanes.add(averageY + laneGap * step)
  }
  const occupiedY = [...relevant.map((node) => node.y), start.y, end.y]
  const minY = Math.min(...occupiedY)
  const maxY = Math.max(...occupiedY)
  const outerGap = PERT_DEFAULTS.rowGap / 2 + clearance + edgeClearance
  const outerLanes: number[] = []
  for (let step = 0; step < Math.max(2, laneCount); step++) {
    outerLanes.push(minY - outerGap - laneGap * step, maxY + outerGap + laneGap * step)
  }

  // 内部候选优先保留靠近端点中线的有限集合；外部候选始终保留，供长边绕开密集区域。
  const nearestInternal = [...internalLanes]
    .sort((a, b) => Math.abs(a - averageY) - Math.abs(b - averageY) || a - b)
    .slice(0, Math.max(12, laneCount * 2))
  const lanes = [...new Set([...nearestInternal, ...outerLanes])]
  const candidates: Array<{ route: PertBezierSegment[]; deviation: number; shapePenalty: number }> =
    []
  if (!forceCorridor) candidates.push({ route: direct, deviation: 0, shapePenalty: 0 })
  for (const laneY of lanes) {
    const curved = singleCurveForLane(start, end, sourceBranch, laneY, branchLength)
    if (curved)
      candidates.push({ route: curved, deviation: Math.abs(laneY - averageY), shapePenalty: 1 })
    const bowed = bowedSegments(start, end, sourceBranch, laneY, branchLength)
    if (bowed)
      candidates.push({ route: bowed, deviation: Math.abs(laneY - averageY), shapePenalty: 24 })
    const corridor = corridorSegments(start, end, sourceBranch, laneY, branchLength)
    if (corridor)
      candidates.push({
        route: corridor,
        deviation: Math.abs(laneY - averageY),
        shapePenalty: 96,
      })
  }

  const occupancy = buildRouteOccupancy(occupiedRoutes, edgeClearance)
  let best = direct
  let bestScore = Number.POSITIVE_INFINITY
  for (const candidate of candidates) {
    const nodeCollisions = routeCollisionCount(candidate.route, relevant, clearance)
    const overlapPenalty = routeOverlapPenaltyWithOccupancy(
      candidate.route,
      occupancy,
      edgeClearance,
      branchLength * 1.35,
    )
    const score =
      nodeCollisions * 1_000_000_000 +
      overlapPenalty * 720 +
      candidate.deviation * 1.8 +
      approximateRouteLength(candidate.route) * 0.08 +
      candidate.shapePenalty
    if (score < bestScore) {
      best = candidate.route
      bestScore = score
    }
  }
  return best
}

/**
 * 用一段三次贝塞尔穿过指定的纵向通道。第一控制点固定为共享分支点，
 * 第二控制点反解曲线中点，因此同源边共享初始切线，同时仍能形成宽缓的不同曲率。
 */
function singleCurveForLane(
  start: PertPoint,
  end: PertPoint,
  sourceBranch: PertPoint,
  laneY: number,
  branchLength: number,
): PertBezierSegment[] | null {
  const horizontalDistance = end.x - start.x
  if (horizontalDistance <= branchLength * 1.25) return null
  const targetControlX =
    end.x - Math.min(branchLength * 2.2, Math.max(branchLength * 0.7, horizontalDistance * 0.24))
  const targetControlY = (8 * laneY - 4 * start.y - end.y) / 3
  return [[start, sourceBranch, { x: targetControlX, y: targetControlY }, end]]
}

/** 两段连续三次贝塞尔构成宽缓弧线；laneY 不同会产生稳定、可辨的不同曲率。 */
function bowedSegments(
  start: PertPoint,
  end: PertPoint,
  sourceBranch: PertPoint,
  laneY: number,
  branchLength: number,
): PertBezierSegment[] | null {
  const targetBranch = { x: end.x - branchLength * 0.35, y: end.y }
  const usableStart = sourceBranch.x
  const usableEnd = targetBranch.x
  if (usableEnd - usableStart < branchLength) return null
  const middle = { x: (usableStart + usableEnd) / 2, y: laneY }
  const middleHandle = Math.min(branchLength * 1.6, Math.max(18, (usableEnd - usableStart) * 0.14))
  return [
    [start, sourceBranch, { x: middle.x - middleHandle, y: laneY }, middle],
    [middle, { x: middle.x + middleHandle, y: laneY }, targetBranch, end],
  ]
}

function corridorSegments(
  start: PertPoint,
  end: PertPoint,
  sourceBranch: PertPoint,
  corridorY: number,
  branchLength: number,
): PertBezierSegment[] | null {
  const entry = { x: start.x + branchLength * 2.4, y: corridorY }
  const exit = { x: end.x - branchLength * 2.4, y: corridorY }
  if (entry.x >= exit.x) return null
  const targetBranch = { x: end.x - branchLength * 0.35, y: end.y }
  return [
    [start, sourceBranch, { x: entry.x - branchLength, y: corridorY }, entry],
    [
      entry,
      { x: entry.x + (exit.x - entry.x) / 3, y: corridorY },
      { x: entry.x + ((exit.x - entry.x) * 2) / 3, y: corridorY },
      exit,
    ],
    [exit, { x: exit.x + branchLength, y: corridorY }, targetBranch, end],
  ]
}

interface RouteOccupancyPoint extends PertPoint {
  routeIndex: number
}

interface RouteOccupancy {
  cellSize: number
  grid: Map<string, RouteOccupancyPoint[]>
  routes: Array<{ start: PertPoint; end: PertPoint }>
}

function buildRouteOccupancy(routes: PertBezierSegment[][], clearance: number): RouteOccupancy {
  const cellSize = Math.max(8, clearance)
  const grid = new Map<string, RouteOccupancyPoint[]>()
  const metadata: Array<{ start: PertPoint; end: PertPoint }> = []
  routes.forEach((route, routeIndex) => {
    if (route.length === 0) return
    metadata.push({ start: route[0][0], end: route[route.length - 1][3] })
    for (const point of sampleBezierRoute(route, 16)) {
      const key = routeGridKey(point, cellSize)
      grid.set(key, [...(grid.get(key) ?? []), { ...point, routeIndex }])
    }
  })
  return { cellSize, grid, routes: metadata }
}

function routeGridKey(point: PertPoint, cellSize: number): string {
  return `${Math.floor(point.x / cellSize)}:${Math.floor(point.y / cellSize)}`
}

/** 计算候选路径与既有路径的近距离重叠惩罚；共享源/目标附近允许自然汇聚。 */
export function routeOverlapPenalty(
  candidate: PertBezierSegment[],
  occupiedRoutes: PertBezierSegment[][],
  clearance: number = PERT_DEFAULTS.edgeClearance,
  sharedEndpointDistance: number = PERT_DEFAULTS.branchLength * 1.35,
): number {
  return routeOverlapPenaltyWithOccupancy(
    candidate,
    buildRouteOccupancy(occupiedRoutes, clearance),
    clearance,
    sharedEndpointDistance,
  )
}

/** 统计发生至少一次内部相交的路径对；共享端点与采样段端点接触不计为交叉。 */
export function routeCrossingCount(routes: PertBezierSegment[][]): number {
  const sampled = routes.map((route) => sampleBezierRoute(route, 18))
  let crossings = 0
  for (let first = 0; first < sampled.length; first++) {
    for (let second = first + 1; second < sampled.length; second++) {
      if (polylineRoutesCross(sampled[first], sampled[second])) crossings++
    }
  }
  return crossings
}

function polylineRoutesCross(first: PertPoint[], second: PertPoint[]): boolean {
  for (let a = 1; a < first.length - 1; a++) {
    for (let b = 1; b < second.length - 1; b++) {
      if (pointDistanceSquared(first[a], second[b]) < 1e-8) return true
    }
  }
  for (let a = 1; a < first.length; a++) {
    for (let b = 1; b < second.length; b++) {
      if (segmentsProperlyCross(first[a - 1], first[a], second[b - 1], second[b])) return true
    }
  }
  return false
}

function segmentsProperlyCross(a: PertPoint, b: PertPoint, c: PertPoint, d: PertPoint): boolean {
  const denominator = (b.x - a.x) * (d.y - c.y) - (b.y - a.y) * (d.x - c.x)
  if (Math.abs(denominator) < 1e-7) return false
  const acX = c.x - a.x
  const acY = c.y - a.y
  const alongFirst = (acX * (d.y - c.y) - acY * (d.x - c.x)) / denominator
  const alongSecond = (acX * (b.y - a.y) - acY * (b.x - a.x)) / denominator
  const epsilon = 1e-4
  return (
    alongFirst > epsilon &&
    alongFirst < 1 - epsilon &&
    alongSecond > epsilon &&
    alongSecond < 1 - epsilon
  )
}

function routeOverlapPenaltyWithOccupancy(
  candidate: PertBezierSegment[],
  occupancy: RouteOccupancy,
  clearance: number,
  sharedEndpointDistance: number,
): number {
  if (candidate.length === 0 || occupancy.routes.length === 0) return 0
  const start = candidate[0][0]
  const end = candidate[candidate.length - 1][3]
  const neighborRange = Math.max(1, Math.ceil(clearance / occupancy.cellSize))
  const clearanceSquared = clearance ** 2
  let penalty = 0
  for (const point of sampleBezierRoute(candidate, 16)) {
    const cellX = Math.floor(point.x / occupancy.cellSize)
    const cellY = Math.floor(point.y / occupancy.cellSize)
    for (let offsetX = -neighborRange; offsetX <= neighborRange; offsetX++) {
      for (let offsetY = -neighborRange; offsetY <= neighborRange; offsetY++) {
        for (const occupied of occupancy.grid.get(`${cellX + offsetX}:${cellY + offsetY}`) ?? []) {
          const route = occupancy.routes[occupied.routeIndex]
          if (!route) continue
          const sharesStart = pointDistanceSquared(start, route.start) < 0.01
          const sharesEnd = pointDistanceSquared(end, route.end) < 0.01
          if (
            (sharesStart && pointDistanceSquared(point, start) < sharedEndpointDistance ** 2) ||
            (sharesEnd && pointDistanceSquared(point, end) < sharedEndpointDistance ** 2)
          )
            continue
          const distanceSquared = pointDistanceSquared(point, occupied)
          if (distanceSquared >= clearanceSquared) continue
          const proximity = 1 - Math.sqrt(distanceSquared) / clearance
          penalty += 1 + proximity * 4
        }
      }
    }
  }
  return penalty
}

function sampleBezierRoute(segments: PertBezierSegment[], stepsPerSegment: number): PertPoint[] {
  return segments.flatMap((segment, segmentIndex) =>
    Array.from({ length: stepsPerSegment + 1 }, (_, step) => {
      if (segmentIndex > 0 && step === 0) return null
      return cubicPoint(segment, step / stepsPerSegment)
    }).filter((point): point is PertPoint => point !== null),
  )
}

function approximateRouteLength(segments: PertBezierSegment[]): number {
  const points = sampleBezierRoute(segments, 8)
  let length = 0
  for (let index = 1; index < points.length; index++) {
    length += Math.sqrt(pointDistanceSquared(points[index - 1], points[index]))
  }
  return length
}

function pointDistanceSquared(a: PertPoint, b: PertPoint): number {
  return (a.x - b.x) ** 2 + (a.y - b.y) ** 2
}

export function routeCollisionCount(
  segments: PertBezierSegment[],
  nodes: PertNode[],
  clearance: number = PERT_DEFAULTS.obstacleClearance,
): number {
  const collided = new Set<string>()
  for (const segment of segments) {
    for (let step = 0; step <= 72; step++) {
      const point = cubicPoint(segment, step / 72)
      for (const node of nodes) {
        const exclusion = node.radius + clearance
        if ((point.x - node.x) ** 2 + (point.y - node.y) ** 2 < exclusion ** 2) {
          collided.add(node.id)
        }
      }
    }
  }
  return collided.size
}

function cubicPoint(segment: PertBezierSegment, t: number): PertPoint {
  const [start, control1, control2, end] = segment
  const inverse = 1 - t
  return {
    x:
      inverse ** 3 * start.x +
      3 * inverse ** 2 * t * control1.x +
      3 * inverse * t ** 2 * control2.x +
      t ** 3 * end.x,
    y:
      inverse ** 3 * start.y +
      3 * inverse ** 2 * t * control1.y +
      3 * inverse * t ** 2 * control2.y +
      t ** 3 * end.y,
  }
}
