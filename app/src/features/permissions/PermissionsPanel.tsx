import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { Loader2, ShieldCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import { SwitchState } from '@/components/ui/switch'
import { usePermissions, useSavePermissions } from '@/hooks/usePermissions'
import {
  ACTION_KEYS,
  ACTION_DOMAIN_LABELS,
  ACTION_LABELS,
  type PermissionMap,
} from '@/types/models'

/**
 * 权限管理（TF-028 / TF-036 中文化）：
 * 17 action 中文 label + 轨道内嵌「允许/拒绝」文字的开关；
 * 布局固定顶部（标题）与底部（保存），仅权限列表内部滚动。
 */
export function PermissionsPanel() {
  const { data, isLoading, isError } = usePermissions()
  const savePermissions = useSavePermissions()
  const [draft, setDraft] = useState<PermissionMap | null>(null)

  const current: PermissionMap | null = draft ?? data?.actions ?? null

  // 按域分组展示（task.* / import.* 等），域标题中文化（TF-036）。
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
    // 固定顶/底：标题与保存按钮常驻，仅列表区域内部滚动
    <div className="flex h-full min-h-0 flex-col">
      {/* 顶部：标题行（固定） */}
      <div className="shrink-0 pb-3">
        <div className="flex items-center gap-2">
          <ShieldCheck className="size-4 text-primary-600" />
          <h3 className="text-sm font-semibold">Agent 权限</h3>
          <span className="text-caption text-muted-foreground">
            CLI / MCP / 远程 Agent 的操作范围；UI 不受限
          </span>
        </div>
      </div>

      {/* 中部：权限列表（内部滚动） */}
      <div className="min-h-0 flex-1 space-y-5 overflow-y-auto pr-1">
        {groups.map(([domain, keys]) => (
          <div key={domain}>
            <Separator className="mb-3" />
            <div className="mb-2 text-label uppercase tracking-wider text-muted-foreground">
              {ACTION_DOMAIN_LABELS[domain] ?? domain}
            </div>
            <div className="grid gap-2 sm:grid-cols-2">
              {keys.map((key) => {
                const actionKey = key as keyof PermissionMap
                const checked = current[actionKey]
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
                    <SwitchState
                      checked={checked}
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

      {/* 底部：保存按钮（固定） */}
      <div className="shrink-0 border-t border-divider pt-4">
        <div className="flex items-center justify-end gap-2">
          {dirty && (
            <span className="text-caption text-muted-foreground">
              {Object.values(draft ?? {}).filter(Boolean).length} 项允许
            </span>
          )}
          <Button onClick={save} disabled={!dirty || savePermissions.isPending}>
            {savePermissions.isPending && <Loader2 className="size-4 animate-spin" />}
            {dirty ? '保存权限' : '已保存'}
          </Button>
        </div>
      </div>
    </div>
  )
}
