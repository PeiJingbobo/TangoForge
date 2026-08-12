import { useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { Archive, ArrowLeft, RotateCcw, X } from 'lucide-react'
import { ApiError } from '@/api/client'
import { flattenTree } from '@/components/kanban/tree-utils'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { TaskForm, type TaskFormHandle } from '@/features/tasks/TaskForm'
import {
  TaskKnowledgeSection,
  type TaskKnowledgeDoc,
} from '@/features/knowledge/TaskKnowledgeSection'
import { useTask, useTasks, useUpdateTask, useArchiveTask, useRestoreTask } from '@/hooks/useTasks'
import { useStateMachine } from '@/hooks/useStateMachine'
import { useEventInvalidator } from '@/hooks/useEvents'
import { useProjectId } from '@/hooks/useProject'
import { getTitleBarHeight } from '@/lib/window-chrome'
import { useTaskDrawerStore, type TaskDrawerMode } from '@/stores/task-drawer'
import type { Task, UpdateTaskInput } from '@/types/task'

/** 任务详情内嵌知识库文档摘要（/api/tasks/:id knowledge_documents） */
interface TaskWithKnowledge extends Task {
  knowledge_documents?: TaskKnowledgeDoc[]
}

/**
 * 任务详情抽屉（TF 改造）：全局右侧抽屉形态。
 * - props 化：taskId 优先级高（内部 useTask 加载 + useUpdateTask 保存）；
 *   传入 task 对象时直接使用，编辑完成经 onSaved(latest) 回调最新详情（不内部发请求）；
 * - mode：'edit'（默认，可编辑保存）/ 'read'（只读展示）；
 * - stacked：Dialog 页面堆栈中的内层（非顶层），关闭遮罩保留下层可见；
 * - 复用 TaskForm（行内编辑 + 差异提交）。
 */
export interface TaskDetailDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** 任务 id（优先级高：内部加载详情并内部保存） */
  taskId?: string
  /** 任务详情对象（传入时直接使用；编辑完成经 onSaved 回调最新详情） */
  task?: Task
  mode?: TaskDrawerMode
  /** 堆栈内层（非顶层）：不渲染遮罩 */
  stacked?: boolean
  /** 堆栈深度（0=根层）；用于 z-index 递增，保证层级关系稳定 */
  level?: number
  /** 编辑保存成功回调（task 对象模式必需；taskId 模式保存后也回调最新详情） */
  onSaved?: (task: Task) => void
}

