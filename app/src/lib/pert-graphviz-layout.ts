import { instance, type Graph, type Viz } from '@viz-js/viz'
import type { GraphData } from '../types/models'
import {
  PERT_DEFAULTS,
  pertEdgeId,
  pertLayout,
  routeCollisionCount,
  type PertBezierSegment,
  type PertLayout,
  type PertNode,
  type PertPoint,
} from './pert-layout'

interface GraphvizDrawOperation {
  op?: string
  points?: [number, number][]
}

interface GraphvizObject {
  name?: string
  pos?: string
}

interface GraphvizEdge {
  id?: string
  tail?: number
  head?: number
  _draw_?: GraphvizDrawOperation[]
}

interface GraphvizJSON {
  bb?: string
  objects?: GraphvizObject[]
  edges?: GraphvizEdge[]
}

let vizPromise: Promise<Viz> | null = null

const GRAPHVIZ_POINTS_PER_SEGMENT = 8
const TARGET_ARROW_GAP = 11
const GRAPHVIZ_ROUTE_SIMPLIFY_TOLERANCES = [140, 96, 64, 36, 18, 0]

function getViz(): Promise<Viz> {
  vizPromise ??= instance()
  return vizPromise
}

/**
 * 使用 Graphviz dot 完成 proper-layer、交叉最小化、节点避让和 B 样条路由。
 * WASM 布局按需加载；调用方应保留同步 pertLayout 作为加载/失败兜底。
 */
export async function graphvizPertLayout(data: GraphData): Promise<PertLayout> {
  const semantic = pertLayout(data)
  if (semantic.nodes.length === 0 || semantic.hasCycle) return semantic

  const dependencyEdges = semantic.edges.map((edge) => ({ from: edge.from, to: edge.to }))
  const semanticNodeById = new Map(semantic.nodes.map((node) => [node.id, node]))
  const graph: Graph = {
    directed: true,
    strict: true,
    graphAttributes: {
      rankdir: 'LR',
      splines: 'spline',
      // dot 会为跨层边插入虚拟节点；实际节点保持明显疏离，同时避免虚拟节点把高度放大数倍。
      nodesep: 1.6,
      ranksep: PERT_DEFAULTS.layerGap / 72,
      pad: PERT_DEFAULTS.canvasPadding / 72,
      outputorder: 'edgesfirst',
      overlap: 'false',
      pack: 128,
      packmode: 'node',
      remincross: 'true',
      newrank: 'true',
      mclimit: 32,
      nslimit: 8,
      nslimit1: 8,
      searchsize: 100,
    },
    nodeAttributes: {
      shape: 'circle',
      fixedsize: 'true',
      width: PERT_DEFAULTS.nodeDiameter / 72,
      height: PERT_DEFAULTS.nodeDiameter / 72,
      label: '',
    },
    edgeAttributes: {
      tailport: 'e',
      headport: 'w',
    },
    nodes: semantic.nodes
      .slice()
      .sort(
        (a, b) => a.layer - b.layer || a.indexInLayer - b.indexInLayer || a.id.localeCompare(b.id),
      )
      .map((node) => ({ name: node.id })),
    edges: dependencyEdges
      .slice()
      .sort((a, b) => {
        const sourceA = semanticNodeById.get(a.from)
        const sourceB = semanticNodeById.get(b.from)
        const targetA = semanticNodeById.get(a.to)
        const targetB = semanticNodeById.get(b.to)
        return (
          (sourceA?.layer ?? 0) - (sourceB?.layer ?? 0) ||
          (sourceA?.indexInLayer ?? 0) - (sourceB?.indexInLayer ?? 0) ||
          (targetA?.indexInLayer ?? 0) - (targetB?.indexInLayer ?? 0) ||
          pertEdgeId(a.from, a.to).localeCompare(pertEdgeId(b.from, b.to))
        )
      })
      .map((edge) => ({
        tail: edge.from,
        head: edge.to,
        attributes: { id: pertEdgeId(edge.from, edge.to) },
      })),
  }

  const viz = await getViz()
  const output = viz.renderJSON(graph, { engine: 'dot', yInvert: true }) as GraphvizJSON
  const objects = output.objects ?? []
  const positioned = new Map(
    objects.flatMap((object) => {
      const point = parsePoint(object.pos)
      return object.name && point ? [[object.name, point] as const] : []
    }),
  )
  if (positioned.size !== semantic.nodes.length) {
    throw new Error('Graphviz 未返回完整的 PERT 节点坐标')
  }

  const padding = PERT_DEFAULTS.canvasPadding
  const nodes = semantic.nodes.map((node) => {
    const point = positioned.get(node.id)!
    return {
      ...node,
      x: point.x + padding + deterministicOffset(`${node.id}:graphviz-x`, PERT_DEFAULTS.scatterX),
      y: point.y + padding,
    }
  })
  const nodeById = new Map(nodes.map((node) => [node.id, node]))
  const graphvizEdges = new Map(
    (output.edges ?? []).flatMap((edge) => {
      const id = edge.id ?? edgeIdFromIndexes(edge, objects)
      return id ? [[id, edge] as const] : []
    }),
  )
  const fallbackEdges = new Map(semantic.edges.map((edge) => [edge.id, edge]))
  const edges = dependencyEdges.map((edge) => {
    const id = pertEdgeId(edge.from, edge.to)
    const source = nodeById.get(edge.from)!
    const target = nodeById.get(edge.to)!
    const rendered = graphvizEdges.get(id)
    const graphvizSegments = rendered ? drawOperationsToBezier(rendered._draw_ ?? [], padding) : []
    const segments = smoothGraphvizRoute(graphvizSegments, source, target, nodes)
    const fallback = fallbackEdges.get(id)!
    return {
      ...fallback,
      segments: segments.length > 0 ? segments : fallback.segments,
      rootId: source.rootId,
      routed: segments.length > 1 || source.layer + 1 < target.layer,
    }
  })

  const bounds = parseBoundingBox(output.bb)
  const routePoints = edges.flatMap((edge) => sampleBezierRoute(edge.segments, 24))
  const maxX = Math.max(
    0,
    ...nodes.map((node) => node.x + node.radius),
    ...routePoints.map((point) => point.x),
  )
  const maxY = Math.max(
    0,
    ...nodes.map((node) => node.y + node.radius + 24),
    ...routePoints.map((point) => point.y),
  )
  const width = Math.max(semantic.width, (bounds?.width ?? 0) + padding * 2, maxX + padding)
  const height = Math.max(semantic.height, (bounds?.height ?? 0) + padding * 2, maxY + padding)
  return { ...semantic, nodes, edges, width, height }
}

