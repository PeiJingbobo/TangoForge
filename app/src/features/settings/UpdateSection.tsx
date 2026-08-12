import { useCallback, useEffect, useState } from 'react'
import {
  CheckCircle2,
  Download,
  ExternalLink,
  Loader2,
  RefreshCw,
  TriangleAlert,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import type { UpdatePayload, UpdateStatus } from '@/types/update'

/**
 * 全局设置「关于」板块（TF-036，docs/CI-CD-UPDATER.md）：
 * - 当前版本 + 「检查更新」；更新状态由主进程推送（IPC update:state），此处订阅展示；
 * - Windows：检测到新版本后「下载并安装」→「重启并安装」（electron-updater）；
 * - macOS（未签名阶段）：检测到新版本后提供「打开下载」（自动打开 dmg 由用户手动安装）。
 */
export function UpdateSection() {
  const update = window.tangoforge?.update
  const [status, setStatus] = useState<UpdateStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)

  const refresh = useCallback(async () => {
    if (!update) {
      setStatus(null)
      setLoading(false)
      return
    }
    try {
      setStatus(await update.getState())
    } finally {
      setLoading(false)
    }
  }, [update])

  useEffect(() => {
    if (!update) {
      setStatus(null)
      setLoading(false)
      return
    }
    const unsubscribe = update.onState((payload: UpdatePayload) => {
      setStatus((s) => (s ? { ...s, ...payload } : s))
    })
    void refresh()
    return unsubscribe
  }, [update, refresh])

  const act = async (fn: () => Promise<boolean> | boolean) => {
    setBusy(true)
    try {
      await fn()
    } finally {
      setBusy(false)
    }
  }

  if (!update) {
    return <p className="text-sm text-muted-foreground">在线更新仅在桌面端安装版（App）中可用。</p>
  }

  if (loading && !status) return <Skeleton className="h-40 w-full" />

  const s = status
  const supported = s?.supported ?? false
  const isDownloading = s?.state === 'downloading'
  const checking = s?.state === 'checking'

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h3 className="text-sm font-medium text-foreground">TangoForge 更新</h3>
          <p className="mt-1 text-caption text-muted-foreground">
            当前版本：
            <code className="font-mono text-[11px]">{s?.currentVersion ?? '—'}</code>
            {!supported && '（当前为开发/未打包构建，在线更新不可用）'}
          </p>
        </div>
        {supported && (
          <Button
            size="sm"
            variant="outline"
            disabled={busy || checking || isDownloading}
            onClick={() => void act(() => update.check())}
          >
            {checking ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : (
              <RefreshCw className="size-3.5" />
            )}
            {checking ? '检查中…' : '检查更新'}
          </Button>
        )}
      </div>

      {!supported && (
        <div className="rounded-lg border border-border bg-muted/30 p-3 text-sm text-muted-foreground">
          请从 GitHub Releases 下载安装版后使用在线更新功能。
        </div>
      )}

      {supported && <StatusPanel status={s} busy={busy} onAct={act} />}
    </div>
  )
}

function StatusPanel({
  status,
  busy,
  onAct,
}: {
  status: UpdateStatus | null
  busy: boolean
  onAct: (fn: () => Promise<boolean> | boolean) => Promise<void>
}) {
  const update = window.tangoforge!.update!
  const st = status?.state ?? 'idle'

  if (st === 'checking') {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" /> 正在检查更新…
      </div>
    )
  }

  if (st === 'available') {
    return (
      <div className="space-y-3 rounded-lg border border-border p-3">
        <div className="flex items-center gap-2">
          <Badge variant="default">发现新版本 v{status?.version}</Badge>
          <span className="text-xs text-muted-foreground">
            macOS 需手动下载安装；Windows 可直接下载更新
          </span>
        </div>
        {status?.releaseNotes && (
          <pre className="max-h-40 overflow-y-auto whitespace-pre-wrap rounded-md bg-muted/30 p-2 font-sans text-xs text-muted-foreground">
            {status.releaseNotes}
          </pre>
        )}
        {status?.downloadUrl ? (
          <Button disabled={busy} onClick={() => void onAct(() => update.openDownload())}>
            <ExternalLink className="size-4" /> 打开下载
          </Button>
        ) : (
          <Button disabled={busy} onClick={() => void onAct(() => update.download())}>
            <Download className="size-4" /> 下载并安装
          </Button>
        )}
      </div>
    )
  }

  if (st === 'downloading') {
    const percent = status?.percent ?? 0
    return (
      <div className="space-y-2 rounded-lg border border-border p-3">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" /> 正在下载更新… {percent}%
        </div>
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
          <div
            className="h-full rounded-full bg-primary-500 transition-[width]"
            style={{ width: `${percent}%` }}
            role="progressbar"
            aria-valuenow={percent}
            aria-valuemin={0}
            aria-valuemax={100}
          />
        </div>
      </div>
    )
  }

  if (st === 'downloaded') {
    return (
      <div className="flex items-center justify-between gap-3 rounded-lg border border-border p-3">
        <div className="flex items-center gap-2 text-sm">
          <CheckCircle2 className="size-4 text-success" />
          新版本 v{status?.version} 已下载
        </div>
        <Button disabled={busy} onClick={() => void onAct(() => update.install())}>
          重启并安装
        </Button>
      </div>
    )
  }

  if (st === 'not-available') {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <CheckCircle2 className="size-4 text-success" /> 当前已是最新版本
      </div>
    )
  }

  if (st === 'error') {
    return (
      <div className="space-y-2 rounded-lg border border-destructive-200 bg-destructive-soft p-3">
        <div className="flex items-center gap-2 text-sm text-destructive-ink">
          <TriangleAlert className="size-4" /> 检查更新失败：{status?.error}
        </div>
        <Button
          size="sm"
          variant="outline"
          disabled={busy}
          onClick={() => void onAct(() => update.check())}
        >
          重试
        </Button>
      </div>
    )
  }

  return (
    <p className="text-caption text-muted-foreground">
      点击「检查更新」检测 GitHub Releases 是否有新版本。
    </p>
  )
}
