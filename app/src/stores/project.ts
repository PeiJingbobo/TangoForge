import { create } from 'zustand'
import { persist } from 'zustand/middleware'

/**
 * 当前项目（workdir）全局状态。
 * 项目以工作目录为唯一标识（AGENTS.md §1），跨端显式传递；
 * 前端默认项目持久化到 localStorage，切换后重启恢复。
 */
interface ProjectState {
  project: string | null
  setProject: (project: string | null) => void
}

export const useProjectStore = create<ProjectState>()(
  persist(
    (set) => ({
      project: null,
      setProject: (project) => set({ project }),
    }),
    { name: 'tangoforge:project' },
  ),
)
