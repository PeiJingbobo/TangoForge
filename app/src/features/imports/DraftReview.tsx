import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { ArrowLeft, Check, Loader2, Trash2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { TreeNav } from '@/components/common/TreeNav'
import { TimelineView, StatusView } from '@/features/tasks/NavViews'
import { TaskForm, type TaskFormHandle } from '@/features/tasks/TaskForm'
import { useDraftDetail, useUpdateDraftTasks } from '@/hooks/useDraftDetail'
import { useConfirmDraft, useDiscardDraft } from '@/hooks/useImports'
import { useStateMachine } from '@/hooks/useStateMachine'
import { useProjectId } from '@/hooks/useProject'
import { useRef } from 'react'
import type { ParsedTask } from '@/types/models'
import type { StateMachineState } from '@/types/models'
import type { Task, TaskTreeNode, UpdateTaskInput } from '@/types/task'

/**
 * 草稿审阅（TF 优化）：虚拟任务体系（草稿任务树）三视图预览 + 任务详情编辑。
 * 树形/时间线/状态分类与任务导航一致；任务点击 → 抽屉编辑（保存经 PUT 落草稿）。
 * 顶部操作：返回/关闭/丢弃/确认导入。
 */

// 草稿任务 → 树节点（路径作 id："1"/"1.2"）
function toTreeNode(t: ParsedTask, path: string): TaskTreeNode {
  return {
    id: path,
    project_id: 1,
    parent_id: null,
    title: t.title,
    description: t.description,
    status: t.status,
    priority: t.priority,
    tags: t.tags,
    assignee: t.assignee,
    depends_on: t.depends_on,
    archived_from: '',
    source_file: '',
    source_section: '',
    created_at: '',
    updated_at: '',
    children: (t.children ?? []).map((c, i) => toTreeNode(c, `${path}.${i + 1}`)),
  }
}

// 草稿任务 → 伪 Task（详情编辑用；depends_on 为标题引用）
function toTask(t: ParsedTask, path: string): Task {
  return {
    id: path,
    project_id: 1,
    parent_id: null,
    title: t.title,
    description: t.description,
    status: t.status,
    priority: t.priority,
    tags: t.tags,
    assignee: t.assignee,
    depends_on: t.depends_on,
    archived_from: '',
    source_file: '',
    source_section: '',
    created_at: '',
    updated_at: '',
  }
}

// 按路径替换树节点（编辑保存回写）
function replaceAtPath(tasks: ParsedTask[], path: string, next: ParsedTask): ParsedTask[] {
  const [head, ...rest] = path.split('.')
  const idx = Number(head) - 1
  if (idx < 0 || idx >= tasks.length) return tasks
  if (rest.length === 0) {
    return tasks.map((t, i) => (i === idx ? next : t))
  }
  return tasks.map((t, i) =>
    i === idx ? { ...t, children: replaceAtPath(t.children ?? [], rest.join('.'), next) } : t,
  )
}

export interface DraftReviewProps {
  draftId: string
  onExit: () => void
}

export function DraftReview({ draftId, onExit }: DraftReviewProps) {
  const pid = useProjectId()
  const { data: detail, isLoading } = useDraftDetail(draftId, pid)
  const updateTasks = useUpdateDraftTasks(pid)
  const confirmDraft = useConfirmDraft(pid)
  const discardDraft = useDiscardDraft(pid)
  const { data: sm } = useStateMachine(pid)
  const [tasks, setTasks] = useState<ParsedTask[] | null>(null)
  const [editPath, setEditPath] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // 本地任务树（优先编辑态，否则加载明细）
  const states = sm?.States ?? []

  const treeNodes = useMemo(
    () => (tasks ?? detail?.tasks ?? []).map((t, i) => toTreeNode(t, `${i + 1}`)),
    [tasks, detail],
  )
  const flatTasks = useMemo(() => {
    const walk = (list: ParsedTask[], prefix: string): Task[] =>
      list.flatMap((t, i) => {
        const path = prefix ? `${prefix}.${i + 1}` : `${i + 1}`
        return [toTask(t, path), ...walk(t.children ?? [], path)]
      })
    return walk(tasks ?? detail?.tasks ?? [], '')
  }, [tasks, detail])

  const editTask = editPath ? flatTasks.find((t) => t.id === editPath) : undefined

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-9 w-2/3" />
        <Skeleton className="h-[420px] w-full rounded-2xl" />
      </div>
    )
  }
  if (!detail) {
    return <p className="text-sm text-muted-foreground">草稿加载失败或已被处理。</p>
  }

  const persistEdit = (latest: Task) => {
    // 伪 Task → ParsedTask（回写树）
    const next: ParsedTask = {
      title: latest.title,
      description: latest.description,
      status: latest.status,
      priority: latest.priority,
      tags: latest.tags,
      assignee: latest.assignee,
      depends_on: latest.depends_on,
    }
    const updated = replaceAtPath(tasks ?? detail?.tasks ?? [], latest.id, next)
    setTasks(updated)
    setBusy(true)
    updateTasks.mutate(
      { draftId, tasks: updated },
      {
        onSuccess: () => toast.success('草稿任务已保存'),
        onError: (e) => toast.error(e instanceof Error ? e.message : '保存失败'),
        onSettled: () => setBusy(false),
      },
    )
  }

  const handleConfirm = () => {
    setBusy(true)
    confirmDraft.mutate(draftId, {
      onSuccess: (r) => {
        toast.success('导入完成', {
          description: `${r.source_file}：创建 ${r.created} 个，覆盖归档 ${r.archived} 个`,
        })
        onExit()
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : '确认失败'),
      onSettled: () => setBusy(false),
    })
  }

  const handleDiscard = () => {
    if (!window.confirm('确认丢弃该草稿？丢弃后无法恢复。')) return
    setBusy(true)
    discardDraft.mutate(draftId, {
      onSuccess: () => {
        toast.success('草稿已丢弃')
        onExit()
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : '丢弃失败'),
      onSettled: () => setBusy(false),
    })
  }

  return (
    <div>
      {/* 审阅头部：返回 + 标题 + 统计 + 操作（关闭/丢弃/确认导入） */}
      <div className="mb-5 flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={onExit}
          aria-label="返回草稿列表"
          title="返回"
          className="grid size-9 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
        >
          <ArrowLeft className="size-4" />
        </button>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h1 className="text-h2 truncate text-foreground">草稿审阅</h1>
            <span className="shrink-0 truncate font-mono text-xs text-muted-foreground">
              {detail.source_file}
            </span>
          </div>
          <p className="mt-0.5 text-caption text-muted-foreground">
            {flatTasks.length} 个任务 · 状态机 {states.length} 态 · 草稿为虚拟任务体系，确认后入库
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button variant="ghost" onClick={onExit} disabled={busy}>
            <X className="size-4" /> 关闭
          </Button>
          <Button
            variant="outline"
            onClick={handleDiscard}
            disabled={busy}
            className="text-destructive-ink"
          >
            {busy ? <Loader2 className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
            丢弃
          </Button>
          <Button onClick={handleConfirm} disabled={busy}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : <Check className="size-4" />}
            确认导入
          </Button>
        </div>
      </div>

      {/* 三视图预览（与任务导航一致） */}
      <Tabs defaultValue="tree">
        <TabsList>
          <TabsTrigger value="tree">树形</TabsTrigger>
          <TabsTrigger value="timeline">时间线</TabsTrigger>
          <TabsTrigger value="status">状态分类</TabsTrigger>
        </TabsList>
        <TabsContent value="tree" className="pt-4">
          <TreeNav tree={treeNodes} onSelect={setEditPath} />
        </TabsContent>
        <TabsContent value="timeline" className="pt-4">
          <TimelineView tasks={flatTasks} onOpen={setEditPath} />
        </TabsContent>
        <TabsContent value="status" className="pt-4">
          <StatusView tasks={flatTasks} states={states} onOpen={setEditPath} />
        </TabsContent>
      </Tabs>

      {/* 草稿任务详情编辑 */}
      <DraftTaskDrawer
        open={Boolean(editTask)}
        onOpenChange={(o) => {
          if (!o) setEditPath(null)
        }}
        task={editTask}
        states={states}
        allTasks={flatTasks}
        onSaved={persistEdit}
      />
    </div>
  )
}

