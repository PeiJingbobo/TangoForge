import { PermissionsPanel } from '@/features/permissions/PermissionsPanel'

/** Agent 权限（TF-029 项目二级 tab）：仅 UI 可改（接口层已双重校验）。 */
export function PermissionsPage() {
  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="text-h2 text-foreground">Agent 权限</h1>
      <p className="mt-1 text-caption text-muted-foreground">
        CLI / MCP / 远程 Agent 在当前项目的操作范围；UI 不受限。
      </p>
      <div className="mt-6">
        <PermissionsPanel />
      </div>
    </div>
  )
}
