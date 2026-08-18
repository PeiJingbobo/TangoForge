import { useCallback, useState } from 'react'

export type GraphViewMode = 'pert' | 'force'

const STORAGE_KEY = 'tangoforge.graph.view'

/**
 * 全景图渲染模式 hook（TF-055）：PERT（默认）/ 力导向（兜底），偏好持久化 localStorage。
 */
export function useGraphViewMode(): [GraphViewMode, (m: GraphViewMode) => void] {
  const [mode, setMode] = useState<GraphViewMode>(() => {
    try {
      return localStorage.getItem(STORAGE_KEY) === 'force' ? 'force' : 'pert'
    } catch {
      return 'pert'
    }
  })
  const set = useCallback((m: GraphViewMode) => {
    setMode(m)
    try {
      localStorage.setItem(STORAGE_KEY, m)
    } catch {
      // localStorage 不可用时仅内存态
    }
  }, [])
  return [mode, set]
}
