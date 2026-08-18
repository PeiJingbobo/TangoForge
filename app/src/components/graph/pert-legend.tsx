import type { StateMachineState } from '@/types/models'

/**
 * PERT 图例（TF-055）：状态机颜色圆点 + Label 说明。
 * 颜色契约与 StateBadge / 力导向图一致（QA 2026-08-09）：未知状态回退灰。
 */
export function PertLegend({ states }: { states: StateMachineState[] }) {
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 px-1" aria-label="状态图例">
      {states.map((s) => (
        <span key={s.Key} className="flex items-center gap-1.5 text-caption text-muted-foreground">
          <span
            className="size-2.5 rounded-full"
            style={{ backgroundColor: s.Color || '#9aa0a6' }}
          />
          {s.Label || s.Key}
        </span>
      ))}
      <span className="flex items-center gap-1.5 text-caption text-muted-foreground">
        <span className="size-2.5 rounded-full bg-[#9aa0a6]" />
        未知/已归档
      </span>
    </div>
  )
}
