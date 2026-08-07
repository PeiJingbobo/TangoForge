import * as React from 'react'
import { Toaster as Sonner, type ToasterProps } from 'sonner'
import { cn } from '@/lib/utils'

/**
 * Toast 容器（shadcn-ui 原始 Toaster + 功能配置，TF-037）：
 * - 位置：顶部居中（原右下角）；
 * - 手动关闭：closeButton 启用 X 按钮（sonner 默认样式，不再自定义样式避免与 specificity 冲突）；
 * - 自动关闭：3s；
 * - 长文本换行：sonner 默认样式已含 overflow-wrap: anywhere（styles.css L86），无需自定义；
 * - theme 跟随 html.dark（docs/UI-VISION.md 明暗跟随系统）。
 *
 * 简化原因：之前用 `!important` 后缀类覆盖 sonner 默认 close button 样式
 * （[data-sonner-toast][data-styled=true] [data-close-button] 选择器 specificity 0,3,0）
 * 导致与 Tailwind utility 打架出现样式错乱；还原到 shadcn 模板最稳定。
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
      {...props}
    />
  )
}

export { Toaster }
