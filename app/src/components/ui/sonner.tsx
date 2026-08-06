import * as React from 'react'
import { Toaster as Sonner, type ToasterProps } from 'sonner'
import { cn } from '@/lib/utils'

/**
 * Toast 容器（sonner 封装）
 * theme 跟随 html.dark（docs/UI-VISION.md 明暗跟随系统）。
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
      position="bottom-right"
      toastOptions={{
        classNames: {
          toast:
            'group toast group-[.toaster]:rounded-xl group-[.toaster]:border-border group-[.toaster]:shadow-[var(--shadow-pop)]',
          title: 'group-[.toast]:text-sm group-[.toast]:font-semibold',
          description: 'group-[.toast]:text-muted-foreground',
          success: 'group-[.toast]:[&>svg]:text-success',
          error: 'group-[.toast]:[&>svg]:text-destructive',
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
