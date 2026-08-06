import { Link, NavLink, Outlet, useNavigate } from 'react-router'
import {
  Boxes,
  FolderKanban,
  GitBranch,
  LayoutGrid,
  Map,
  Moon,
  Settings,
  ShieldCheck,
  Sun,
  Upload,
  BookOpen,
  ScrollText,
  Loader2,
} from 'lucide-react'
import { useThemeMode } from '@/hooks/useThemeMode'
import { useProjects } from '@/hooks/useProjects'
import { useDaemonStatus } from '@/hooks/useDaemonStatus'
import { useProjectStore } from '@/stores/project'
import { cn } from '@/lib/utils'

/**
 * 应用布局（TF-029 布局重构）：
 * 左侧全局导航（项目概览 / 项目列表 flex-1 可滚动 / 底部主题+设置+守护进程状态）；
 * 右侧：激活项目后顶部显示项目二级功能 tab（看板/导航/全景图/导入导出/权限/Skills/审计）+ 内容区。
 */
const PROJECT_TABS = [
  { to: 'kanban', label: '看板', icon: LayoutGrid },
  { to: 'nav', label: '导航', icon: GitBranch },
  { to: 'graph', label: '全景图', icon: Map },
  { to: 'io', label: '导入导出', icon: Upload },
  { to: 'permissions', label: '权限', icon: ShieldCheck },
  { to: 'skills', label: 'Skills', icon: BookOpen },
  { to: 'audit', label: '审计', icon: ScrollText },
]

export function AppLayout() {
  const project = useProjectStore((s) => s.project)
  const setProject = useProjectStore((s) => s.setProject)
  const navigate = useNavigate()
  const { data: projects, isLoading } = useProjects()
  const daemonUp = useDaemonStatus()
  const { mode, setMode } = useThemeMode()

  const encoded = project ? encodeURIComponent(project) : ''

  const activateProject = (workdir: string) => {
    setProject(workdir)
    navigate(`/project/${encodeURIComponent(workdir)}/kanban`)
  }

  return (
    <div className="flex min-h-screen">
      {/* 左侧全局导航栏 */}
      <aside className="sticky top-0 flex h-screen w-60 shrink-0 flex-col border-r border-divider bg-card">
        {/* 品牌 + 顶部一级菜单 */}
        <div className="flex items-center gap-2.5 px-4 pb-3 pt-4">
          <span className="grid size-7 place-items-center rounded-lg bg-primary text-sm font-extrabold text-primary-foreground">
            T
          </span>
          <span className="text-base font-bold tracking-tight">TangoForge</span>
        </div>
        <nav className="px-2.5 pb-2">
          <NavLink
            to="/"
            className={({ isActive }) =>
              cn(
                'flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition-colors',
                isActive
                  ? 'bg-primary-50 font-semibold text-primary-700'
                  : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
              )
            }
          >
            <Boxes className="size-4" />
            项目概览
          </NavLink>
        </nav>
        <div className="border-t border-divider" />

        {/* 中部：项目列表（flex-1 占满剩余高度，内部滚动） */}
        <div className="flex min-h-0 flex-1 flex-col">
          <div className="flex items-center justify-between px-4 pb-1.5 pt-3">
            <span className="text-label uppercase tracking-wider text-muted-foreground">项目</span>
            {isLoading && <Loader2 className="size-3 animate-spin text-muted-foreground" />}
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto px-2.5 pb-2">
            {!isLoading && (!projects || projects.length === 0) && (
              <p className="px-3 py-2 text-xs text-muted-foreground">
                暂无项目，到「项目概览」导入工作目录。
              </p>
            )}
            {projects?.map((p) => (
              <button
                key={p.id}
                type="button"
                onClick={() => activateProject(p.workdir)}
                className={cn(
                  'group flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors',
                  project === p.workdir
                    ? 'bg-primary-50 font-semibold text-primary-700'
                    : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
                )}
              >
                <FolderKanban className="size-4 shrink-0" />
                <span className="truncate">{p.name}</span>
              </button>
            ))}
          </div>
        </div>

        {/* 底部：主题切换 + 设置 + 守护进程状态 */}
        <div className="border-t border-divider p-2.5">
          <button
            type="button"
            onClick={() => setMode(mode === 'dark' ? 'light' : 'dark')}
            className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
            aria-label="切换亮暗色"
          >
            {mode === 'dark' ? <Sun className="size-4" /> : <Moon className="size-4" />}
            亮色 / 暗色
          </button>
          <NavLink
            to="/settings"
            className={({ isActive }) =>
              cn(
                'flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition-colors',
                isActive
                  ? 'bg-primary-50 font-semibold text-primary-700'
                  : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
              )
            }
          >
            <Settings className="size-4" />
            设置
          </NavLink>
          <div className="mt-1 flex items-center gap-2.5 rounded-lg px-3 py-1.5">
            <span
              aria-hidden
              className={cn(
                'size-2 rounded-full',
                daemonUp
                  ? 'bg-success shadow-[0_0_0_3px_var(--success-soft)]'
                  : 'bg-muted-foreground/40',
              )}
            />
            <span className="text-xs text-muted-foreground">
              {daemonUp ? '守护进程运行中' : '守护进程未连接'}
            </span>
          </div>
        </div>
      </aside>

      {/* 右侧：二级 tab + 内容 */}
      <div className="flex min-w-0 flex-1 flex-col">
        {project && (
          <div className="sticky top-0 z-30 flex items-center gap-1 overflow-x-auto border-b border-divider bg-background/85 px-6 py-2 backdrop-blur-md">
            <span className="mr-2 shrink-0 max-w-40 truncate text-xs text-muted-foreground">
              {project.split(/[\\/]/).pop()}
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
            <Link
              to="/"
              className="ml-auto shrink-0 rounded-full px-2 py-1 text-xs text-muted-foreground hover:text-accent-foreground"
            >
              返回概览
            </Link>
          </div>
        )}
        <main className="mx-auto w-full max-w-[1160px] flex-1 px-6 py-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
