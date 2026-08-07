import { PermissionsPanel } from '@/features/permissions/PermissionsPanel'

/**
 * Agent 权限（TF-029 项目二级 tab / TF-036 中文化 + 滚动布局）：
 * 仅 UI 可改（接口层已双重校验）。页面标题固定顶部，权限列表内部滚动。
 */
export function PermissionsPage() {
  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* 顶部标题（固定） */}
      <div className="shrink-0">
        <h1 className="text-h2 text-foreground">Agent 权限</h1>
        <p className="mt-1 text-caption text-muted-foreground">
          CLI / MCP / 远程 Agent 在当前项目的操作范围；UI 不受限。
        </p>
      </div>
      {/* 面板占满剩余高度（内部：标题/列表滚动/保存按钮固定） */}
      <div className="mt-5 min-h-0 flex-1">
        <PermissionsPanel />
      </div>
    </div>
  )
}
