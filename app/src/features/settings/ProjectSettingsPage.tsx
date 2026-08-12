import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import {
  DndContext,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { GripVertical, Loader2, Plus, RotateCcw, Save, Trash2, Undo2 } from 'lucide-react'
import { parse as yamlParse, stringify as yamlStringify } from 'yaml'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  useProjectConfig,
  useUpdateProjectConfig,
  isProjectConfigRejected,
} from '@/hooks/useProjectConfig'
import type {
  ProjectConfigDTO,
  StateMachineDTO,
  StateMachineState,
  StateMachineTransition,
} from '@/types/models'
import { reorderStateRows } from '@/features/settings/state-machine-utils'
import { cn } from '@/lib/utils'

/**
 * 项目设置页（TF-032，/project/:projectId/settings）——项目 config.yaml 可视化编辑。
 *
 * 交互模型（QA 已确认 + 2026-08-07 优化）：
 * - 结构化表单（状态机 + 导出模板）为主，另设「高级」YAML 原文区兜底（未来字段扩展）；
 * - 显式保存（sticky 底部操作栏）：「保存修改」提交，「放弃修改」恢复服务端值；
 * - **状态机流转规则为派生数据**：每个状态行下方直接点亮「流转到」目标标签，
 *   Transitions 由 states + targets 隐式生成（每状态一条 from 规则）——无独立流转规则编辑区；
 *   YAML 中手写的 transitions 保存时重新生成覆盖（规范化：去重、对齐 states）；
 * - 状态列表支持拖拽排序（dnd-kit sortable，targets 跟随索引）；
 * - 表单与 YAML 区是同一 draft 的两种视图：表单编辑同步刷新 YAML（未手动编辑时）；
 *   手动编辑 YAML 后以 YAML 为准（yamlDirty），可「从表单生成」覆盖；
 * - 后端校验失败（编辑校验 400 / STATUS_IN_USE 422）toast 提示并保留修改供重试。
 */

/** 与后端 config.DefaultStateMachine 保持一致（docs/TASK-SEMANTICS.md §5.1） */
const DEFAULT_STATE_MACHINE: StateMachineDTO = {
  States: [
    { Key: 'todo', Label: '待办', Color: '#9aa0a6' },
    { Key: 'doing', Label: '进行中', Color: '#1a73e8' },
    { Key: 'done', Label: '已完成', Color: '#34a853' },
  ],
  Transitions: [
    { From: 'todo', To: ['doing', 'done'] },
    { From: 'doing', To: ['todo', 'done'] },
    { From: 'done', To: ['doing', 'todo'] },
  ],
}

const newRowId = () => crypto.randomUUID()

/* ---------- 派生 Transitions 工具（流转规则由 states+targets 隐式生成） ---------- */

/** Transitions → 每个状态的 to 集合（按 states 顺序；多 from 规则取首条，to 去重） */
function transitionsToTargets(sm: StateMachineDTO): string[][] {
  const byKey = new Map<string, string[]>()
  for (const t of sm.Transitions) {
    if (!byKey.has(t.From)) byKey.set(t.From, [...new Set(t.To)])
  }
  return sm.States.map((s) => byKey.get(s.Key) ?? [])
}

/** states + targets → Transitions（每状态一条 from 规则；to 去重、过滤空项） */
function generateTransitions(
  states: StateMachineState[],
  targets: string[][],
): StateMachineTransition[] {
  return states.map((s, i) => ({
    From: s.Key,
    To: [...new Set((targets[i] ?? []).filter((k) => k !== ''))],
  }))
}

/** 规范化 DTO：Transitions 一律由 states+targets 重新生成（覆盖 YAML 手写规则） */
function normalizeDto(dto: ProjectConfigDTO, targets: string[][]): ProjectConfigDTO {
  return {
    StateMachine: {
      States: dto.StateMachine.States,
      Transitions: generateTransitions(dto.StateMachine.States, targets),
    },
    Export: dto.Export,
  }
}

/** DTO → config.yaml 磁盘格式（snake_case 键）的 YAML 文本 */
function toYamlText(dto: ProjectConfigDTO): string {
  return yamlStringify({
    state_machine: {
      states: dto.StateMachine.States.map((s) => ({
        key: s.Key,
        label: s.Label,
        color: s.Color,
      })),
      transitions: dto.StateMachine.Transitions.map((t) => ({ from: t.From, to: t.To })),
    },
    export: dto.Export.TemplatePath ? { template_path: dto.Export.TemplatePath } : {},
  })
}

