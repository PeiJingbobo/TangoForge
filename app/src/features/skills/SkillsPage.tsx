import { SkillsPanel } from '@/features/skills/SkillsPanel'

/** Skills（TF-033 重设计）：技能库 + 宿主安装矩阵 + AGENTS.md 提示词复制。 */
export function SkillsPage() {
  return (
    <div className="mx-auto max-w-4xl">
      <h1 className="text-h2 text-foreground">Skills</h1>
      <p className="mt-1 text-caption text-muted-foreground">
        为指定 Agent 工具安装技能包、检查安装状态；系统提供内置技能库与自定义编辑。
      </p>
      <div className="mt-6">
        <SkillsPanel />
      </div>
    </div>
  )
}
