import { cn } from '@/lib/utils'

/** Skeleton（shadcn-ui new-york 风格）：加载占位，纯背景色差，无发光。 */
function Skeleton({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="skeleton"
      className={cn('animate-pulse rounded-md bg-muted', className)}
      {...props}
    />
  )
}

export { Skeleton }