export function TaskDetailDrawer({
  open,
  onOpenChange,
  taskId,
  task: taskProp,
  mode = 'edit',
  stacked = false,
  level = 0,
  onSaved,
}: TaskDetailDrawerProps) {
  const pid = useProjectId()
  // taskId 模式加载（open=false 时不请求）；task 对象模式不加载
  const { data: loaded } = useTask(taskId && open ? taskId : undefined, pid)
  const { data: taskData } = useTasks(undefined, pid)
  const { data: sm } = useStateMachine(pid)
  const updateTask = useUpdateTask(pid)
  const archiveTask = useArchiveTask(pid)
  const restoreTask = useRestoreTask(pid)
  const [busy, setBusy] = useState(false)
  const [dirty, setDirty] = useState(false)
  const formRef = useRef<TaskFormHandle>(null)

  useEventInvalidator(pid)

  // 桌面自绘标题栏高度：全高抽屉头部需预留顶部内边距，避免被标题栏遮挡
  const titleBarH = getTitleBarHeight()

  // taskId 模式：用加载数据；task 对象模式：直接用 props（优先级 id > 对象，但对象非空时以对象为展示源）
  const task = taskProp ?? loaded

  const allTasks = useMemo(() => flattenTree(taskData?.tree ?? []), [taskData])
  const states = useMemo(() => sm?.States ?? [], [sm])
  const dependNames = useMemo(() => {
    const byId = new Map(allTasks.map((t) => [t.id, t.title]))
    return (task?.depends_on ?? []).map((id) => byId.get(id) ?? id)
  }, [allTasks, task])

  const handleSubmit = (body: UpdateTaskInput & { status?: string }) => {
    if (!task) return
    if (taskProp) {
      // 对象模式：不发请求，显式构造最新详情经回调交给调用方（避免 body 展开类型漂移）
      const latest: Task = {
        ...task,
        title: body.title ?? task.title,
        description: body.description ?? task.description,
        status: body.status ?? task.status,
        priority: body.priority === undefined ? task.priority : Number(body.priority),
        assignee: body.assignee ?? task.assignee,
        tags: body.tags ?? task.tags,
        depends_on: body.depends_on ?? task.depends_on,
      }
      onSaved?.(latest)
      return
    }
    // taskId 模式：内部保存
    setBusy(true)
    updateTask.mutate(
      { id: task.id, body },
      {
        onSuccess: (t) => {
          toast.success('已保存', { description: t.title })
          onSaved?.(t)
        },
        onError: (err) => {
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
    if (!task) return
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
        onOpenChange(false)
      },
      onError: (err) => toast.error(err instanceof Error ? err.message : '归档失败'),
      onSettled: () => setBusy(false),
    })
  }

  const confirmRestore = () => {
    if (!task) return
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
    task && mode === 'edit' ? (
      task.status === 'archived' ? (
        <Button
          variant="ghost"
          onClick={confirmRestore}
          disabled={busy}
          className="text-success-ink"
        >
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
    ) : null

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        overlay={!stacked}
        zIndex={50 + level * 10}
        style={{ paddingTop: titleBarH }}
        showCloseButton={false}
        className="flex w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-xl"
      >
        <SheetHeader className="border-b border-divider px-6 py-4">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => onOpenChange(false)}
              aria-label="返回关闭详情"
              title="返回"
              className="grid size-8 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
            >
              <ArrowLeft className="size-4" />
            </button>
            <SheetTitle className="text-base">任务详情</SheetTitle>
            {/* 关闭：与标题同一行、右侧对齐、垂直居中（避开标题栏遮挡） */}
            <button
              type="button"
              onClick={() => onOpenChange(false)}
              aria-label="关闭详情"
              title="关闭"
              className="ml-auto grid size-8 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
            >
              <X className="size-4" />
            </button>
          </div>
          <SheetDescription>
            {task ? `${task.status}${task.priority > 0 ? ` · P${task.priority}` : ''}` : '加载中…'}
            {mode === 'read' ? ' · 只读' : ' · 编辑'}
          </SheetDescription>
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
          {!task ? (
            <div className="space-y-4">
              <Skeleton className="h-9 w-2/3" />
              <Skeleton className="h-5 w-1/3" />
              <Skeleton className="h-40 w-full" />
            </div>
          ) : (
            <div className="space-y-6">
              <TaskForm
                ref={formRef}
                task={task}
                states={states}
                allTasks={allTasks}
                readOnly={mode === 'read'}
                onSubmit={handleSubmit}
                onDirtyChange={setDirty}
              />

              {/* 资料区（TF-052：任务关联知识库文档） */}
              <TaskKnowledgeSection
                taskId={task.id}
                documents={(task as TaskWithKnowledge).knowledge_documents}
                project={pid ?? ''}
              />

              {/* 元信息（只读，抽屉内纵向排列） */}
              <div className="rounded-xl border border-divider p-4 text-sm">
                <div className="flex justify-between gap-3">
                  <span className="shrink-0 text-muted-foreground">依赖</span>
                  <span className="min-w-0 text-right font-medium break-words">
                    {dependNames.length > 0 ? dependNames.join('、') : '无'}
                  </span>
                </div>
                <div className="mt-2 flex justify-between gap-3">
                  <span className="shrink-0 text-muted-foreground">创建</span>
                  <span className="font-medium">{new Date(task.created_at).toLocaleString()}</span>
                </div>
                <div className="mt-2 flex justify-between gap-3">
                  <span className="shrink-0 text-muted-foreground">更新</span>
                  <span className="font-medium">{new Date(task.updated_at).toLocaleString()}</span>
                </div>
                <div className="mt-2 text-muted-foreground">来源</div>
                <div className="mt-0.5 break-all font-mono text-xs">
                  {task.source_file
                    ? `${task.source_file}${task.source_section ? ` / ${task.source_section}` : ''}`
                    : '手动创建'}
                </div>
              </div>
            </div>
          )}
        </div>

        {/* footer 固定抽屉底部（滚动区外）：归档左对齐，关闭/保存右对齐 */}
        <SheetFooter className="border-t border-divider bg-muted/60 px-6 py-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-2">{mode === 'edit' && archiveAction}</div>
          <div className="flex items-center gap-2">
            <Button variant="ghost" onClick={() => onOpenChange(false)}>
              关闭
            </Button>
            {mode === 'edit' && (
              <Button onClick={() => formRef.current?.submit()} disabled={!dirty || busy}>
                {busy ? '保存中…' : dirty ? '保存修改' : '已保存'}
              </Button>
            )}
          </div>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

/**
 * 全局任务抽屉（store 桥接）：挂载于 AppLayout。
 * 各入口（看板/导航/全景图/新建）通过 openDrawer 打开根层；
 * 详情内打开关联任务经 pushTask 压入新层 —— 每层渲染为一个独立 Dialog，
 * 逐层返回（popTask），关闭全部层后抽屉消失。
 */
export function GlobalTaskDrawer() {
  const { stack, popTask } = useTaskDrawerStore()
  return (
    <>
      {stack.map((entry, i) => {
        const isTop = i === stack.length - 1
        return (
          <TaskDetailDrawer
            key={`${i}-${entry.taskId ?? entry.task?.id ?? 'new'}`}
            open={entry.open}
            onOpenChange={(o) => {
              // 仅顶层可关闭：关闭/返回 → 弹出该层（下层原样保留）
              if (!o && isTop) popTask()
            }}
            taskId={entry.taskId}
            task={entry.task}
            mode={entry.mode}
            stacked={!isTop}
            level={i}
            onSaved={entry.onSaved}
          />
        )
      })}
    </>
  )
}

/**
 * 路由兼容页（/project/:id/tasks/:taskId）：直接访问/刷新旧链接时渲染抽屉（背景为空）。
 */
export function TaskDetailPage() {
  const { taskId } = useParams()
  const navigate = useNavigate()
  return (
    <TaskDetailDrawer
      open
      taskId={taskId}
      onOpenChange={(o) => {
        if (!o) navigate(-1)
      }}
    />
  )
}
