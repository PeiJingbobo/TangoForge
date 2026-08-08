import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { CheckCircle2, Loader2, TerminalSquare, TriangleAlert } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'

/**
 * 全局设置「CLI」板块（QA 2026-08-08）：
 * 注册状态检测（where/command -v 探测 PATH 可用性）+ 注册/卸载到全局 + 已注册路径显示。
 * 数据层在主进程（cli-register.ts，IPC cli:status/register/unregister）；
 * Web/测试环境无 window.tangoforge.cli → 提示仅桌面端可用。
 */

export interface CliStatus {
  registered: boolean
  path: string | null
  ours: boolean
  cliPath: string
}

export function CLISettingsSection() {
  const [status, setStatus] = useState<CliStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    if (!window.tangoforge?.cli) {
      setStatus(null)
      setLoading(false)
      return
    }
    try {
      setStatus(await window.tangoforge.cli.status())
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const act = async (
    key: 'register' | 'unregister',
    fn: () => Promise<{ ok: boolean; message: string }>,
  ) => {
    setBusy(key)
    try {
      const r = await fn()
      if (r.ok) toast.success(r.message)
      else toast.error(r.message)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '操作失败')
    } finally {
      setBusy(null)
      setLoading(true)
      await refresh()
    }
  }

  if (!window.tangoforge?.cli) {
    return (
      <p className="text-sm text-muted-foreground">
        CLI 注册仅在桌面端 App 中可用（Web 预览无系统 PATH 权限）。
      </p>
    )
  }

  if (loading && !status) return <Skeleton className="h-40 w-full" />

  const registered = status?.registered ?? false

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h3 className="text-sm font-medium text-foreground">CLI（tangoforge）</h3>
          <p className="mt-1 text-caption text-muted-foreground">
            注册后可在任意终端（或由 AI 经 shell）直接调用{' '}
            <code className="font-mono text-[11px]">tangoforge</code>
            ，与 App 等价操作任务/导入/导出/Skill（经守护进程 HTTP）。
          </p>
        </div>
        <Badge variant={registered ? 'success' : 'warning'} className="shrink-0">
          {registered ? (
            <>
              <CheckCircle2 className="size-3" /> 已注册
            </>
          ) : (
            <>
              <TriangleAlert className="size-3" /> 未注册
            </>
          )}
        </Badge>
      </div>

      {!registered && (
        <div className="rounded-lg border border-warning/30 bg-warning/5 p-3 text-sm text-warning-ink">
          tangoforge 尚未注册到全局命令。注册后即可在任意终端使用，也可让 AI 助手通过 shell 运行
          tangoforge 完成项目操作（未注册时 AI 需使用完整可执行文件路径）。
        </div>
      )}

      <div className="space-y-1.5 text-xs text-muted-foreground">
        <div className="flex items-center gap-2">
          <TerminalSquare className="size-3.5 shrink-0" />
          <span>本机 CLI 路径：</span>
          <code className="truncate font-mono text-[11px]">{status?.cliPath ?? '—'}</code>
        </div>
        {registered && status?.path && (
          <div className="flex items-center gap-2">
            <CheckCircle2 className="size-3.5 shrink-0 text-success" />
            <span>当前解析：</span>
            <code className="truncate font-mono text-[11px]">{status.path}</code>
            {!status.ours && <span className="text-warning-ink">（非本 App 分发的 CLI）</span>}
          </div>
        )}
      </div>

      <div className="flex items-center gap-2">
        {registered ? (
          <Button
            variant="outline"
            disabled={busy !== null}
            onClick={() => void act('unregister', () => window.tangoforge!.cli!.unregister())}
          >
            {busy === 'unregister' && <Loader2 className="size-4 animate-spin" />}
            卸载注册
          </Button>
        ) : (
          <Button
            disabled={busy !== null}
            onClick={() => void act('register', () => window.tangoforge!.cli!.register())}
          >
            {busy === 'register' && <Loader2 className="size-4 animate-spin" />}
            注册到全局
          </Button>
        )}
        <Button variant="ghost" size="sm" disabled={busy !== null} onClick={() => void refresh()}>
          刷新状态
        </Button>
      </div>
    </div>
  )
}
