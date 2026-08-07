import { useMemo } from 'react'
import { useGraph } from '@/hooks/useGraph'
import { useStateMachine } from '@/hooks/useStateMachine'
import { useEventInvalidator } from '@/hooks/useEvents'
import { useProjectId } from '@/hooks/useProject'
import { useTaskDrawerStore } from '@/stores/task-drawer'
import { GraphView } from '@/components/graph/graph-view'
import { Skeleton } from '@/components/ui/skeleton'

/** 全景地图页（TF-028）：D3 力导向全量任务图 */
export function GraphPage() {
  const pid = useProjectId()
  const { data: graph, isLoading } = useGraph(pid)
  const { data: sm } = useStateMachine(pid)
  const openTaskDrawer = useTaskDrawerStore((st) => st.openDrawer)
  useEventInvalidator(pid)

  const nodeCount = graph?.nodes.length ?? 0

  const clustered = useMemo(() => nodeCount > 300, [nodeCount])

  if (isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-8 w-40" />
        <Skeleton className="h-[520px] w-full rounded-2xl" />
      </div>
    )
  }

  return (
    <div>
      <div className="mb-4">
        <h1 className="text-h2 text-foreground">任务全景图</h1>
        <p className="mt-1 text-caption text-muted-foreground">
          {nodeCount} 个节点 · 力导向布局（拖拽/滚轮缩放）· 点击节点打开任务
          {clustered && ' · 节点过多已按状态聚簇'}
        </p>
      </div>
      {nodeCount === 0 ? (
        <div className="rounded-2xl border border-dashed border-border p-16 text-center text-body text-muted-foreground">
          项目暂无任务，导入 Markdown 后这里会呈现任务依赖全景。
        </div>
      ) : (
        <GraphView
          data={graph ?? { nodes: [], edges: [] }}
          states={sm?.States ?? []}
          onSelect={(id) => openTaskDrawer({ taskId: id })}
        />
      )}
    </div>
  )
}
