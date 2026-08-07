import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { Loader2, Plus, RotateCcw, Save, Trash2, Undo2 } from 'lucide-react'
import { parse as yamlParse, stringify as yamlStringify } from 'yaml'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  useProjectConfig,
  useUpdateProjectConfig,
  isProjectConfigRejected,
} from '@/hooks/useProjectConfig'
import type { ProjectConfigDTO, StateMachineDTO } from '@/types/models'
import { cn } from '@/lib/utils'

/**
 * 项目设置页（TF-032，/project/:projectId/settings）——项目 config.yaml 可视化编辑。
 *
 * 交互模型（QA 已确认）：
 * - 结构化表单（状态机 + 导出模板）为主，另设「高级」YAML 原文区兜底（未来字段扩展）；
 * - 显式保存（sticky 底部操作栏）：「保存修改」提交，「放弃修改」恢复服务端值；
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

/** YAML 文本 → DTO（兼容 snake_case 磁盘格式与 PascalCase DTO 键）；解析失败返回 null */
function fromYamlText(text: string): ProjectConfigDTO | null {
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
  return {
    StateMachine: { States: states, Transitions: transitions },
    Export: { TemplatePath: String(ex?.template_path ?? ex?.TemplatePath ?? '') },
  }
}

export function ProjectSettingsPage() {
  const { data, isLoading, isError, error, refetch, isFetching } = useProjectConfig()
  const update = useUpdateProjectConfig()
  const [draft, setDraft] = useState<ProjectConfigDTO | null>(null)
  const [yamlText, setYamlText] = useState('')
  const [yamlDirty, setYamlDirty] = useState(false)

  // 数据首次加载后建立编辑副本（draft / yaml 同源）。
  useEffect(() => {
    if (data && !draft) {
      setDraft(data)
      setYamlText(toYamlText(data))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  const dirty = useMemo(
    () => draft !== null && data !== undefined && JSON.stringify(draft) !== JSON.stringify(data),
    [draft, data],
  )

  if (isLoading) {
    return (
      <div className="mx-auto max-w-2xl">
        <p className="text-caption uppercase tracking-[0.09em] text-muted-foreground">项目设置</p>
        <h1 className="text-h2 text-foreground">设置</h1>
        <Skeleton className="mt-6 h-64 w-full" />
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="mx-auto max-w-2xl">
        <p className="text-caption uppercase tracking-[0.09em] text-muted-foreground">项目设置</p>
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

  /** 表单变更：更新 draft，未手动编辑 YAML 时同步刷新 YAML 区 */
  const patchForm = (next: ProjectConfigDTO) => {
    setDraft(next)
    if (!yamlDirty) setYamlText(toYamlText(next))
  }

  /** 保存：YAML 区手动编辑过则以 YAML 为准（先解析校验） */
  const handleSave = () => {
    if (!draft) return
    let payload: ProjectConfigDTO
    if (yamlDirty) {
      const parsed = fromYamlText(yamlText)
      if (!parsed) {
        toast.error('YAML 解析失败', { description: '请检查格式（需含 state_machine.states）' })
        return
      }
      payload = parsed
    } else {
      payload = draft
    }
    update.mutate(payload, {
      onSuccess: (saved) => {
        setDraft(saved)
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
    setDraft(data)
    setYamlText(toYamlText(data))
    setYamlDirty(false)
    toast.info('已放弃修改，恢复为已保存的配置')
  }

  const states = current.StateMachine.States
  const transitions = current.StateMachine.Transitions

  return (
    <div className="mx-auto max-w-2xl pb-24">
      <p className="text-caption uppercase tracking-[0.09em] text-muted-foreground">项目设置</p>
      <h1 className="text-h2 text-foreground">设置</h1>
      <p className="mt-1 text-caption text-muted-foreground">
        编辑项目 config.yaml（状态机 / 导出模板）。保存时校验配置与状态占用，失败不落盘。
      </p>

      {/* ---------- 状态机 ---------- */}
      <section className="mt-6 rounded-2xl border border-border bg-surface p-5">
        <div className="flex items-baseline justify-between gap-2">
          <div>
            <h2 className="text-sm font-semibold text-foreground">状态机</h2>
            <p className="mt-0.5 text-caption text-muted-foreground">
              有任务占用的状态不可删除/重命名（保存时后端校验）。
            </p>
          </div>
          <Button
            variant="ghost"
            size="sm"
            className="shrink-0 text-muted-foreground"
            onClick={() => patchForm({ ...current, StateMachine: DEFAULT_STATE_MACHINE })}
          >
            <RotateCcw className="size-3.5" />
            恢复默认
          </Button>
        </div>

        {/* states */}
        <div className="mt-4 space-y-2">
          {states.map((s, i) => (
            <div
              key={i}
              className="flex items-center gap-2 rounded-xl border border-border bg-background p-2"
            >
              <input
                type="color"
                value={/^#[0-9a-fA-F]{6}$/.test(s.Color) ? s.Color : '#2292D8'}
                onChange={(e) => patchState(i, 'Color', e.target.value)}
                className="size-8 shrink-0 cursor-pointer rounded-lg border-0 bg-transparent p-0"
                aria-label={`状态 ${i + 1} 颜色`}
              />
              <Input
                value={s.Key}
                onChange={(e) => patchState(i, 'Key', e.target.value)}
                placeholder="key（如 review）"
                className="h-8 w-32 shrink-0 font-mono text-sm"
                aria-label={`状态 ${i + 1} key`}
              />
              <Input
                value={s.Label}
                onChange={(e) => patchState(i, 'Label', e.target.value)}
                placeholder="显示名称（如 评审中）"
                className="h-8 flex-1 text-sm"
                aria-label={`状态 ${i + 1} 名称`}
              />
              <Button
                variant="ghost"
                size="icon"
                className="size-8 shrink-0 text-muted-foreground hover:text-destructive-ink"
                onClick={() => removeState(i)}
                aria-label={`删除状态 ${s.Key || i + 1}`}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}
        </div>
        <Button variant="outline" size="sm" className="mt-2" onClick={addState}>
          <Plus className="size-3.5" />
          添加状态
        </Button>

        {/* transitions */}
        <div className="mt-6">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            流转规则
          </h3>
          <div className="mt-2 space-y-2">
            {transitions.map((t, i) => (
              <div
                key={i}
                className="flex items-center gap-2 rounded-xl border border-border bg-background p-2"
              >
                <Select value={t.From} onValueChange={(v) => patchTransition(i, 'From', v)}>
                  <SelectTrigger
                    className="h-8 w-36 shrink-0 text-sm"
                    aria-label={`流转 ${i + 1} 起始状态`}
                  >
                    <SelectValue placeholder="起始状态" />
                  </SelectTrigger>
                  <SelectContent>
                    {states.map((s) => (
                      <SelectItem key={s.Key} value={s.Key}>
                        {s.Label || s.Key}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <div className="flex flex-1 flex-wrap items-center gap-1.5">
                  {states.map((s) => {
                    const active = t.To.includes(s.Key)
                    return (
                      <Badge
                        key={s.Key}
                        role="button"
                        variant={active ? 'default' : 'outline'}
                        className="cursor-pointer px-2.5 py-1"
                        onClick={() => toggleTransitionTo(i, s.Key)}
                        aria-label={`流转 ${i + 1} 目标 ${s.Label || s.Key}`}
                      >
                        {s.Label || s.Key}
                      </Badge>
                    )
                  })}
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-8 shrink-0 text-muted-foreground hover:text-destructive-ink"
                  onClick={() => removeTransition(i)}
                  aria-label={`删除流转 ${i + 1}`}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
            ))}
          </div>
          <Button variant="outline" size="sm" className="mt-2" onClick={addTransition}>
            <Plus className="size-3.5" />
            添加流转
          </Button>
        </div>
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
            onChange={(e) => patchForm({ ...current, Export: { TemplatePath: e.target.value } })}
            placeholder="如 .taskboard/generated-template.tmpl"
            className="flex-1 font-mono text-sm"
            aria-label="导出模板路径"
          />
          <Button
            variant="ghost"
            size="sm"
            className="shrink-0 text-muted-foreground"
            onClick={() => patchForm({ ...current, Export: { TemplatePath: '' } })}
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
              setYamlText(toYamlText(draft ?? data))
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
            YAML 区已手动修改，保存时将以其内容为准。
          </p>
        )}
      </section>

      {/* ---------- sticky 操作栏 ---------- */}
      <div
        className={cn(
          'sticky bottom-0 z-30 -mx-1 mt-6 flex items-center justify-end gap-2 rounded-2xl border bg-background/85 px-3 py-2.5 backdrop-blur-md',
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

  // ---- 状态机表单操作（借助闭包访问 current） ----
  function patchState(i: number, field: 'Key' | 'Label' | 'Color', value: string) {
    const next: ProjectConfigDTO = {
      ...current,
      StateMachine: {
        ...current.StateMachine,
        States: current.StateMachine.States.map((s, idx) =>
          idx === i ? { ...s, [field]: value } : s,
        ),
      },
    }
    patchForm(next)
  }

  function removeState(i: number) {
    const key = current.StateMachine.States[i].Key
    const next: ProjectConfigDTO = {
      ...current,
      StateMachine: {
        States: current.StateMachine.States.filter((_, idx) => idx !== i),
        Transitions: current.StateMachine.Transitions.map((t) => ({
          ...t,
          From: t.From === key ? '' : t.From,
          To: t.To.filter((x) => x !== key),
        })).filter((t) => t.From !== ''),
      },
    }
    patchForm(next)
  }

  function addState() {
    const n = current.StateMachine.States.length + 1
    patchForm({
      ...current,
      StateMachine: {
        ...current.StateMachine,
        States: [
          ...current.StateMachine.States,
          { Key: `status${n}`, Label: '', Color: '#2292D8' },
        ],
      },
    })
  }

  function patchTransition(i: number, field: 'From', value: string) {
    const next: ProjectConfigDTO = {
      ...current,
      StateMachine: {
        ...current.StateMachine,
        Transitions: current.StateMachine.Transitions.map((t, idx) =>
          idx === i ? { ...t, [field]: value } : t,
        ),
      },
    }
    patchForm(next)
  }

  function toggleTransitionTo(i: number, key: string) {
    const next: ProjectConfigDTO = {
      ...current,
      StateMachine: {
        ...current.StateMachine,
        Transitions: current.StateMachine.Transitions.map((t, idx) =>
          idx === i
            ? { ...t, To: t.To.includes(key) ? t.To.filter((x) => x !== key) : [...t.To, key] }
            : t,
        ),
      },
    }
    patchForm(next)
  }

  function removeTransition(i: number) {
    patchForm({
      ...current,
      StateMachine: {
        ...current.StateMachine,
        Transitions: current.StateMachine.Transitions.filter((_, idx) => idx !== i),
      },
    })
  }

  function addTransition() {
    const from = current.StateMachine.States[0]?.Key ?? ''
    patchForm({
      ...current,
      StateMachine: {
        ...current.StateMachine,
        Transitions: [...current.StateMachine.Transitions, { From: from, To: [] }],
      },
    })
  }
}
