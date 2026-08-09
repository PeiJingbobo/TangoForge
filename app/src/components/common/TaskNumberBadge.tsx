import { Copy } from 'lucide-react'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'

/**
 * 任务编号徽标（TF-040）：展示简短编号（T01/P0…）。
 * 有编号 → 等宽小徽标；无编号 → 不渲染。
 * QA 2026-08-09：点击复制编号到剪贴板（hover 显示复制图标；stopPropagation
 * 避免触发所在行/卡片的打开详情；复制通道：navigator.clipboard → Electron
 * IPC → execCommand 兜底）。
 */
export function TaskNumberBadge({
  number,
  className,
}: {
  number?: string | null
  className?: string
}) {
  if (!number) return null

  const copy = async (e: React.MouseEvent | React.KeyboardEvent) => {
    e.stopPropagation()
    e.preventDefault()
    const ok = await copyText(number)
    toast[ok ? 'success' : 'error'](ok ? `已复制编号 ${number}` : '复制编号失败')
  }

  return (
    <span
      role="button"
      tabIndex={0}
      title="点击复制编号"
      aria-label={`复制编号 ${number}`}
      onClick={(e) => void copy(e)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') void copy(e)
      }}
      className={cn(
        'group inline-flex shrink-0 cursor-pointer items-center gap-0.5 rounded-md bg-primary-50 px-1.5 py-px font-mono text-[10px] font-semibold leading-4 text-primary-700 transition-colors hover:bg-primary-100 hover:text-primary-800',
        className,
      )}
    >
      {number}
      <Copy className="size-2.5 opacity-0 transition-opacity group-hover:opacity-70" />
    </span>
  )
}

/** 复制文本：navigator.clipboard → Electron IPC（file:// 可靠）→ execCommand 兜底。 */
async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // 继续降级
  }
  try {
    if (window.tangoforge?.clipboard) {
      await window.tangoforge.clipboard.writeText(text)
      return true
    }
  } catch {
    // 继续降级
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}
