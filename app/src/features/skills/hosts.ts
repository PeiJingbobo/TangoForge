/**
 * Agent 宿主矩阵（TF-042：全部目录型 .xxx/skills，无单文件 .md 宿主）。
 * 与后端 internal/skill/hosts.go 的 Hosts 矩阵一一对齐——SkillsPanel（项目 Skills 配置页）
 * 与 SkillStep（项目导入引导）必须共用此常量，保证两端选择项完全一致。
 */
export const SKILL_HOSTS: { key: string; label: string; scope: 'project' | 'user' }[] = [
  { key: '.claude/skills', label: '.claude/skills（Claude Code）', scope: 'project' },
  { key: '.cursor/skills', label: '.cursor/skills（Cursor）', scope: 'project' },
  { key: '.github/skills', label: '.github/skills（GitHub Copilot）', scope: 'project' },
  { key: 'user-claude', label: '~/.claude/skills（Claude 全局）', scope: 'user' },
  { key: 'user-codebuddy', label: '~/.workbuddy/skills（WorkBuddy 全局）', scope: 'user' },
]

/** 默认选中宿主（首个项目级目录型宿主）。 */
export const DEFAULT_SKILL_HOST = SKILL_HOSTS[0].key