/**
 * YAML 文本 → DTO + targets（兼容 snake_case 磁盘格式与 PascalCase DTO 键）。
 * transitions 反推为 targets 作为初始值；保存时仍会重新生成覆盖（规范化）。
 * 解析失败 / 无 states → 返回 null。
 */
function fromYamlText(text: string): { dto: ProjectConfigDTO; targets: string[][] } | null {
  let parsed: unknown
  try {
    parsed = yamlParse(text)
  } catch {
    return null
  }
  if (typeof parsed !== 'object' || parsed === null) return null
  const root = parsed as Record<string, unknown>
  const sm = (root.state_machine ?? root.StateMachine) as Record<string, unknown> | undefined
  const ex = (root.export ?? root.Export) as Record<string, unknown> | undefined
  const statesRaw = sm?.states ?? sm?.States
  if (!Array.isArray(statesRaw)) return null
  const states = statesRaw.map((s) => {
    const st = (s ?? {}) as Record<string, unknown>
    return {
      Key: String(st.key ?? st.Key ?? '').trim(),
      Label: String(st.label ?? st.Label ?? ''),
      Color: String(st.color ?? st.Color ?? '#9aa0a6'),
    }
  })
  const transitionsRaw = sm?.transitions ?? sm?.Transitions
  const transitions = (Array.isArray(transitionsRaw) ? transitionsRaw : []).map((t) => {
    const tr = (t ?? {}) as Record<string, unknown>
    const toRaw = Array.isArray(tr.to) ? tr.to : Array.isArray(tr.To) ? tr.To : []
    return {
      From: String(tr.from ?? tr.From ?? '').trim(),
      To: toRaw.map((x) => String(x ?? '')),
    }
  })
  // 反推 targets：按 YAML states 顺序对齐（引用不存在的 key 忽略）。
  const byKey = new Map<string, string[]>()
  for (const t of transitions) {
    if (!byKey.has(t.From)) byKey.set(t.From, [...new Set(t.To)])
  }
  const targets = states.map((s) => byKey.get(s.Key) ?? [])
  return {
    dto: {
      StateMachine: { States: states, Transitions: transitions },
      Export: { TemplatePath: String(ex?.template_path ?? ex?.TemplatePath ?? '') },
    },
    targets,
  }
}

/* ---------- 状态行（可排序 + 行内目标标签） ---------- */

interface StateRowProps {
  rowId: string
  index: number
  state: StateMachineState
  targets: string[]
  allStates: StateMachineState[]
  onPatchField: (i: number, field: 'Key' | 'Label' | 'Color', value: string) => void
  onRemove: (i: number) => void
  onToggleTarget: (i: number, key: string) => void
}

function StateRow({
  rowId,
  index,
  state,
  targets,
  allStates,
  onPatchField,
  onRemove,
  onToggleTarget,
}: StateRowProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: rowId,
  })
  const style = { transform: CSS.Transform.toString(transform), transition }

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(
        'rounded-xl border border-border bg-background p-2 transition-shadow',
        isDragging && 'z-10 shadow-lg',
      )}
    >
      {/* 第一行：拖拽手柄 + 颜色 + key + 名称 + 删除 */}
      <div className="flex items-center gap-2">
        <button
          type="button"
          className="size-6 shrink-0 cursor-grab touch-none text-muted-foreground hover:text-foreground"
          aria-label={`拖动排序状态 ${state.Label || state.Key || index + 1}`}
          {...attributes}
          {...listeners}
        >
          <GripVertical className="size-4" />
        </button>
        <input
          type="color"
          value={/^#[0-9a-fA-F]{6}$/.test(state.Color) ? state.Color : '#2292D8'}
          onChange={(e) => onPatchField(index, 'Color', e.target.value)}
          className="size-8 shrink-0 cursor-pointer rounded-lg border-0 bg-transparent p-0"
          aria-label={`状态 ${index + 1} 颜色`}
        />
        <Input
          value={state.Key}
          onChange={(e) => onPatchField(index, 'Key', e.target.value)}
          placeholder="key（如 review）"
          className="h-8 w-32 shrink-0 font-mono text-sm"
          aria-label={`状态 ${index + 1} key`}
        />
        <Input
          value={state.Label}
          onChange={(e) => onPatchField(index, 'Label', e.target.value)}
          placeholder="显示名称（如 评审中）"
          className="h-8 flex-1 text-sm"
          aria-label={`状态 ${index + 1} 名称`}
        />
        <Button
          variant="ghost"
          size="icon"
          className="size-8 shrink-0 text-muted-foreground hover:text-destructive-ink"
          onClick={() => onRemove(index)}
          aria-label={`删除状态 ${state.Key || index + 1}`}
        >
          <Trash2 className="size-4" />
        </Button>
      </div>
      {/* 第二行：流转目标（点亮式多选，隐式生成该状态的 from 流转规则） */}
      <div className="mt-1.5 flex flex-wrap items-center gap-1.5 pl-8">
        <span className="text-xs text-muted-foreground">流转到</span>
        {allStates.map((s) => {
          const active = targets.includes(s.Key)
          return (
            <Badge
              key={s.Key}
              role="button"
              variant={active ? 'default' : 'outline'}
              className="cursor-pointer px-2.5 py-1"
              onClick={() => onToggleTarget(index, s.Key)}
              aria-label={`状态 ${state.Label || state.Key || index + 1} 流转目标 ${s.Label || s.Key}`}
            >
              {s.Label || s.Key}
            </Badge>
          )
        })}
      </div>
    </div>
  )
}

