import { useEffect, useState } from 'react'
import { Minus, Square, X } from 'lucide-react'
import { cn } from '@/lib/utils'

/**
 * 自绘标题栏（TF-038 双平台无边框窗口）：
 * - macOS：hiddenInset 原生红黄绿保留在左上角，本组件只提供高度 + 拖拽区
 *   （左侧留白约 78px 避免内容与交通灯重叠）；
 * - Windows：frame:false 无系统按钮，右侧自绘「最小化 / 最大化切换 / 关闭」，
 *   最大化图标随状态切换（事件订阅），按钮 no-drag；
 * - 非桌面环境（Web 预览）：不渲染（占位 0 高度，不影响布局）。
 *
 * 拖拽：-webkit-app-region: drag；交互元素必须 no-drag。
 */
export function WindowTitleBar() {
  const win = typeof window !== 'undefined' ? window.tangoforge?.window : undefined
  if (!win) return null
  return <TitleBarInner win={win} />
}

interface WinCtl {
  platform: string
  minimize: () => Promise<void>
  toggleMaximize: () => Promise<void>
  close: () => Promise<void>
  isMaximized: () => Promise<boolean>
  onMaximizedChange: (cb: (m: boolean) => void) => () => void
}

function TitleBarInner({ win }: { win: WinCtl }) {
  const isMac = win.platform === 'darwin'
  const isWin = win.platform === 'win32'
  const [maximized, setMaximized] = useState(false)

  // 初始查询 + 订阅最大化状态（仅 Windows 需要，但统一处理无妨）。
  useEffect(() => {
    let mounted = true
    void win.isMaximized().then((m) => {
      if (mounted) setMaximized(m)
    })
    const off = win.onMaximizedChange((m) => setMaximized(m))
    return () => {
      mounted = false
      off()
    }
  }, [win])

  // 非 mac/win 桌面（linux 等）：保留系统标题栏，不渲染自绘条。
  if (!isMac && !isWin) return null

  return (
    <div
      className={cn(
        'relative flex h-9 shrink-0 select-none items-center border-b border-divider bg-card',
        // 拖拽区（mac 整条可拖；win 仅中间空白区可拖，按钮 no-drag）
        '[-webkit-app-region:drag]',
        // mac：左侧为交通灯留白（红黄绿），内容从 ~78px 起
        isMac && 'pl-[78px]',
      )}
    >
      {/* 中部：应用名（可选；保持居中弱化） */}
      {isMac && (
        <span className="pointer-events-none ml-auto mr-auto text-xs font-medium text-muted-foreground/70">
          TangoForge
        </span>
      )}

      {/* Windows 右侧控制按钮 */}
      {isWin && (
        <div
          className="absolute inset-y-0 right-0 flex items-stretch [-webkit-app-region:no-drag]"
          aria-label="窗口控制"
        >
          <TitleBarButton label="最小化" onClick={() => void win.minimize()}>
            <Minus className="size-4" />
          </TitleBarButton>
          <TitleBarButton
            label={maximized ? '还原' : '最大化'}
            onClick={() => void win.toggleMaximize()}
          >
            <Square className={cn('size-3.5', maximized && 'scale-90 opacity-80')} />
          </TitleBarButton>
          <TitleBarButton label="关闭" onClick={() => void win.close()} danger>
            <X className="size-4" />
          </TitleBarButton>
        </div>
      )}
    </div>
  )
}

function TitleBarButton({
  label,
  onClick,
  danger,
  children,
}: {
  label: string
  onClick: () => void
  danger?: boolean
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      className={cn(
        'grid w-11 place-items-center text-muted-foreground transition-colors',
        danger
          ? 'hover:bg-destructive hover:text-destructive-foreground'
          : 'hover:bg-accent hover:text-accent-foreground',
      )}
    >
      {children}
    </button>
  )
}
