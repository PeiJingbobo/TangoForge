import { Badge } from '@/components/ui/badge'

/**
 * 全局设置 —— TF-028 实现（权限管理、Skill、主题设置正式化）。
 * 骨架占位；主题切换临时入口当前位于顶栏 ThemeToggle。
 */
export function SettingsPage() {
  return (
    <div>
      <p className="text-caption uppercase tracking-[0.09em] text-muted-foreground">全局设置</p>
      <h1 className="text-h2 text-foreground">设置</h1>
      <div className="mt-6 rounded-[14px] border border-dashed border-border p-12 text-center text-body text-muted-foreground">
        设置页骨架占位：权限管理、Skill 列表、主题偏好持久化将在 TF-028 实现。
        <div className="mt-4">
          <Badge variant="outline">TF-028 实现</Badge>
        </div>
      </div>
    </div>
  )
}
