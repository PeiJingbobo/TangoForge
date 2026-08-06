/** TanStack Query 键（统一管理，避免字符串散落） */
export const qk = {
  projects: ['projects'] as const,
  tasks: (project: string) => ['tasks', project] as const,
  task: (project: string, id: string) => ['tasks', project, id] as const,
  graph: (project: string) => ['graph', project] as const,
  stateMachine: (project: string) => ['state-machine', project] as const,
  drafts: (project: string) => ['drafts', project] as const,
  permissions: (project: string) => ['permissions', project] as const,
  skills: (project: string) => ['skills', project] as const,
  audit: (project: string) => ['audit', project] as const,
}
