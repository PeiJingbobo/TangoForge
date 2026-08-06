import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { Archive, RotateCcw } from 'lucide-react'
import { ApiError } from '@/api/client'
import { flattenTree } from '@/components/kanban/tree-utils'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import { TaskForm } from '@/features/tasks/TaskForm'
import { useTask, useTasks, useUpdateTask, useArchiveTask, useRestoreTask } from '@/hooks/useTasks'
import { useStateMachine } from '@/hooks/useStateMachine'
import { useEventInvalidator } from '@/hooks/useEvents'
import { useProjectId } from '@/hooks/useProject'
import type { UpdateTaskInput } from '@/types/task'

/**
 * 任务详情（TF-026，UI-VISION 场景 C）：
 * 阅读流（标题 → meta → 描述 → 子块）+ 属性栏分割线分组 + sticky 底部操作。
 */
export function TaskDetail({ taskId }: { taskId: string }) {
  const pid = useProjectId()
  const navigate = useNavigate()
  const { data: task, isLoading } = useTask(taskId, pid)
  const { data: sm } = useStateMachine(pid)
  const { data: taskData } = useTasks(undefined, pid)
  const updateTask = useUpdateTask(pid)
  const archiveTask = useArchiveTask(pid)
  const restoreTask = useRestoreTask(pid)
  const [busy, setBusy] = useState(false)

  useEventInvalidator(pid)

  const allTasks = useMemo(() => flattenTree(taskData?.tree ?? []), [taskData])
  const states = useMemo(() => sm?.States ?? [], [sm])
  const dependNames = useMemo(() => {
    const byId = new Map(allTasks.map((t) => [t.id, t.title]))
    return (task?.depends_on ?? []).map((id) => byId.get(id) ?? id)
  }, [allTasks, task])

  if (isLoading || !task) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-9 w-2/3" />
        <Skeleton className="h-5 w-1/3" />
        <Skeleton className="h-40 w-full" />
      </div>
    )
  }

  const handleSubmit = (body: UpdateTaskInput & { status?: string }) => {
    setBusy(true)
    updateTask.mutate(
      { id: task.id, body },
      {
        onSuccess: (t) => {
          toast.success('已保存', { description: t.title })
        },
        onError: (err) => {
          // 依赖环/非法流转等业务错误：toast 展示后端 message（含无环提示语义）
          toast.error(err instanceof Error ? err.message : '保存失败', {
            description:
              err instanceof ApiError && err.code === 'CIRCULAR_DEPENDENCY'
                ? '已检测到循环依赖，请调整依赖关系'
                : undefined,
          })
        },
        onSettled: () => setBusy(false),
      },
    )
  }

  const confirmArchive = () => {
    const children = allTasks.filter((t) => t.parent_id === task.id)
    const msg =
      children.length > 0
        ? `该任务有 ${children.length} 个子任务，归档后将级联置为顶层任务。确认归档？`
        : '确认归档该任务？（归档后可在回收站还原）'
    if (!window.confirm(msg)) return
    setBusy(true)
    archiveTask.mutate(task.id, {
      onSuccess: (r) => {
        toast.success('已归档', {
          description:
            r.children_cleared > 0
              ? `级联置空 ${r.children_cleared} 个子任务`
              : r.dependent_count > 0
                ? `仍有 ${r.dependent_count} 个任务依赖它`
                : undefined,
        })
        navigate(`/project/${encodeURIComponent(pid ?? '')}/kanban`)
      },
      onError: (err) => toast.error(err instanceof Error ? err.message : '归档失败'),
      onSettled: () => setBusy(false),
    })
  }

  const confirmRestore = () => {
    setBusy(true)
    restoreTask.mutate(
      { id: task.id },
      {
        onSuccess: (t) => toast.success('已还原', { description: t.status }),
        onError: (err) => toast.error(err instanceof Error ? err.message : '还原失败'),
        onSettled: () => setBusy(false),
      },
    )
  }

  const archiveAction =
    task.status === 'archived' ? (
      <Button variant="ghost" onClick={confirmRestore} disabled={busy} className="text-success-ink">
        <RotateCcw className="size-4" /> 还原
      </Button>
    ) : (
      <Button
        variant="ghost"
        onClick={confirmArchive}
        disabled={busy}
        className="text-destructive-ink"
      >
        <Archive className="size-4" /> 归档
      </Button>
    )

  return (
    <div className="grid grid-cols-1 gap-10 lg:grid-cols-[minmax(0,1fr)_260px]">
      {/* 左：阅读流 + 编辑 */}
      <div className="min-w-0">
        <TaskForm
          task={task}
          states={states}
          allTasks={allTasks}
          saving={busy}
          onSubmit={handleSubmit}
          archiveAction={archiveAction}
        />
      </div>

      {/* 右：属性栏（分割线分组，UI-VISION §2.5） */}
      <aside className="hidden lg:block">
        <div className="space-y-0">
          <div className="py-3">
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">项目</span>
              <span className="font-medium">{pid?.split(/[\\/]/).pop()}</span>
            </div>
            <div className="mt-2 flex justify-between text-sm">
              <span className="text-muted-foreground">创建</span>
              <span className="font-medium">{new Date(task.created_at).toLocaleString()}</span>
            </div>
            <div className="mt-2 flex justify-between text-sm">
              <span className="text-muted-foreground">更新</span>
              <span className="font-medium">{new Date(task.updated_at).toLocaleString()}</span>
            </div>
          </div>
          <Separator />
          <div className="py-3">
            <div className="flex justify-between gap-3 text-sm">
              <span className="shrink-0 text-muted-foreground">依赖</span>
              <span className="text-right font-medium">
                {dependNames.length > 0 ? dependNames.join('、') : '无'}
              </span>
            </div>
          </div>
          <Separator />
          <div className="py-3">
            <div className="text-sm text-muted-foreground">来源</div>
            <div className="mt-1 break-all font-mono text-xs">
              {task.source_file
                ? `${task.source_file}${task.source_section ? ` / ${task.source_section}` : ''}`
                : '手动创建'}
            </div>
          </div>
        </div>
      </aside>
    </div>
  )
}

export function TaskDetailPage() {
  const { taskId } = useParams()
  return <TaskDetail taskId={taskId ?? ''} />
}
