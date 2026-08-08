import { cn } from '@/lib/utils'

/**
 * 任务编号徽标（TF-040）：展示简短编号（T01/P0…）。
 * 有编号 → 等宽小徽标；无编号 → 不渲染。
 */
export function TaskNumberBadge({
  number,
  className,
}: {
  number?: string | null
  className?: string
}) {
  if (!number) return null
  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center rounded-md bg-primary-50 px-1.5 py-px font-mono text-[10px] font-semibold leading-4 text-primary-700',
        className,
      )}
    >
      {number}
    </span>
  )
}
