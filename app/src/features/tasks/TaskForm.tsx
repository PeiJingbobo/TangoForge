import { forwardRef, useEffect, useImperativeHandle, useMemo, useState } from 'react'
import { Check, Pencil, Plus, X } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { StateMachineState } from '@/types/models'
import type { Task, UpdateTaskInput } from '@/types/task'
import { PRIORITY_OPTIONS } from '@/features/tasks/constants'

/**
 * 任务编辑表单（TF-026 行内编辑：点击字段进入编辑）。
 * 纯内容组件：不含底部操作区（footer 由宿主（抽屉）在滚动区外渲染，
 * 通过 ref.submit() 触发提交、onDirtyChange 通知脏状态）。
 */
export interface TaskFormProps {
  task: Task
  states: StateMachineState[]
  /** 全量任务（依赖选择/子任务标题引用） */
  allTasks: Task[]
  /** 只读模式：禁用全部编辑入口（详情抽屉只读态） */
  readOnly?: boolean
  onSubmit: (body: UpdateTaskInput & { status?: string }) => void
  /** 脏状态变化通知（footer 保存按钮可用性） */
  onDirtyChange?: (dirty: boolean) => void
}

/** 宿主可通过 ref 触发提交 / 读取脏状态 */
export interface TaskFormHandle {
  submit: () => void
  dirty: boolean
}