function parsePoint(value: string | undefined): PertPoint | null {
  if (!value) return null
  const [x, y] = value.split(',').map(Number)
  return Number.isFinite(x) && Number.isFinite(y) ? { x, y } : null
}

function parseBoundingBox(value: string | undefined): { width: number; height: number } | null {
  if (!value) return null
  const [x0, y0, x1, y1] = value.split(',').map(Number)
  if (![x0, y0, x1, y1].every(Number.isFinite)) return null
  return { width: Math.abs(x1 - x0), height: Math.abs(y1 - y0) }
}

function edgeIdFromIndexes(edge: GraphvizEdge, objects: GraphvizObject[]): string | null {
  const from = edge.tail === undefined ? undefined : objects[edge.tail]?.name
  const to = edge.head === undefined ? undefined : objects[edge.head]?.name
  return from && to ? pertEdgeId(from, to) : null
}

function drawOperationsToBezier(
  operations: GraphvizDrawOperation[],
  padding: number,
): PertBezierSegment[] {
  const segments: PertBezierSegment[] = []
  for (const operation of operations) {
    if (operation.op !== 'b' || !operation.points || operation.points.length < 4) continue
    for (let index = 0; index + 3 < operation.points.length; index += 3) {
      const points = operation.points.slice(index, index + 4).map(([x, y]) => ({
        x: x + padding,
        y: y + padding,
      })) as PertBezierSegment
      segments.push(points)
    }
  }
  return segments
}

function smoothGraphvizRoute(
  graphvizSegments: PertBezierSegment[],
  source: PertNode,
  target: PertNode,
  nodes: PertNode[],
): PertBezierSegment[] {
  if (graphvizSegments.length === 0) return []
  const start = { x: source.x + source.radius, y: source.y }
  const end = { x: target.x - target.radius - TARGET_ARROW_GAP, y: target.y }
  const sourceBranch = { x: start.x + PERT_DEFAULTS.branchLength, y: start.y }
  const targetBranchLength = Math.min(
    PERT_DEFAULTS.branchLength,
    Math.max(36, end.x - start.x) * 0.3,
  )
  const rawGuide = sampleBezierRoute(graphvizSegments, GRAPHVIZ_POINTS_PER_SEGMENT)
  const guide = retargetGuidePoints(rawGuide, start, end)
  const obstacles = nodes.filter((node) => node.id !== source.id && node.id !== target.id)

  for (const tolerance of GRAPHVIZ_ROUTE_SIMPLIFY_TOLERANCES) {
    const simplified = simplifyRoutePoints(guide, tolerance)
    const fitted = fitNaturalBezierRoute(simplified, PERT_DEFAULTS.branchLength, targetBranchLength)
    if (fitted.length === 0) continue
    fitted[0][1] = sourceBranch
    if (routeCollisionCount(fitted, obstacles, 6) === 0) return fitted
  }

  return retargetGraphvizSegments(graphvizSegments, start, sourceBranch, end, targetBranchLength)
}

