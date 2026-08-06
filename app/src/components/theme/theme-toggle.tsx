import * as React from 'react'
import { Palette, Moon, Sun, Monitor } from 'lucide-react'
import {
  ACCENT_PRESETS,
  applyAccent,
  applyThemeMode,
  resolveMode,
  type ThemeMode,
} from '@/lib/theme'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

/**
 * 主题切换（临时入口，TF-022 验收用；TF-028 设置页将正式化并移除本组件顶栏挂载）。
 * 能力：浅色/深色/跟随系统 + 预设主色 + 自定义取色（动态计算色阶）。
 */
const MODES: { key: ThemeMode; label: string; icon: typeof Sun }[] = [
  { key: 'light', label: '浅色', icon: Sun },
  { key: 'dark', label: '深色', icon: Moon },
  { key: 'system', label: '跟随系统', icon: Monitor },
]

function ThemeToggle() {
  const [mode, setMode] = React.useState<ThemeMode>('system')
  const [accent, setAccent] = React.useState('sky-blue')
  const pickerRef = React.useRef<HTMLInputElement>(null)

  // 初始化：默认 sky + 跟随系统（UI-VISION 浅色优先）
  React.useEffect(() => {
    applyThemeMode('system')
    applyAccent('sky-blue', resolveMode('system'))
  }, [])

  const changeMode = (m: ThemeMode) => {
    setMode(m)
    const resolved = applyThemeMode(m)
    // 自定义主色需按主题重算（applyThemeMode 已处理 custom），预设走 CSS 类
    if (document.documentElement.dataset.accent !== 'custom') {
      applyAccent(accent, resolved)
    }
  }

  const changeAccent = (key: string) => {
    setAccent(key)
    applyAccent(key, resolveMode(mode))
  }

  const changeCustom = (hex: string) => {
    setAccent(hex)
    applyAccent(hex, resolveMode(mode))
  }

  return (
    <div className="flex items-center gap-2">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            size="icon"
            className="h-8 w-8 rounded-full"
            aria-label="主题设置"
          >
            <Palette className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuLabel>界面模式</DropdownMenuLabel>
          {MODES.map((m) => {
            const Icon = m.icon
            return (
              <DropdownMenuItem key={m.key} onClick={() => changeMode(m.key)}>
                <Icon className="size-4" />
                {m.label}
                {mode === m.key && <span className="ml-auto text-primary">✓</span>}
              </DropdownMenuItem>
            )
          })}
          <DropdownMenuSeparator />
          <DropdownMenuLabel>主强调色</DropdownMenuLabel>
          <div className="flex items-center gap-2 px-2 py-1.5">
            {ACCENT_PRESETS.map((p) => (
              <button
                key={p.key}
                type="button"
                title={p.label}
                onClick={() => changeAccent(p.key)}
                className={cn(
                  'size-5 rounded-full border-2 border-transparent transition-transform hover:scale-110',
                  accent === p.key && 'border-foreground',
                )}
                style={{ backgroundColor: p.hex }}
                aria-label={`主色 ${p.label}`}
              />
            ))}
            <button
              type="button"
              title="自定义主色"
              onClick={() => pickerRef.current?.click()}
              className={cn(
                'relative size-5 overflow-hidden rounded-full border-2 border-transparent transition-transform hover:scale-110',
                !ACCENT_PRESETS.some((p) => p.key === accent) && 'border-foreground',
              )}
              style={{
                background:
                  'conic-gradient(#f85a5a,#f7b84b,#e8f06b,#4ee57c,#4a7fff,#9b5cff,#f85a5a)',
              }}
              aria-label="自定义主色"
            />
            <input
              ref={pickerRef}
              type="color"
              className="sr-only"
              value={ACCENT_PRESETS.some((p) => p.key === accent) ? ACCENT_PRESETS[0].hex : accent}
              onChange={(e) => changeCustom(e.target.value)}
            />
          </div>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

export { ThemeToggle }