export const TaskForm = forwardRef<TaskFormHandle, TaskFormProps>(function TaskForm(
  { task, states, allTasks, readOnly = false, onSubmit, onDirtyChange },
  ref,
) {
  const [title, setTitle] = useState(task.title)
  const [description, setDescription] = useState(task.description)
  const [status, setStatus] = useState(task.status)
  const [priority, setPriority] = useState(task.priority)
  const [assignee, setAssignee] = useState(task.assignee)
  const [tags, setTags] = useState<string[]>(task.tags)
  const [dependsOn, setDependsOn] = useState<string[]>(task.depends_on)
  const [editingTitle, setEditingTitle] = useState(false)
  const [editingDesc, setEditingDesc] = useState(false)
  const [newTag, setNewTag] = useState('')

  // 外部数据变化（保存成功/切任务）时同步表单
  useEffect(() => {
    setTitle(task.title)
    setDescription(task.description)
    setStatus(task.status)
    setPriority(task.priority)
    setAssignee(task.assignee)
    setTags(task.tags)
    setDependsOn(task.depends_on)
  }, [task])

  const tasksById = useMemo(() => new Map(allTasks.map((t) => [t.id, t])), [allTasks])
  const dependTasks = dependsOn.map((id) => tasksById.get(id)).filter((t): t is Task => Boolean(t))
  const candidateDeps = useMemo(
    () => allTasks.filter((t) => t.id !== task.id && !dependsOn.includes(t.id)),
    [allTasks, task.id, dependsOn],
  )
  const children = useMemo(
    () => allTasks.filter((t) => t.parent_id === task.id),
    [allTasks, task.id],
  )

  const addTag = () => {
    const t = newTag.trim()
    if (t && !tags.includes(t)) setTags([...tags, t])
    setNewTag('')
  }

  const addDependency = (id: string) => {
    if (id && !dependsOn.includes(id)) setDependsOn([...dependsOn, id])
  }

  const handleSubmit = () => {
    const body: UpdateTaskInput & { status?: string } = {}
    if (title.trim() && title.trim() !== task.title) body.title = title.trim()
    if (description !== task.description) body.description = description
    if (status !== task.status) body.status = status
    if (priority !== task.priority) body.priority = priority
    if (assignee !== task.assignee) body.assignee = assignee
    if (JSON.stringify(tags) !== JSON.stringify(task.tags)) body.tags = tags
    if (JSON.stringify(dependsOn) !== JSON.stringify(task.depends_on)) body.depends_on = dependsOn
    onSubmit(body)
  }

  const dirty =
    title !== task.title ||
    description !== task.description ||
    status !== task.status ||
    priority !== task.priority ||
    assignee !== task.assignee ||
    JSON.stringify(tags) !== JSON.stringify(task.tags) ||
    JSON.stringify(dependsOn) !== JSON.stringify(task.depends_on)

  // 暴露提交能力与脏状态给宿主（抽屉 footer）
  useImperativeHandle(ref, () => ({ submit: handleSubmit, dirty }))
  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])

  return (
    <div>
      {/* 标题（只读纯文本 / 点击行内编辑） */}
      {readOnly ? (
        <h1 className="text-h1 text-foreground">{title}</h1>
      ) : editingTitle ? (
        <div className="flex items-center gap-2">
          <Input
            autoFocus
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') setEditingTitle(false)
              if (e.key === 'Escape') {
                setTitle(task.title)
                setEditingTitle(false)
              }
            }}
            onBlur={() => setEditingTitle(false)}
            className="h-10 text-xl font-bold"
            aria-label="任务标题编辑"
          />
        </div>
      ) : (
        <button
          type="button"
          onClick={() => setEditingTitle(true)}
          className="group flex w-full items-start gap-2 text-left"
          aria-label="编辑标题"
        >
          <h1 className="min-w-0 flex-1 text-h1 text-foreground break-words group-hover:text-primary-600">
            {title}
          </h1>
          <span className="mt-2 hidden shrink-0 items-center gap-1 text-caption text-muted-foreground group-hover:flex">
            <Pencil className="size-3" /> 点击编辑标题
          </span>
        </button>
      )}

      {/* meta 行：状态/优先级/负责人 */}
      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Select value={status} onValueChange={setStatus} disabled={readOnly}>
          <SelectTrigger
            aria-label="状态"
            className="h-8 w-auto gap-2 rounded-full border-border bg-muted px-3 text-xs"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {states.map((s) => (
              <SelectItem key={s.Key} value={s.Key}>
                {s.Label || s.Key}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={String(priority)}
          onValueChange={(v) => setPriority(Number(v))}
          disabled={readOnly}
        >
          <SelectTrigger
            aria-label="优先级"
            className="h-8 w-auto gap-2 rounded-full border-border bg-muted px-3 text-xs"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {PRIORITY_OPTIONS.map((p) => (
              <SelectItem key={p.value} value={String(p.value)}>
                {p.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input
          value={assignee}
          onChange={(e) => setAssignee(e.target.value)}
          placeholder="负责人"
          aria-label="负责人"
          readOnly={readOnly}
          className="h-8 w-28 rounded-full border-border bg-muted px-3 text-xs"
        />
      </div>

      {/* 描述（只读纯文本 / 点击行内编辑） */}
      <div className="mt-6">
        {readOnly ? (
          description ? (
            <p className="text-body leading-relaxed whitespace-pre-wrap">{description}</p>
          ) : (
            <span className="text-body text-muted-foreground">暂无描述</span>
          )
        ) : editingDesc ? (
          <textarea
            autoFocus
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                setDescription(task.description)
                setEditingDesc(false)
              }
            }}
            onBlur={() => setEditingDesc(false)}
            rows={6}
            className="w-full rounded-[14px] border border-input bg-card px-4 py-3 text-body leading-relaxed outline-none focus:border-primary-400 focus:ring-[3px] focus:ring-primary-100"
            aria-label="任务描述编辑"
          />
        ) : (
          <button
            type="button"
            onClick={() => setEditingDesc(true)}
            className="group block w-full rounded-[14px] p-3 text-left -m-3"
            aria-label="编辑描述"
          >
            {description ? (
              <p className="text-body leading-relaxed whitespace-pre-wrap">{description}</p>
            ) : (
              <span className="text-body text-muted-foreground">添加描述…（点击编辑）</span>
            )}
          </button>
        )}
      </div>

      {/* 标签 */}
      <div className="mt-6 flex flex-wrap items-center gap-1.5">
        {tags.map((t) => (
          <Badge key={t} className={cn('max-w-full gap-1', !readOnly && 'pr-1.5')}>
            <span className="min-w-0 break-words">{t}</span>
            {!readOnly && (
              <button
                type="button"
                aria-label={`移除标签 ${t}`}
                onClick={() => setTags(tags.filter((x) => x !== t))}
                className="shrink-0 rounded-full hover:text-destructive-ink"
              >
                <X className="size-3" />
              </button>
            )}
          </Badge>
        ))}
        {!readOnly &&
          (newTag ? (
            <Input
              autoFocus
              value={newTag}
              onChange={(e) => setNewTag(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') addTag()
                if (e.key === 'Escape') setNewTag('')
              }}
              onBlur={addTag}
              className="h-7 w-24 rounded-full px-2 text-xs"
              aria-label="新标签"
            />
          ) : (
            <button
              type="button"
              aria-label="添加标签"
              onClick={() => setNewTag(' ')}
              className="flex h-7 items-center gap-1 rounded-full border border-dashed border-border px-2.5 text-xs text-muted-foreground hover:border-primary-300 hover:text-primary-600"
            >
              <Plus className="size-3" /> 标签
            </button>
          ))}
      </div>

      {/* 依赖 */}
      <div className="mt-6">
        <Label className="mb-2 text-muted-foreground">依赖（depends_on）</Label>
        {/* 第一行：依赖标签流 */}
        <div className="flex flex-wrap items-center gap-1.5">
          {dependTasks.map((t) => (
            <Badge
              key={t.id}
              variant="outline"
              className={cn('max-w-full gap-1', !readOnly && 'pr-1.5')}
            >
              <span className="min-w-0 break-words">{t.title}</span>
              {!readOnly && (
                <button
                  type="button"
                  aria-label={`移除依赖 ${t.title}`}
                  onClick={() => setDependsOn(dependsOn.filter((id) => id !== t.id))}
                  className="shrink-0 rounded-full hover:text-destructive-ink"
                >
                  <X className="size-3" />
                </button>
              )}
            </Badge>
          ))}
          {dependTasks.length === 0 && <span className="text-sm text-muted-foreground">无</span>}
        </div>
        {/* 第二行：添加依赖入口（始终位于所有标签下方） */}
        {!readOnly && (
          <div className="mt-1.5">
            <Select value="" onValueChange={addDependency}>
              <SelectTrigger
                aria-label="添加依赖"
                className="h-7 w-auto gap-1 rounded-full border border-dashed border-border px-2.5 text-xs text-muted-foreground"
              >
                <Plus className="size-3" /> 添加依赖
              </SelectTrigger>
              <SelectContent>
                {candidateDeps.length === 0 && (
                  <div className="px-2 py-1.5 text-xs text-muted-foreground">没有可依赖的任务</div>
                )}
                {candidateDeps.slice(0, 50).map((t) => (
                  <SelectItem key={t.id} value={t.id}>
                    {t.title}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
      </div>

      {/* 子任务（只读展示） */}
      {children.length > 0 && (
        <div className="mt-6 border-t border-divider pt-4">
          <Label className="mb-2 text-muted-foreground">子任务（{children.length}）</Label>
          <ul className="space-y-1.5">
            {children.map((c) => (
              <li key={c.id} className="flex items-center gap-2 text-sm">
                <Check className="size-3.5 text-success-ink" />
                <span
                  className={cn(c.status === 'archived' && 'text-muted-foreground line-through')}
                >
                  {c.title}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
})
