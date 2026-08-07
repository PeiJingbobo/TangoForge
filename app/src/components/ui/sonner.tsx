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
          // 关闭按钮：卡片内部右侧对齐、上下居中（TF-037 优化）
          closeButton:
            'group-[.toast]:absolute group-[.toast]:top-1/2 group-[.toast]:right-1 group-[.toast]:-translate-y-1/2 group-[.toast]:size-6 group-[.toast]:grid group-[.toast]:place-items-center group-[.toast]:rounded-full group-[.toast]:text-muted-foreground group-[.toast]:transition-colors group-[.toast]:hover:bg-accent group-[.toast]:hover:text-accent-foreground',
          success: 'group-[.toast]:[&>svg]:text-success',
          error: 'group-[.toast]:[&>svg]:text-destructive',
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
