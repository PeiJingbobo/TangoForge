import { create } from 'zustand'
import { persist } from 'zustand/middleware'

/** 项目内二级功能 tab（与 ProjectPanel PROJECT_TABS 保持一致） */
export const PROJECT_SECTIONS = [
  'kanban',
  'nav',
  'graph',
  'io',
  'permissions',
  'skills',
  'audit',
  'settings',
] as const

export type ProjectSection = (typeof PROJECT_SECTIONS)[number]

export function isProjectSection(s: string): s is ProjectSection {
  return (PROJECT_SECTIONS as readonly string[]).includes(s)
}

/**
 * 当前项目（workdir）全局状态。
 * 项目以工作目录为唯一标识（AGENTS.md §1），跨端显式传递；
 * project 持久化到 localStorage（重启恢复会话：上次项目 + 上次二级页 lastSection）。
 */
interface ProjectState {
  project: string | null
  /** 上次停留的项目内二级页（会话恢复用） */
  lastSection: ProjectSection
  setProject: (project: string | null) => void
  setLastSection: (section: ProjectSection) => void
}

export const useProjectStore = create<ProjectState>()(
  persist(
    (set) => ({
      project: null,
      lastSection: 'kanban',
      setProject: (project) => set({ project }),
      setLastSection: (lastSection) => set({ lastSection }),
    }),
    { name: 'tangoforge:project' },
  ),
)
