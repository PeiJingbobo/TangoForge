import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PermissionsPanel } from '@/features/permissions/PermissionsPanel'
import { SkillsPanel } from '@/features/skills/SkillsPanel'

/**
 * 设置页（TF-028 落地）：权限管理（仅 UI）+ Skill 浏览。
 * 主题偏好持久化入口随 TF-028 一并具备（ThemeToggle 后续迁移至此）。
 */
export function SettingsPage() {
  return (
    <div className="mx-auto max-w-3xl">
      <p className="text-caption uppercase tracking-[0.09em] text-muted-foreground">全局设置</p>
      <h1 className="text-h2 text-foreground">设置</h1>

      <Tabs defaultValue="permissions" className="mt-6">
        <TabsList>
          <TabsTrigger value="permissions">权限</TabsTrigger>
          <TabsTrigger value="skills">Skills</TabsTrigger>
        </TabsList>
        <TabsContent value="permissions" className="pt-6">
          <PermissionsPanel />
        </TabsContent>
        <TabsContent value="skills" className="pt-6">
          <SkillsPanel />
        </TabsContent>
      </Tabs>
    </div>
  )
}
