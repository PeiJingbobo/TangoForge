import { SkillsPanel } from '@/features/skills/SkillsPanel'

/** Skills 浏览（TF-029 项目二级 tab）：列表 + instructions 详情。 */
export function SkillsPage() {
  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="text-h2 text-foreground">Skills</h1>
      <p className="mt-1 text-caption text-muted-foreground">
        项目 .taskboard/skills/ 下被扫描到的 Agent 技能包。
      </p>
      <div className="mt-6">
        <SkillsPanel />
      </div>
    </div>
  )
}