/* ---------- 页面 ---------- */

export function ProjectSettingsPage() {
  const { data, isLoading, isError, error, refetch, isFetching } = useProjectConfig()
  const update = useUpdateProjectConfig()
  const [draft, setDraft] = useState<ProjectConfigDTO | null>(null)
  /** 每状态流转目标集合（与 states 索引对齐；transitions 由其派生） */
  const [targets, setTargets] = useState<string[][]>([])
  /** 状态行稳定 id（拖拽排序用，与 key 解耦） */
  const [rowIds, setRowIds] = useState<string[]>([])
  const [yamlText, setYamlText] = useState('')
  const [yamlDirty, setYamlDirty] = useState(false)

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))

  // 数据首次加载后建立编辑副本（draft / targets / rowIds / yaml 同源）。
  useEffect(() => {
    if (data && !draft) {
      setDraft(data)
      setTargets(transitionsToTargets(data.StateMachine))
      setRowIds(data.StateMachine.States.map(() => newRowId()))
      setYamlText(toYamlText(normalizeDto(data, transitionsToTargets(data.StateMachine))))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  const dirty = useMemo(() => {
    if (draft === null || data === undefined) return false
    return (
      JSON.stringify(normalizeDto(draft, targets)) !==
      JSON.stringify(normalizeDto(data, transitionsToTargets(data.StateMachine)))
    )
  }, [draft, targets, data])

  if (isLoading) {
    return (
      <div className="w-full">
        <h1 className="text-h2 text-foreground">设置</h1>
        <Skeleton className="mt-6 h-64 w-full" />
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="w-full">
        <h1 className="text-h2 text-foreground">设置</h1>
        <div className="mt-6 rounded-2xl border border-destructive-200 bg-destructive-soft p-5">
          <div className="text-sm font-semibold text-destructive-ink">项目配置加载失败</div>
          <p className="mt-1 text-sm text-muted-foreground">
            {error instanceof Error ? error.message : '无法连接守护进程，请确认服务已启动。'}
          </p>
          <Button className="mt-3" onClick={() => void refetch()} disabled={isFetching}>
            {isFetching && <Loader2 className="size-4 animate-spin" />}
            重试
          </Button>
        </div>
      </div>
    )
  }

  const current = draft ?? data
  const states = current.StateMachine.States

  /** 统一提交入口：更新 draft + targets（+ rowIds），未手动编辑 YAML 时同步刷新 */
  const commit = (next: ProjectConfigDTO, nextTargets: string[][], nextRowIds?: string[]) => {
    setDraft(next)
    setTargets(nextTargets)
    if (nextRowIds) setRowIds(nextRowIds)
    if (!yamlDirty) setYamlText(toYamlText(normalizeDto(next, nextTargets)))
  }

  /** 保存：YAML 区手动编辑过则以 YAML 为准；Transitions 一律重新生成（覆盖手写规则） */
  const handleSave = () => {
    if (!draft) return
    let payload: ProjectConfigDTO
    let payloadTargets: string[][]
    if (yamlDirty) {
      const parsed = fromYamlText(yamlText)
      if (!parsed) {
        toast.error('YAML 解析失败', { description: '请检查格式（需含 state_machine.states）' })
        return
      }
      payload = parsed.dto
      payloadTargets = parsed.targets
    } else {
      payload = draft
      payloadTargets = targets
    }
    payload = normalizeDto(payload, payloadTargets) // 覆盖 transitions：由 states+targets 生成
    update.mutate(payload, {
      onSuccess: (saved) => {
        setDraft(saved)
        const savedTargets = transitionsToTargets(saved.StateMachine)
        setTargets(savedTargets)
        setYamlText(toYamlText(saved))
        setYamlDirty(false)
        toast.success('项目设置已保存')
      },
      onError: (err) => {
        if (isProjectConfigRejected(err)) {
          toast.error(err.message, { description: '已保留你的修改，可调整后重试' })
        } else {
          toast.error(err instanceof Error ? err.message : '保存失败')
        }
      },
    })
  }

  const handleReset = () => {
    const resetTargets = transitionsToTargets(data.StateMachine)
    setDraft(data)
    setTargets(resetTargets)
    setYamlText(toYamlText(data))
    setYamlDirty(false)
    toast.info('已放弃修改，恢复为已保存的配置')
  }

  /* ---------- 状态机表单操作 ---------- */

  const patchState = (i: number, field: 'Key' | 'Label' | 'Color', value: string) => {
    commit(
      {
        ...current,
        StateMachine: {
          ...current.StateMachine,
          States: states.map((s, idx) => (idx === i ? { ...s, [field]: value } : s)),
        },
      },
      targets,
    )
  }

  const removeState = (i: number) => {
    const key = states[i].Key
    commit(
      {
        ...current,
        StateMachine: {
          States: states.filter((_, idx) => idx !== i),
          Transitions: [],
        },
      },
      targets.filter((_, idx) => idx !== i).map((t) => t.filter((k) => k !== key)),
      rowIds.filter((_, idx) => idx !== i),
    )
  }

  const addState = () => {
    const n = states.length + 1
    commit(
      {
        ...current,
        StateMachine: {
          ...current.StateMachine,
          States: [...states, { Key: `status${n}`, Label: '', Color: '#2292D8' }],
        },
      },
      [...targets, []],
      [...rowIds, newRowId()],
    )
  }

  const toggleTarget = (i: number, key: string) => {
    const next = targets.map((t, idx) =>
      idx === i ? (t.includes(key) ? t.filter((k) => k !== key) : [...t, key]) : t,
    )
    commit(
      {
        ...current,
        StateMachine: { ...current.StateMachine, States: states, Transitions: [] },
      },
      next,
    )
  }

  const resetStateMachine = () => {
    commit(
      { ...current, StateMachine: DEFAULT_STATE_MACHINE },
      transitionsToTargets(DEFAULT_STATE_MACHINE),
      DEFAULT_STATE_MACHINE.States.map(() => newRowId()),
    )
  }

  /* ---------- 拖拽排序（states 与 targets 同步移动，索引保持对齐） ---------- */

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const from = rowIds.indexOf(String(active.id))
    const to = rowIds.indexOf(String(over.id))
    if (from === -1 || to === -1) return
    const next = reorderStateRows(states, targets, rowIds, from, to)
    commit(
      {
        ...current,
        StateMachine: { ...current.StateMachine, States: next.states, Transitions: [] },
      },
      next.targets,
      next.rowIds,
    )
  }

  return (
    // 标题固定顶部、操作栏固定底部，仅表单主体内部滚动（TF-042）；宽度与其他页面统一
    <div className="flex h-full w-full min-h-0 flex-col">
      <div className="shrink-0">
        <h1 className="text-h2 text-foreground">设置</h1>
        <p className="mt-1 text-caption text-muted-foreground">
          编辑项目 config.yaml（状态机 / 导出模板）。保存时校验配置与状态占用，失败不落盘。
        </p>
      </div>

      {/* 表单主体（仅此处内部滚动） */}
      <div className="min-h-0 flex-1 overflow-y-auto pr-1">
        {/* ---------- 状态机 ---------- */}
        <section className="mt-6 rounded-2xl border border-border bg-surface p-5">
          <div className="flex items-baseline justify-between gap-2">
            <div>
              <h2 className="text-sm font-semibold text-foreground">状态机</h2>
              <p className="mt-0.5 text-caption text-muted-foreground">
                有任务占用的状态不可删除/重命名（保存时后端校验）；拖拽手柄可排序。
              </p>
            </div>
            <Button
              variant="ghost"
              size="sm"
              className="shrink-0 text-muted-foreground"
              onClick={resetStateMachine}
            >
              <RotateCcw className="size-3.5" />
              恢复默认
            </Button>
          </div>

          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            onDragEnd={handleDragEnd}
          >
            <SortableContext items={rowIds} strategy={verticalListSortingStrategy}>
              <div className="mt-4 space-y-2">
                {states.map((s, i) => (
                  <StateRow
                    key={rowIds[i] ?? i}
                    rowId={rowIds[i] ?? String(i)}
                    index={i}
                    state={s}
                    targets={targets[i] ?? []}
                    allStates={states}
                    onPatchField={patchState}
                    onRemove={removeState}
                    onToggleTarget={toggleTarget}
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
          <Button variant="outline" size="sm" className="mt-2" onClick={addState}>
            <Plus className="size-3.5" />
            添加状态
          </Button>
        </section>

        {/* ---------- 导出 ---------- */}
        <section className="mt-4 rounded-2xl border border-border bg-surface p-5">
          <h2 className="text-sm font-semibold text-foreground">导出配置</h2>
          <p className="mt-0.5 text-caption text-muted-foreground">
            自定义 Markdown 导出模板路径（相对项目目录）；留空 = 使用内置默认模板。
          </p>
          <div className="mt-3 flex items-center gap-2">
            <Input
              value={current.Export.TemplatePath}
              onChange={(e) =>
                commit({ ...current, Export: { TemplatePath: e.target.value } }, targets)
              }
              placeholder="如 .taskboard/generated-template.tmpl"
              className="flex-1 font-mono text-sm"
              aria-label="导出模板路径"
            />
            <Button
              variant="ghost"
              size="sm"
              className="shrink-0 text-muted-foreground"
              onClick={() => commit({ ...current, Export: { TemplatePath: '' } }, targets)}
            >
              <RotateCcw className="size-3.5" />
              恢复默认
            </Button>
          </div>
        </section>

        {/* ---------- 高级（YAML 原文） ---------- */}
        <section className="mt-4 rounded-2xl border border-border bg-surface p-5">
          <div className="flex items-baseline justify-between gap-2">
            <div>
              <h2 className="text-sm font-semibold text-foreground">高级：config.yaml 原文</h2>
              <p className="mt-0.5 text-caption text-muted-foreground">
                直接编辑 YAML（与磁盘格式一致）。手动修改后保存以本区为准；未来新增字段可在此兜底。
              </p>
            </div>
            <Button
              variant="ghost"
              size="sm"
              className="shrink-0 text-muted-foreground"
              onClick={() => {
                setYamlText(toYamlText(normalizeDto(draft ?? data, targets)))
                setYamlDirty(false)
              }}
            >
              <Undo2 className="size-3.5" />
              从表单生成
            </Button>
          </div>
          <textarea
            value={yamlText}
            onChange={(e) => {
              setYamlText(e.target.value)
              setYamlDirty(true)
            }}
            spellCheck={false}
            className="mt-3 h-56 w-full resize-y rounded-xl border border-border bg-background p-3 font-mono text-xs leading-relaxed text-foreground outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40"
            aria-label="config.yaml 原文"
          />
          {yamlDirty && (
            <p className="mt-1.5 text-caption text-warning-ink">
              YAML 区已手动修改，保存时将以其内容为准（流转规则将按状态行的「流转到」重新生成）。
            </p>
          )}
        </section>
      </div>

      {/* ---------- 底部操作栏（固定，不随主体滚动） ---------- */}
      <div
        className={cn(
          'mt-4 flex shrink-0 items-center justify-end gap-2 rounded-2xl border bg-background/85 px-3 py-2.5 backdrop-blur-md',
          dirty || yamlDirty ? 'border-primary-300' : 'border-border',
        )}
      >
        <span className="mr-auto text-caption text-muted-foreground">
          {dirty || yamlDirty ? '有未保存的修改' : '已是最新'}
        </span>
        <Button variant="outline" size="sm" onClick={handleReset} disabled={!dirty && !yamlDirty}>
          <Undo2 className="size-3.5" />
          放弃修改
        </Button>
        <Button
          size="sm"
          onClick={handleSave}
          disabled={update.isPending || (!dirty && !yamlDirty)}
        >
          {update.isPending ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : (
            <Save className="size-3.5" />
          )}
          保存修改
        </Button>
      </div>
    </div>
  )
}
