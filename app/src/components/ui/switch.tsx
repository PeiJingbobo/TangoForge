import * as React from 'react'
import * as SwitchPrimitives from '@radix-ui/react-switch'
import { cn } from '@/lib/utils'

/**
 * Switch（shadcn-ui new-york 风格）
 * 开启态主色填充（docs/UI-VISION.md demo 表单规格）。
 */
function Switch({ className, ...props }: React.ComponentProps<typeof SwitchPrimitives.Root>) {
  return (
    <SwitchPrimitives.Root
      data-slot="switch"
      className={cn(
        'peer inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary data-[state=unchecked]:bg-input',
        className,
      )}
      {...props}
    >
      <SwitchPrimitives.Thumb
        data-slot="switch-thumb"
        className="pointer-events-none block size-5 rounded-full bg-background shadow-lg ring-0 transition-transform data-[state=checked]:translate-x-5 data-[state=unchecked]:translate-x-0"
      />
    </SwitchPrimitives.Root>
  )
}

/**
 * SwitchState（TF-036 权限界面）：轨道内嵌「允许 / 拒绝」文字的开关变体。
 * checked → 主色轨道 + 左侧白字「允许」；unchecked → 灰轨道 + 右侧灰字「拒绝」。
 * 受控组件：调用方持有 checked / onCheckedChange（Radix Switch 语义一致，role=switch）。
 */
function SwitchState({
  checked,
  onCheckedChange,
  disabled,
  className,
  'aria-label': ariaLabel,
}: {
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  disabled?: boolean
  className?: string
  'aria-label'?: string
}) {
  return (
    <SwitchPrimitives.Root
      data-slot="switch-state"
      checked={checked}
      onCheckedChange={onCheckedChange}
      disabled={disabled}
      aria-label={ariaLabel}
      className={cn(
        'relative inline-flex h-7 w-[76px] shrink-0 cursor-pointer items-center rounded-full transition-colors',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
        'disabled:cursor-not-allowed disabled:opacity-50',
        checked ? 'bg-primary' : 'bg-input',
        className,
      )}
    >
      {/* 允许（checked 时左侧可见） */}
      <span
        aria-hidden
        className={cn(
          'pointer-events-none absolute left-2.5 text-[11px] leading-none font-semibold transition-opacity duration-150',
          checked ? 'text-primary-foreground opacity-100' : 'opacity-0',
        )}
      >
        允许
      </span>
      {/* 拒绝（unchecked 时右侧可见） */}
      <span
        aria-hidden
        className={cn(
          'pointer-events-none absolute right-2.5 text-[11px] leading-none font-semibold transition-opacity duration-150',
          checked ? 'opacity-0' : 'text-muted-foreground opacity-100',
        )}
      >
        拒绝
      </span>
      <SwitchPrimitives.Thumb
        data-slot="switch-state-thumb"
        className={cn(
          'pointer-events-none absolute top-0.5 block size-6 rounded-full bg-background shadow-lg ring-0 transition-[left] duration-150',
          checked ? 'left-[50px]' : 'left-0.5',
        )}
      />
    </SwitchPrimitives.Root>
  )
}

export { Switch, SwitchState }
