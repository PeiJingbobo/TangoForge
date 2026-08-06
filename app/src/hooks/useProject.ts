import { useProjectStore } from '@/stores/project'

/**
 * 解析当前项目 workdir：显式参数优先，否则取全局 store。
 * 返回 undefined 时 hooks 应禁用请求（无项目上下文）。
 */
export function useProjectId(project?: string): string | undefined {
  const stored = useProjectStore((s) => s.project)
  return project ?? stored ?? undefined
}

/** 切换当前项目（全局 store 持久化） */
export function useSetProject(): (project: string | null) => void {
  return useProjectStore((s) => s.setProject)
}

/** 只读读取当前项目（组件外/事件回调场景） */
export function getProjectId(): string | null {
  return useProjectStore.getState().project
}
