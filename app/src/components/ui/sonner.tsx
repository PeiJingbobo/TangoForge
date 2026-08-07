import * as React from 'react'
import { Toaster as Sonner, type ToasterProps } from 'sonner'
import { cn } from '@/lib/utils'

/**
 * Toast 容器（sonner 封装，TF-037 全局提示优化）：
 * - 位置：顶部居中（原右下角）；
 * - 手动关闭：右上角 X 按钮（closeButton，同时保留 3s 自动关闭）；
 * - 长文本：允许换行（whitespace-normal + break-words），长报错完整展示；
 * - theme 跟随 html.dark（docs/UI-VISION.md 明暗跟随系统）。
 */
function useSonnerTheme(): 'light' | 'dark' {
  const [theme, setTheme] = React.useState<'light' | 'dark'>(() =>
    document.documentElement.classList.contains('dark') ? 'dark' : 'light',
  )
  React.useEffect(() => {
    const el = document.documentElement
    const observer = new MutationObserver(() => {
      setTheme(el.classList.contains('dark') ? 'dark' : 'light')
    })
    observer.observe(el, { attributes: true, attributeFilter: ['class'] })
    return () => observer.disconnect()
  }, [])
  return theme
}

function Toaster({ className, ...props }: ToasterProps) {
  const theme = useSonnerTheme()
  return (
    <Sonner
      theme={theme}
      className={cn('toaster group', className)}
      position="top-center"
      offset={20}
      closeButton
      duration={3000}
      toastOptions={{
        classNames: {
          toast:
            'group toast group-[.toaster]:max-w-[min(92vw,440px)] group-[.toaster]:rounded-xl group-[.toaster]:border-border group-[.toaster]:shadow-[var(--shadow-pop)] group-[.toaster]:whitespace-normal group-[.toaster]:break-words group-[.toaster]:pr-8',
          title:
            'group-[.toast]:text-sm group-[.toast]:font-semibold group-[.toast]:whitespace-normal group-[.toast]:break-words',
          description:
            'group-[.toast]:text-muted-foreground group-[.toast]:whitespace-normal group-[.toast]:break-words',
          actionButton: 'group-[.toast]:bg-primary group-[.toast]:text-primary-foreground',
          cancelButton: 'group-[.toast]:bg-muted group-[.toast]:text-muted-foreground',
          // 关闭按钮：卡片内部右侧对齐、上下居中（TF-037 优化）。
          // sonner 默认用 `transform: translate(-35%, -35%)` + `left:0/right:unset/top:0`
          // 把按钮推到 toast 左外上方；其选择器 specificity (0,3,0) 高于 Tailwind group 变体 (0,1,0)，
          // 必须用 `!important` 全面覆盖定位 + arbitrary `transform` 重置才能贴在卡片右中。
          // 关闭按钮：卡片内部右侧对齐、上下居中（TF-037 优化）。
          // sonner 默认用 `transform: translate(-35%, -35%)` + `left:0/right:unset/top:0/width:height:20px`
          // 把按钮推到 toast 左外上方；其选择器 specificity (0,3,0) 高于 Tailwind group 变体 (0,1,0)，
          // 必须用 `!important` 全面覆盖定位 + 重置 transform 才能贴在卡片右中。
          // Tailwind v4 important 修饰符用后缀 `utility!`（覆盖 v3 前缀 `!utility`）。
          closeButton:
            'absolute top-[50%]! right-1! size-6! flex! items-center! justify-center! ' +
            'rounded-full! bg-transparent! border-0! p-0! shadow-none! ' +
            'text-muted-foreground! transition-colors! hover:bg-accent! hover:text-accent-foreground! ' +
            'transform-none!',
          success: 'group-[.toast]:[&>svg]:text-success',
          error: 'group-[.toast]:[&>svg]:text-destructive',
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