function retargetGuidePoints(points: PertPoint[], start: PertPoint, end: PertPoint): PertPoint[] {
  const middle = points.slice(1, -1).filter((point) => point.x > start.x + 8 && point.x < end.x - 8)
  const guide = [start, ...middle, end]
  return removeNearDuplicatePoints(guide, 12)
}

function retargetGraphvizSegments(
  segments: PertBezierSegment[],
  start: PertPoint,
  sourceBranch: PertPoint,
  end: PertPoint,
  targetBranchLength: number,
): PertBezierSegment[] {
  const cloned = segments.map(
    (segment) => segment.map((point) => ({ ...point })) as PertBezierSegment,
  )
  if (cloned.length === 0) return cloned
  const originalStart = cloned[0][0]
  const originalEnd = cloned[cloned.length - 1][3]
  const startDelta = { x: start.x - originalStart.x, y: start.y - originalStart.y }
  const endDelta = { x: end.x - originalEnd.x, y: end.y - originalEnd.y }
  cloned[0][0] = start
  cloned[0][1] = sourceBranch
  cloned[cloned.length - 1][2] = { x: end.x - targetBranchLength, y: end.y }
  cloned[cloned.length - 1][3] = end
  for (let index = 0; index < cloned.length; index++) {
    const weight = cloned.length === 1 ? 0.5 : index / (cloned.length - 1)
    const delta = {
      x: startDelta.x * (1 - weight) + endDelta.x * weight,
      y: startDelta.y * (1 - weight) + endDelta.y * weight,
    }
    for (let pointIndex = 0; pointIndex < 4; pointIndex++) {
      if ((index === 0 && pointIndex <= 1) || (index === cloned.length - 1 && pointIndex >= 2))
        continue
      cloned[index][pointIndex] = {
        x: cloned[index][pointIndex].x + delta.x,
        y: cloned[index][pointIndex].y + delta.y,
      }
    }
  }
  return cloned
}

function simplifyRoutePoints(points: PertPoint[], tolerance: number): PertPoint[] {
  if (points.length <= 2 || tolerance <= 0) return removeNearDuplicatePoints(points, 8)
  const keep = new Set<number>([0, points.length - 1])
  simplifyRange(points, 0, points.length - 1, tolerance ** 2, keep)
  return [...keep].sort((a, b) => a - b).map((index) => points[index])
}

function simplifyRange(
  points: PertPoint[],
  startIndex: number,
  endIndex: number,
  toleranceSquared: number,
  keep: Set<number>,
) {
  if (endIndex - startIndex <= 1) return
  let farthestIndex = -1
  let farthestDistance = 0
  for (let index = startIndex + 1; index < endIndex; index++) {
    const distance = perpendicularDistanceSquared(
      points[index],
      points[startIndex],
      points[endIndex],
    )
    if (distance > farthestDistance) {
      farthestDistance = distance
      farthestIndex = index
    }
  }
  if (farthestDistance <= toleranceSquared || farthestIndex < 0) return
  keep.add(farthestIndex)
  simplifyRange(points, startIndex, farthestIndex, toleranceSquared, keep)
  simplifyRange(points, farthestIndex, endIndex, toleranceSquared, keep)
}

function fitNaturalBezierRoute(
  points: PertPoint[],
  sourceBranchLength: number,
  targetBranchLength: number,
): PertBezierSegment[] {
  const guide = removeNearDuplicatePoints(points, 4)
  if (guide.length < 2) return []
  if (guide.length === 2) {
    const [start, end] = guide
    return [
      [
        start,
        { x: start.x + sourceBranchLength, y: start.y },
        { x: end.x - targetBranchLength, y: end.y },
        end,
      ],
    ]
  }
  const parameter = chordParameters(guide)
  const secondX = solveClampedSecondDerivatives(
    guide.map((point) => point.x),
    parameter,
    sourceBranchLength,
    targetBranchLength,
  )
  const secondY = solveClampedSecondDerivatives(
    guide.map((point) => point.y),
    parameter,
    0,
    0,
  )
  const segments: PertBezierSegment[] = []
  for (let index = 0; index < guide.length - 1; index++) {
    const h = parameter[index + 1] - parameter[index]
    const slopeX = (guide[index + 1].x - guide[index].x) / h
    const slopeY = (guide[index + 1].y - guide[index].y) / h
    const derivativeStart = {
      x: slopeX - (h * (2 * secondX[index] + secondX[index + 1])) / 6,
      y: slopeY - (h * (2 * secondY[index] + secondY[index + 1])) / 6,
    }
    const derivativeEnd = {
      x: slopeX + (h * (secondX[index] + 2 * secondX[index + 1])) / 6,
      y: slopeY + (h * (secondY[index] + 2 * secondY[index + 1])) / 6,
    }
    segments.push([
      guide[index],
      {
        x: guide[index].x + (derivativeStart.x * h) / 3,
        y: guide[index].y + (derivativeStart.y * h) / 3,
      },
      {
        x: guide[index + 1].x - (derivativeEnd.x * h) / 3,
        y: guide[index + 1].y - (derivativeEnd.y * h) / 3,
      },
      guide[index + 1],
    ])
  }
  return segments
}

