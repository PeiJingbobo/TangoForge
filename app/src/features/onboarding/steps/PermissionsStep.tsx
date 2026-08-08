import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { CheckCircle2, Loader2, ShieldCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { usePermissions, useSavePermissions } from '@/hooks/usePermissions'
import { ACTION_LABELS, type PermissionMap } from '@/types/models'

/**
 * 引导 Step 3：Agent 权限配置（TF-041 简化版）。
 * 默认只读 5 项；可勾选常用写操作（task.create/update/update_status/import/export）。
 * 保存后 onReady(true)；也可保留默认直接继续。
 */
export function PermissionsStep({
  workdir,
  onReady,
}: {
  workdir: string
  onReady: (ok: boolean) => void
}) {
  const { data, isLoading } = usePermissions(workdir)
  const save = useSavePermissions(workdir)
  const [draft, setDraft] = useState<PermissionMap | null>(null)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (data?.actions && !draft) {
      setDraft({ ...data.actions })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  // 引导页仅展示常用写操作 + 只读基础项。
  const FOCUS_KEYS = [
    'task.read',
    'task.create',
    'task.update',
    'task.update_status',
    'task.delete',
    'import.run',
    'import.confirm',
    'export.run',
    'graph.read',
    'skill.read',
  ] as const

  const toggle = (key: keyof PermissionMap) => {
    if (!draft) return
    setDraft({ ...draft, [key]: !draft[key] })
    setSaved(false)
  }

  const doSave = () => {
    if (!draft) return
    save.mutate(draft, {
      onSuccess: () => {
        setSaved(true)
        toast.success('权限已保存')
        onReady(true)
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : '保存权限失败'),
    })
  }

  const skip = () => {
    toast.info('已保留默认权限（只读），可在「权限」页随时调整')
    onReady(true)
  }

  if (isLoading || !draft) {
    return (
      <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" /> 加载默认权限…
      </div>
    )
  }

  return (
    // 说明 + 操作按钮固定，仅权限列表内部滚动（TF-043）
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 items-start gap-2 rounded-xl border border-divider bg-muted/50 p-3">
        <ShieldCheck className="mt-0.5 size-4 shrink-0 text-primary-600" />
        <p className="text-xs text-muted-foreground">
          控制 AI 代理（Agent）可访问的项目能力。默认仅只读；勾选后授予写操作。
        </p>
      </div>

      {/* 权限列表（仅此处内部滚动） */}
      <ul className="mt-3 flex min-h-0 flex-1 flex-col gap-1.5 overflow-y-auto pr-1">
        {FOCUS_KEYS.map((key) => (
          <li
            key={key}
            className="flex items-center justify-between rounded-lg border border-divider px-3 py-2.5"
          >
            <span className="text-sm">{ACTION_LABELS[key]}</span>
            <Switch
              checked={draft[key]}
              onCheckedChange={() => toggle(key)}
              aria-label={ACTION_LABELS[key]}
            />
          </li>
        ))}
      </ul>

      {/* 操作按钮（固定底部） */}
      <div className="mt-3 flex shrink-0 items-center justify-between border-t border-divider pt-3">
        <Button variant="ghost" size="sm" onClick={skip}>
          保留默认并继续
        </Button>
        <Button size="sm" onClick={doSave} disabled={save.isPending || saved}>
          {save.isPending ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : (
            <CheckCircle2 className="size-3.5" />
          )}
          {saved ? '已保存' : '保存权限'}
        </Button>
      </div>
    </div>
  )
}
