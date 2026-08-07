import * as React from 'react'
import { Toaster as Sonner, type ToasterProps } from 'sonner'
import { cn } from '@/lib/utils'

/**
 * Toast 容器（TF-037 重新设计布局）：
 * ┌─────────────────────────────────────────┐
 * │ [icon]  [标题         ]          [×]    │
 * │         [详情/描述     ]                 │
 * └─────────────────────────────────────────┘
 * - 左侧：通知图标（sonner 按类型渲染，size-5）
 * - 中间：标题 + 详情上下分布（content flex-col，flex-1 占满）
 * - 右侧：关闭按钮（order-3 + static 静态流重排，margin-left auto 靠右）
 * - 顶部居中；3s 自动关闭；theme 跟随 html.dark
 *
 * 实现要点（避免 specificity 坑）：
 * - sonner 默认 close button 为 absolute + left:0 + transform: translate(-35%,-35%)
 *   （[data-sonner-toast][data-styled=true] [data-close-button]，specificity 0,3,0）；
 *   这里改用「静态流 + flex order 重排」彻底绕开 absolute 定位，
 *   需要覆盖的冲突属性（position/transform/width/height/背景/边框）统一加
 *   Tailwind v4 后缀 `!`（important）穿透。
 * - icon / content 通过 sonner classNames.icon / classNames.content 钩子加 order。
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
      duration={3000}
      closeButton
      toastOptions={{
        classNames: {
          toast:
            'group toast group-[.toaster]:max-w-[min(92vw,440px)] group-[.toaster]:rounded-xl group-[.toaster]:border-border group-[.toaster]:shadow-[var(--shadow-pop)] group-[.toast]:gap-2.5!',
          // 左侧图标（order-1 默认首位；放大到 20px 需覆盖 sonner 16px）
          icon: 'group-[.toast]:order-1 group-[.toast]:size-5! group-[.toast]:shrink-0',
          // 中间：标题 + 详情上下分布（sonner content 默认 flex-col），flex-1 撑满
          content: 'group-[.toast]:order-2 group-[.toast]:min-w-0 group-[.toast]:flex-1',
          title: 'group-[.toast]:text-sm group-[.toast]:font-semibold group-[.toast]:break-words',
          description:
            'group-[.toast]:text-xs group-[.toast]:text-muted-foreground group-[.toast]:break-words',
          // 右侧关闭按钮：静态流 + order-3 重排到最右，覆盖 sonner 默认 absolute 定位
          closeButton:
            'group-[.toast]:order-3! group-[.toast]:static! group-[.toast]:transform-none! ' +
            'group-[.toast]:ml-auto! group-[.toast]:size-6! group-[.toast]:flex! ' +
            'group-[.toast]:items-center! group-[.toast]:justify-center! ' +
            'group-[.toast]:rounded-full! group-[.toast]:bg-transparent! group-[.toast]:border-0! ' +
            'group-[.toast]:p-0! group-[.toast]:shadow-none! ' +
            'group-[.toast]:text-muted-foreground! group-[.toast]:transition-colors! ' +
            'hover:group-[.toast]:bg-accent! hover:group-[.toast]:text-accent-foreground!',
          success: 'group-[.toast]:[&>svg]:text-success',
          error: 'group-[.toast]:[&>svg]:text-destructive',
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
