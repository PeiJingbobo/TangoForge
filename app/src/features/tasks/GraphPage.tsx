import { useMemo } from 'react'
import { useNavigate } from 'react-router'
import { useGraph } from '@/hooks/useGraph'
import { useStateMachine } from '@/hooks/useStateMachine'
import { useEventInvalidator } from '@/hooks/useEvents'
import { useProjectId } from '@/hooks/useProject'
import { useTaskDrawerStore } from '@/stores/task-drawer'
import { GraphView } from '@/components/graph/graph-view'
import { PertView } from '@/components/graph/pert-view'
import { GraphViewToggle } from '@/components/graph/graph-view-toggle'
import { useGraphViewMode } from '@/hooks/useGraphViewMode'
import { Skeleton } from '@/components/ui/skeleton'

/** 任务全景图页（TF-028 力导向 / TF-055 PERT 分层，默认 PERT 可切换） */
export function GraphPage() {
  const pid = useProjectId()
  const navigate = useNavigate()
  const { data: graph, isLoading } = useGraph(pid)
  const { data: sm } = useStateMachine(pid)
  const openTaskDrawer = useTaskDrawerStore((st) => st.openDrawer)
  const [mode, setMode] = useGraphViewMode()
  useEventInvalidator(pid)

  const nodeCount = graph?.nodes.length ?? 0
  const clustered = useMemo(() => nodeCount > 500, [nodeCount])

  const openTask = (id: string) => openTaskDrawer({ taskId: id })
  /** 双击 → 路由全屏任务详情（/project/:workdir/tasks/:taskId） */
  const openTaskFull = (id: string) => {
    if (!pid) return
    navigate(`/project/${encodeURIComponent(pid)}/tasks/${id}`)
  }

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
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-h2 text-foreground">任务全景图</h1>
          <p className="mt-1 text-caption text-muted-foreground">
            {nodeCount} 个节点 · 滚轮缩放 / 拖拽平移 · 单击打开任务 · 双击全屏
            {clustered && ' · 节点过多已按状态聚簇'}
          </p>
        </div>
        <GraphViewToggle mode={mode} onChange={setMode} />
      </div>
      {nodeCount === 0 ? (
        <div className="rounded-2xl border border-dashed border-border p-16 text-center text-body text-muted-foreground">
          项目暂无任务，导入 Markdown 后这里会呈现任务依赖全景。
        </div>
      ) : mode === 'pert' ? (
        <PertView
          data={graph ?? { nodes: [], edges: [] }}
          states={sm?.States ?? []}
          onSelect={openTask}
          onOpenFull={openTaskFull}
        />
      ) : (
        <GraphView
          data={graph ?? { nodes: [], edges: [] }}
          states={sm?.States ?? []}
          onSelect={openTask}
        />
      )}
    </div>
  )
}