function chordParameters(points: PertPoint[]): number[] {
  const parameter = [0]
  for (let index = 1; index < points.length; index++) {
    parameter[index] =
      parameter[index - 1] +
      Math.max(1, Math.sqrt(pointDistanceSquared(points[index - 1], points[index])))
  }
  return parameter
}

function solveClampedSecondDerivatives(
  values: number[],
  parameter: number[],
  startHandleLength: number,
  endHandleLength: number,
): number[] {
  const count = values.length
  const lower = new Array<number>(count).fill(0)
  const diagonal = new Array<number>(count).fill(0)
  const upper = new Array<number>(count).fill(0)
  const rhs = new Array<number>(count).fill(0)
  const firstH = parameter[1] - parameter[0]
  const lastH = parameter[count - 1] - parameter[count - 2]
  diagonal[0] = 2 * firstH
  upper[0] = firstH
  rhs[0] = 6 * ((values[1] - values[0]) / firstH - (startHandleLength * 3) / firstH)
  for (let index = 1; index < count - 1; index++) {
    const previousH = parameter[index] - parameter[index - 1]
    const nextH = parameter[index + 1] - parameter[index]
    lower[index] = previousH
    diagonal[index] = 2 * (previousH + nextH)
    upper[index] = nextH
    rhs[index] =
      6 *
      ((values[index + 1] - values[index]) / nextH -
        (values[index] - values[index - 1]) / previousH)
  }
  lower[count - 1] = lastH
  diagonal[count - 1] = 2 * lastH
  rhs[count - 1] =
    6 * ((endHandleLength * 3) / lastH - (values[count - 1] - values[count - 2]) / lastH)
  return solveTridiagonal(lower, diagonal, upper, rhs)
}

function solveTridiagonal(
  lower: number[],
  diagonal: number[],
  upper: number[],
  rhs: number[],
): number[] {
  const n = diagonal.length
  const c = new Array<number>(n).fill(0)
  const d = new Array<number>(n).fill(0)
  c[0] = upper[0] / diagonal[0]
  d[0] = rhs[0] / diagonal[0]
  for (let index = 1; index < n; index++) {
    const denominator = diagonal[index] - lower[index] * c[index - 1]
    c[index] = index === n - 1 ? 0 : upper[index] / denominator
    d[index] = (rhs[index] - lower[index] * d[index - 1]) / denominator
  }
  const result = new Array<number>(n).fill(0)
  result[n - 1] = d[n - 1]
  for (let index = n - 2; index >= 0; index--)
    result[index] = d[index] - c[index] * result[index + 1]
  return result
}

function sampleBezierRoute(segments: PertBezierSegment[], stepsPerSegment: number): PertPoint[] {
  return segments.flatMap((segment, segmentIndex) =>
    Array.from({ length: stepsPerSegment + 1 }, (_, step) => {
      if (segmentIndex > 0 && step === 0) return null
      return cubicPoint(segment, step / stepsPerSegment)
    }).filter((point): point is PertPoint => point !== null),
  )
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

function removeNearDuplicatePoints(points: PertPoint[], minDistance: number): PertPoint[] {
  const result: PertPoint[] = []
  const minDistanceSquared = minDistance ** 2
  for (const point of points) {
    const previous = result[result.length - 1]
    if (!previous || pointDistanceSquared(previous, point) >= minDistanceSquared) result.push(point)
  }
  if (result.length > 1 && !samePoint(result[result.length - 1], points[points.length - 1])) {
    result.push(points[points.length - 1])
  }
  return result
}

function perpendicularDistanceSquared(point: PertPoint, start: PertPoint, end: PertPoint): number {
  const lengthSquared = pointDistanceSquared(start, end)
  if (lengthSquared === 0) return pointDistanceSquared(point, start)
  const t = Math.max(
    0,
    Math.min(
      1,
      ((point.x - start.x) * (end.x - start.x) + (point.y - start.y) * (end.y - start.y)) /
        lengthSquared,
    ),
  )
  return pointDistanceSquared(point, {
    x: start.x + (end.x - start.x) * t,
    y: start.y + (end.y - start.y) * t,
  })
}

function pointDistanceSquared(a: PertPoint, b: PertPoint): number {
  return (a.x - b.x) ** 2 + (a.y - b.y) ** 2
}

function samePoint(a: PertPoint, b: PertPoint): boolean {
  return Math.abs(a.x - b.x) < 0.001 && Math.abs(a.y - b.y) < 0.001
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
