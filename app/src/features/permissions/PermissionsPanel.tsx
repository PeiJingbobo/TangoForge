import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { Loader2, ShieldCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import { usePermissions, useSavePermissions } from '@/hooks/usePermissions'
import { ACTION_KEYS, type PermissionMap } from '@/types/models'

/** 权限管理（TF-028）：仅 UI 可写（后端已双重校验）。16 action 勾选 + 全量覆盖保存。 */
export function PermissionsPanel() {
  const { data, isLoading, isError } = usePermissions()
  const savePermissions = useSavePermissions()
  const [draft, setDraft] = useState<PermissionMap | null>(null)

  const current: PermissionMap | null = draft ?? data?.actions ?? null

  // 按域分组展示（task.* / import.* 等）
  const groups = useMemo(() => {
    const byDomain = new Map<string, string[]>()
    for (const key of ACTION_KEYS) {
      const domain = key.split('.')[0]
      byDomain.set(domain, [...(byDomain.get(domain) ?? []), key])
    }
    return [...byDomain.entries()]
  }, [])

  if (isLoading) return <Skeleton className="h-40 w-full" />
  if (isError || !current) {
    return <p className="text-sm text-muted-foreground">权限数据加载失败。</p>
  }

  const toggle = (key: keyof PermissionMap) => {
    setDraft((prev) => {
      const base = prev ?? data?.actions
      if (!base) return prev
      return { ...base, [key]: !base[key] }
    })
  }

  const dirty = draft !== null && JSON.stringify(draft) !== JSON.stringify(data?.actions)

  const save = () => {
    if (!draft) return
    savePermissions.mutate(draft, {
      onSuccess: () => {
        toast.success('权限已保存（全量覆盖）')
        setDraft(null)
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : '保存失败'),
    })
  }

  return (
    <div>
      <div className="mb-3 flex items-center gap-2">
        <ShieldCheck className="size-4 text-primary-600" />
        <h3 className="text-sm font-semibold">Agent 权限</h3>
        <span className="text-caption text-muted-foreground">
          CLI / MCP / 远程 Agent 的操作范围；UI 不受限
        </span>
      </div>
      <div className="space-y-5">
        {groups.map(([domain, keys]) => (
          <div key={domain}>
            <Separator className="mb-3" />
            <div className="mb-2 text-label uppercase tracking-wider text-muted-foreground">
              {domain}
            </div>
            <div className="grid gap-2 sm:grid-cols-2">
              {keys.map((key) => {
                const actionKey = key as keyof PermissionMap
                return (
                  <label
                    key={key}
                    className="flex cursor-pointer items-center gap-2.5 rounded-lg border border-divider px-3 py-2 text-sm transition-colors hover:border-primary-300"
                  >
                    <input
                      type="checkbox"
                      checked={current[actionKey]}
                      onChange={() => toggle(actionKey)}
                      aria-label={`权限 ${key}`}
                      className="size-4 accent-[var(--primary-500)]"
                    />
                    <code className="font-mono text-xs">{key}</code>
                    <span
                      className={`ml-auto rounded-full px-2 py-0.5 text-[10px] font-semibold ${
                        current[actionKey]
                          ? 'bg-success-soft text-success-ink'
                          : 'bg-muted text-muted-foreground'
                      }`}
                    >
                      {current[actionKey] ? '允许' : '拒绝'}
                    </span>
                  </label>
                )
              })}
            </div>
          </div>
        ))}
      </div>
      <div className="mt-5 flex justify-end">
        <Button onClick={save} disabled={!dirty || savePermissions.isPending}>
          {savePermissions.isPending && <Loader2 className="size-4 animate-spin" />}
          {dirty ? '保存权限' : '已保存'}
        </Button>
      </div>
    </div>
  )
}
