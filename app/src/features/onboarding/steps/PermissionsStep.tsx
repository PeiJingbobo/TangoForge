import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { CheckCircle2, Loader2, ShieldCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { usePermissions, useSavePermissions } from '@/hooks/usePermissions'
import {
  ACTION_KEYS,
  ACTION_DOMAIN_LABELS,
  ACTION_LABELS,
  type PermissionMap,
} from '@/types/models'

/**
 * 引导 Step 4：Agent 权限配置（与权限配置页 ACTION_KEYS 全量对齐）。
 * 默认只读；可勾选授予写操作。保存后 onReady(true)；也可保留默认直接继续。
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

  const groups = useMemo(() => {
    const byDomain = new Map<string, string[]>()
    for (const key of ACTION_KEYS) {
      const domain = key.split('.')[0]
      byDomain.set(domain, [...(byDomain.get(domain) ?? []), key])
    }
    return [...byDomain.entries()]
  }, [])

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

      {/* 权限列表（按域分组，仅此处内部滚动） */}
      <div className="mt-3 min-h-0 flex-1 space-y-5 overflow-y-auto pr-1">
        {groups.map(([domain, keys]) => (
          <div key={domain}>
            <Separator className="mb-3" />
            <div className="mb-2 text-label uppercase tracking-wider text-muted-foreground">
              {ACTION_DOMAIN_LABELS[domain] ?? domain}
            </div>
            <div className="grid gap-2 sm:grid-cols-2">
              {keys.map((key) => {
                const actionKey = key as keyof PermissionMap
                return (
                  <div
                    key={key}
                    className="flex items-center gap-3 rounded-xl border border-divider px-3 py-2.5 transition-colors hover:border-primary-300"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-medium text-foreground">
                        {ACTION_LABELS[actionKey]}
                      </div>
                      <div className="truncate font-mono text-[11px] text-muted-foreground">
                        {key}
                      </div>
                    </div>
                    <Switch
                      checked={draft[actionKey]}
                      onCheckedChange={() => toggle(actionKey)}
                      aria-label={`权限 ${key}`}
                    />
                  </div>
                )
              })}
            </div>
          </div>
        ))}
      </div>

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