/** 草稿任务编辑抽屉（仿任务详情：只读态不提供；仅编辑，保存经 onSaved 回调） */
function DraftTaskDrawer({
  open,
  onOpenChange,
  task,
  states,
  allTasks,
  onSaved,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  task?: Task
  states: StateMachineState[]
  allTasks: Task[]
  onSaved: (task: Task) => void
}) {
  const formRef = useRef<TaskFormHandle>(null)
  const [dirty, setDirty] = useState(false)

  const handleSubmit = (body: UpdateTaskInput & { status?: string }) => {
    if (!task) return
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
    onSaved(latest)
    onOpenChange(false)
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="flex w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-xl">
        <SheetHeader className="border-b border-divider px-6 py-4">
          <SheetTitle className="text-base">草稿任务</SheetTitle>
          <SheetDescription>编辑将保存到草稿（确认导入时生效）。</SheetDescription>
        </SheetHeader>
        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
          {task ? (
            <TaskForm
              ref={formRef}
              task={task}
              states={states}
              allTasks={allTasks}
              onSubmit={handleSubmit}
              onDirtyChange={setDirty}
            />
          ) : (
            <Skeleton className="h-40 w-full" />
          )}
        </div>
        <SheetFooter className="border-t border-divider bg-muted/60 px-6 py-3 sm:flex-row sm:items-center sm:justify-end">
          <div className="flex items-center gap-2">
            <Button variant="ghost" onClick={() => onOpenChange(false)}>
              关闭
            </Button>
            <Button onClick={() => formRef.current?.submit()} disabled={!dirty}>
              {dirty ? '保存草稿任务' : '已保存'}
            </Button>
          </div>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
