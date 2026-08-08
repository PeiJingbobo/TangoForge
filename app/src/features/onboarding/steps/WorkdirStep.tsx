import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  FolderCheck,
  Loader2,
  RefreshCw,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useProjectCheck, useResetProjectMetadata, useImportProject } from '@/hooks/useProjects'

/**
 * 引导 Step 0：确认工作目录（TF-041）。
 * 流程：检查已导入 / 历史元数据合法性 → 非法提示清空 → 初始化并注册项目。
 * 注册成功后 onReady(true)——后续步骤（导入/权限/Skill）依赖项目上下文。
 * 检查/注册失败 → 显示错误 + 重试按钮（不静默卡住）。
 */
export function WorkdirStep({
  workdir,
  onReady,
}: {
  workdir: string
  onReady: (ok: boolean) => void
}) {
  const check = useProjectCheck()
  const reset = useResetProjectMetadata()
  const importProject = useImportProject()
  const [askReset, setAskReset] = useState(false)
  const [imported, setImported] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  // workdir 或重试计数变化时执行检查。
  useEffect(() => {
    if (!workdir) return
    setError(null)
    setAskReset(false)
    setImported(false)
    check.mutate(workdir, {
      onSuccess: (res) => {
        if (res.registered) {
          // 已导入：提示后直接放行（onReady）——不重复初始化。
          setImported(true)
          onReady(true)
        } else if (res.has_meta && !res.meta_valid) {
          setAskReset(true) // 元数据非法 → 询问清空
        } else {
          doImport() // 无元数据 / 元数据合法 → 直接注册（初始化或复用）
        }
      },
      onError: (e) => {
        setError(e instanceof Error ? e.message : '目录检查失败')
      },
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workdir, attempt])

  const doImport = () => {
    importProject.mutate(
      { workdir },
      {
        onSuccess: () => {
          setImported(true)
          onReady(true)
        },
        onError: (e) => setError(e instanceof Error ? e.message : '项目初始化失败'),
      },
    )
  }

  const doReset = () => {
    reset.mutate(workdir, {
      onSuccess: () => {
        toast.success('已清空历史元数据')
        setAskReset(false)
        doImport()
      },
      onError: (e) => setError(e instanceof Error ? e.message : '清空失败'),
    })
  }

  const busy = check.isPending || importProject.isPending || reset.isPending

  return (
    <div className="space-y-4">
      {/* 目录信息 */}
      <div className="flex items-start gap-3 rounded-xl border border-divider bg-muted/50 p-4">
        <FolderCheck className="mt-0.5 size-5 shrink-0 text-primary-600" />
        <div className="min-w-0">
          <div className="text-sm font-semibold">工作目录</div>
          <div className="mt-1 truncate font-mono text-xs text-muted-foreground" title={workdir}>
            {workdir}
          </div>
        </div>
      </div>

      {/* 检查中 */}
      {busy && (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" /> 正在检查并初始化目录…
        </div>
      )}

      {/* 失败 → 错误 + 重试（修复静默卡住） */}
      {error && !busy && (
        <div className="flex items-start gap-2 rounded-xl border border-destructive-soft bg-destructive-soft/60 p-4">
          <AlertCircle className="mt-0.5 size-5 shrink-0 text-destructive-ink" />
          <div className="flex-1 text-sm">
            <div className="font-semibold">目录检查失败</div>
            <div className="mt-1 text-muted-foreground">{error}</div>
            <Button
              size="sm"
              variant="outline"
              className="mt-3"
              onClick={() => setAttempt((a) => a + 1)}
            >
              <RefreshCw className="size-3.5" /> 重试
            </Button>
          </div>
        </div>
      )}

      {/* 已导入 */}
      {imported && !busy && (
        <div className="flex items-start gap-2 rounded-xl border border-success-soft bg-success-soft/60 p-4">
          <CheckCircle2 className="mt-0.5 size-5 shrink-0 text-success" />
          <div className="text-sm">
            <div className="font-semibold">目录已就绪</div>
            <div className="mt-0.5 text-muted-foreground">
              {check.data?.registered
                ? '该目录已是 TangoForge 项目（复用现有元数据）。'
                : '项目元数据已初始化并注册，可继续后续设置。'}
            </div>
          </div>
        </div>
      )}

      {/* 元数据非法 → 询问清空 */}
      {askReset && !busy && (
        <div className="flex items-start gap-2 rounded-xl border border-warning-soft bg-warning-soft/60 p-4">
          <AlertTriangle className="mt-0.5 size-5 shrink-0 text-warning-ink" />
          <div className="flex-1 text-sm">
            <div className="font-semibold">检测到历史遗留元数据（版本过旧或损坏）</div>
            <div className="mt-1 text-muted-foreground">
              {check.data?.meta_reason ?? '元数据无法解析。'}{' '}
              可选择清空后重新初始化，或忽略（可能无法正常使用）。
            </div>
            <div className="mt-3 flex gap-2">
              <Button size="sm" onClick={doReset} disabled={reset.isPending}>
                {reset.isPending ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : (
                  <RefreshCw className="size-3.5" />
                )}
                清空并重新初始化
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  setAskReset(false)
                  doImport()
                }}
              >
                忽略并继续
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
