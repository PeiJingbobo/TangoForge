import { useEffect } from 'react'
import { NavLink, Outlet, useLocation, useParams } from 'react-router'
import {
  LayoutGrid,
  GitBranch,
  Map,
  Upload,
  ShieldCheck,
  BookOpen,
  ScrollText,
  Library,
  Settings,
} from 'lucide-react'
import { isProjectSection, useProjectStore } from '@/stores/project'
import { cn } from '@/lib/utils'

/**
 * 项目面板（TF-029 布局重构）：
 * 以项目为单位的功能（看板/导航/全景图/导入导出/权限/Skills/审计/设置/任务详情）
 * 全部封装在本组件内；仅当路由位于 /project/:projectId/* 时渲染（一级菜单下不出现）。
 * URL 中的 projectId 即当前项目（进入时同步全局 store，供 API 层读取）。
 */
const PROJECT_TABS = [
  { to: 'kanban', label: '看板', icon: LayoutGrid },
  { to: 'nav', label: '导航', icon: GitBranch },
  { to: 'graph', label: '全景图', icon: Map },
  { to: 'io', label: '导入导出', icon: Upload },
  { to: 'permissions', label: '权限', icon: ShieldCheck },
  { to: 'skills', label: 'Skills', icon: BookOpen },
  { to: 'audit', label: '审计', icon: ScrollText },
  { to: 'knowledge', label: '知识库', icon: Library },
  { to: 'settings', label: '设置', icon: Settings },
]

export function ProjectPanel() {
  const { projectId } = useParams()
  const location = useLocation()
  const setProject = useProjectStore((s) => s.setProject)
  const setLastSection = useProjectStore((s) => s.setLastSection)

  // URL 即项目标识：进入项目路由时同步全局 store（侧边栏高亮/API 上下文一致）
  useEffect(() => {
    if (projectId) setProject(projectId)
  }, [projectId, setProject])

  // 会话恢复：记录当前二级页（任务详情等非 tab 页不覆盖，保持上次 tab）
  useEffect(() => {
    const seg = location.pathname.split('/').filter(Boolean).pop() ?? ''
    if (isProjectSection(seg)) setLastSection(seg)
  }, [location.pathname, setLastSection])

  const encoded = encodeURIComponent(projectId ?? '')

  return (
    // h-full：撑满 main 滚动容器可视高（看板页内部分层时列内滚动；其他页面超高由 main 滚）
    <div className="flex h-full flex-col">
      {/* 二级功能 tab（仅项目路由渲染） */}
      <div className="sticky top-0 z-30 flex items-center gap-1 overflow-x-auto border-b border-divider bg-background/85 px-6 py-2 backdrop-blur-md">
        <span
          className="mr-2 max-w-44 shrink-0 truncate font-mono text-xs text-muted-foreground"
          title={projectId}
        >
          {projectId?.split(/[\\/]/).pop()}
        </span>
        {PROJECT_TABS.map(({ to, label, icon: Icon }) => (
          <NavLink
            key={to}
            to={`/project/${encoded}/${to}`}
            className={({ isActive }) =>
              cn(
                'flex shrink-0 items-center gap-1.5 rounded-full px-3 py-1.5 text-sm transition-colors',
                isActive
                  ? 'bg-primary-50 font-semibold text-primary-700'
                  : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
              )
            }
          >
            <Icon className="size-3.5" />
            {label}
          </NavLink>
        ))}
      </div>

      {/* 内容区：flex-1 撑满剩余高度（看板页 h-full 内部分层；其他页面超高溢出到 main 滚） */}
      <div className="mx-auto w-full max-w-[1160px] min-h-0 flex-1 px-6 py-6">
        <Outlet />
      </div>
    </div>
  )
}
