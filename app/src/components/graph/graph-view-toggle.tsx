import { cn } from '@/lib/utils'
import type { GraphViewMode } from '@/hooks/useGraphViewMode'

/**
 * PERT / 力导向 切换控件（Pill 风格，与 TabsTrigger 视觉一致）。
 */
export function GraphViewToggle({
  mode,
  onChange,
  disabled,
}: {
  mode: GraphViewMode
  onChange: (m: GraphViewMode) => void
  disabled?: boolean
}) {
  const options: { value: GraphViewMode; label: string }[] = [
    { value: 'pert', label: 'PERT 图' },
    { value: 'force', label: '力导向' },
  ]
  return (
    <div
      className="inline-flex h-9 w-fit items-center gap-1 rounded-full bg-muted p-1"
      role="tablist"
      aria-label="全景图渲染方式"
    >
      {options.map((o) => (
        <button
          key={o.value}
          role="tab"
          aria-selected={mode === o.value}
          disabled={disabled}
          onClick={() => onChange(o.value)}
          className={cn(
            'inline-flex h-7 items-center justify-center rounded-full px-3 text-xs font-semibold whitespace-nowrap transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50',
            mode === o.value ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground',
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}
