import { useMemo, useRef, useState } from 'react'
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
import type { ParsedTask } from '@/types/models'
import type { StateMachineState } from '@/types/models'
import type { Task, TaskTreeNode, UpdateTaskInput } from '@/types/task'

/**
 * 草稿审阅（TF 优化）：虚拟任务体系（草稿任务树）三视图预览 + 任务详情编辑。
 * 树形/时间线/状态分类与任务导航一致；任务点击 → 抽屉编辑（保存经 PUT 落草稿）。
 * 依赖关系以草稿内临时唯一 id（ParsedTask.id）引用，与任务标题解耦——
 * LLM 解析/编辑保存均不改标题语义；旧草稿标题引用渲染层规范化为 id。
 * 顶部操作：返回/关闭/丢弃/确认导入。
 */

// 草稿任务 → 树节点（id 用草稿临时唯一编号，TreeNav 折叠/选中以 id 为键；children 由 walkAll 填充）
function toTreeNode(t: ParsedTask): TaskTreeNode {
  return {
    id: t.id,
    project_id: 1,
    parent_id: null,
    title: t.title,
    number: t.number ?? '',
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
    children: [],
  }
}

// 草稿任务 → 伪 Task（详情编辑用；id 为草稿临时唯一编号，依赖引用同空间可直接匹配）
function toTask(t: ParsedTask): Task {
  return {
    id: t.id,
    project_id: 1,
    parent_id: null,
    title: t.title,
    number: t.number ?? '',
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

// 全树依赖引用规范化：标题引用 → 临时 id（id 优先、标题兜底；无法解析保留原样）。
// 旧草稿（标题引用）经一次编辑保存即整体升级为 id 格式——此后修改任务标题不再影响依赖关系。
function normalizeDraftDeps(list: ParsedTask[]): ParsedTask[] {
  const flat: ParsedTask[] = []
  const walk = (l: ParsedTask[]) =>
    l.forEach((t) => {
      flat.push(t)
      walk(t.children ?? [])
    })
  walk(list)
  const idSet = new Set(flat.map((t) => t.id))
  const byTitle = new Map<string, string>()
  for (const t of flat) byTitle.set(t.title, t.id)
  const norm = (t: ParsedTask): ParsedTask => ({
    ...t,
    depends_on: t.depends_on.map((ref) => (idSet.has(ref) ? ref : (byTitle.get(ref) ?? ref))),
    children: (t.children ?? []).map(norm),
  })
  return list.map(norm)
}

export interface DraftReviewProps {
  draftId: string
  onExit: () => void
  /** 外部显式项目标识（workdir 或 id）；缺省读 useProjectId（项目内使用场景）。
   *  引导流程等尚无项目 store 上下文时传入。 */
  project?: string
  /** 确认导入成功回调（引导流程用于把状态同步到步骤 gate）。 */
  onConfirmed?: () => void
}

export function DraftReview({ draftId, onExit, project, onConfirmed }: DraftReviewProps) {
  const fallbackPid = useProjectId()
  const pid = project ?? fallbackPid
  const { data: detail, isLoading } = useDraftDetail(draftId, pid)
  const updateTasks = useUpdateDraftTasks(pid)
  const confirmDraft = useConfirmDraft(pid)
  const discardDraft = useDiscardDraft(pid)
  const { data: sm } = useStateMachine(pid)
  const [tasks, setTasks] = useState<ParsedTask[] | null>(null)
  const [editId, setEditId] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // 本地任务树（优先编辑态，否则加载明细）
  const states = sm?.States ?? []

  const { treeNodes, flatTasks, pathById, byId } = useMemo(() => {
    const tree = tasks ?? detail?.tasks ?? []
    const nodesOut: TaskTreeNode[] = []
    const flatsOut: Task[] = []
    const pathOut = new Map<string, string>() // 临时 id → 树路径（replaceAtPath 用）
    const idOut = new Map<string, ParsedTask>() // 临时 id → 原始任务
    const walkAll = (list: ParsedTask[], prefix: string, bucket: TaskTreeNode[]) => {
      list.forEach((t, i) => {
        const path = prefix ? `${prefix}.${i + 1}` : `${i + 1}`
        // 后端 ensureTaskIDs 保证 id 非空；兜底 path 防止旧数据异常
        const id = t.id || path
        const withId = { ...t, id }
        pathOut.set(id, path)
        idOut.set(id, withId)
        bucket.push(toTreeNode(withId))
        flatsOut.push(toTask(withId))
        walkAll(t.children ?? [], path, bucket[bucket.length - 1].children)
      })
    }
    walkAll(tree, '', nodesOut)
    // 依赖引用规范化：旧草稿标题引用 → 临时 id（id 优先，标题兜底）
    const idSet = new Set(flatsOut.map((t) => t.id))
    const byTitle = new Map(flatsOut.map((t) => [t.title, t.id]))
    for (const t of flatsOut) {
      t.depends_on = t.depends_on.map((ref) => (idSet.has(ref) ? ref : (byTitle.get(ref) ?? ref)))
    }
    return { treeNodes: nodesOut, flatTasks: flatsOut, pathById: pathOut, byId: idOut }
  }, [tasks, detail])

  const editTask = editId ? (byId.has(editId) ? toTask(byId.get(editId)!) : undefined) : undefined

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
    const path = pathById.get(latest.id)
    const original = byId.get(latest.id)
    if (!path) return
    // 伪 Task → ParsedTask（保留临时 id 与子任务树，回写树）
    const next: ParsedTask = {
      id: original?.id ?? latest.id,
      title: latest.title,
      description: latest.description,
      status: latest.status,
      priority: latest.priority,
      tags: latest.tags,
      assignee: latest.assignee,
      depends_on: latest.depends_on,
      children: original?.children,
    }
    const updated = replaceAtPath(tasks ?? detail?.tasks ?? [], path, next)
    // 全树依赖规范化（标题引用 → 临时 id）：旧草稿编辑一次即升级为 id 格式
    const normalized = normalizeDraftDeps(updated)
    setTasks(normalized)
    setBusy(true)
    updateTasks.mutate(
      { draftId, tasks: normalized },
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
        toast.success(
          r.dropped_deps > 0 ? `导入完成（忽略 ${r.dropped_deps} 个失效依赖引用）` : '导入完成',
          {
            description: `${r.source_file}：创建 ${r.created} 个，覆盖归档 ${r.archived} 个${
              r.dropped_deps > 0 ? '。失效引用（被依赖任务标题已修改等）已被忽略' : ''
            }`,
          },
        )
        onConfirmed?.()
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
          <TreeNav tree={treeNodes} onSelect={setEditId} />
        </TabsContent>
        <TabsContent value="timeline" className="pt-4">
          <TimelineView tasks={flatTasks} onOpen={setEditId} />
        </TabsContent>
        <TabsContent value="status" className="pt-4">
          <StatusView tasks={flatTasks} states={states} onOpen={setEditId} />
        </TabsContent>
      </Tabs>

      {/* 草稿任务详情编辑 */}
      <DraftTaskDrawer
        open={Boolean(editTask)}
        onOpenChange={(o) => {
          if (!o) setEditId(null)
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
