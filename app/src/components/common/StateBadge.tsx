import type { StateMachineState } from '@/types/models'

/**
 * 状态徽标（QA 2026-08-09）：按状态机 Color 渲染彩色圆点 + Label。
 * 用于任务导航各视图（树形/时间线/状态分类）展示任务当前状态；
 * 状态机未配置该状态时回退灰色 + 原始 Key。
 */
export function StateBadge({
  status,
  states,
  showLabel = true,
}: {
  status: string
  states: StateMachineState[]
  showLabel?: boolean
}) {
  const st = states.find((s) => s.Key === status)
  const color = st?.Color || '#9aa0a6'
  const label = st?.Label || status
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5 text-caption">
      <span
        className="size-2 shrink-0 rounded-full"
        style={{ backgroundColor: color }}
        title={label}
        aria-label={`状态 ${label}`}
      />
      {showLabel && <span className="truncate text-muted-foreground">{label}</span>}
    </span>
  )
}
