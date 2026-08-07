import { useEffect } from 'react'
import { useLocation, useNavigate } from 'react-router'
import { useProjectStore } from '@/stores/project'

/**
 * 启动会话恢复（TF 布局修复）：
 * 启动落在「项目概览」（/）时，若 localStorage 持久化存在上次项目（persist），
 * 直接恢复进入上次项目（上次二级页 lastSection），避免「项目概览 + 上次项目」双高亮。
 * 非概览路由（/settings、/project/...）不干预（刷新保持当前位置）。
 */
export function useSessionRestore(): void {
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    if (location.pathname !== '/') return
    const { project, lastSection } = useProjectStore.getState()
    if (!project) return
    navigate(`/project/${encodeURIComponent(project)}/${lastSection}`, { replace: true })
  }, [location.pathname, navigate])
}
