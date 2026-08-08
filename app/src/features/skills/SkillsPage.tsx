import { SkillsPanel } from '@/features/skills/SkillsPanel'

/**
 * Skills（TF-033 重设计；TF-042 滚动布局优化）：技能库 + 宿主安装矩阵。
 * 参考 Agent 权限页布局：标题固定顶部，仅主体面板内部滚动，四周留细微间距。
 */
export function SkillsPage() {
  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* 顶部标题（固定） */}
      <div className="mx-auto w-full max-w-4xl shrink-0">
        <h1 className="text-h2 text-foreground">Skills</h1>
        <p className="mt-1 text-caption text-muted-foreground">
          为指定 Agent 工具安装技能包、检查安装状态；系统提供内置技能库与自定义编辑。
        </p>
      </div>
      {/* 主体面板占满剩余高度（内部滚动，四周留细微间距） */}
      <div className="mx-auto mt-5 w-full max-w-4xl min-h-0 flex-1">
        <SkillsPanel />
      </div>
    </div>
  )
}
