import { NavLink, Outlet, useNavigate } from 'react-router'
import { Boxes, FolderKanban, Moon, Settings, Sun } from 'lucide-react'
import { useThemeMode } from '@/hooks/useThemeMode'
import { useProjects } from '@/hooks/useProjects'
import { useDaemonStatus } from '@/hooks/useDaemonStatus'
import { useProjectStore } from '@/stores/project'
import { cn } from '@/lib/utils'

/**
 * 应用布局（TF-029 布局重构）：
 * 左侧全局导航（顶部项目概览 / 中部项目列表 flex-1 可滚动 / 底部一行：亮暗切换图标
 * + 设置图标 + 守护进程指示点）；右侧为内容区（项目内页面由 ProjectPanel 提供二级 tab）。
 */
export function AppLayout() {
  const project = useProjectStore((s) => s.project)
  const setProject = useProjectStore((s) => s.setProject)
  const navigate = useNavigate()
  const { data: projects, isLoading } = useProjects()
  const daemonUp = useDaemonStatus()
  const { mode, setMode } = useThemeMode()

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
            {isLoading && (
              <span className="size-3 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-muted-foreground" />
            )}
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

        {/* 底部一行：亮暗切换（图标）+ 设置（图标）+ 守护进程指示点 */}
        <div className="border-t border-divider p-2.5">
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => setMode(mode === 'dark' ? 'light' : 'dark')}
              className="grid size-9 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
              aria-label="切换亮暗色"
              title="切换亮暗色"
            >
              {mode === 'dark' ? <Sun className="size-4" /> : <Moon className="size-4" />}
            </button>
            <NavLink
              to="/settings"
              className={({ isActive }) =>
                cn(
                  'grid size-9 place-items-center rounded-lg transition-colors',
                  isActive
                    ? 'bg-primary-50 text-primary-700'
                    : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
                )
              }
              aria-label="设置"
              title="设置"
            >
              <Settings className="size-4" />
            </NavLink>
            <span
              className={cn(
                'ml-auto mr-2 size-2.5 rounded-full',
                daemonUp ? 'bg-success' : 'bg-muted-foreground/40',
              )}
              role="status"
              aria-label={daemonUp ? '守护进程运行中' : '守护进程未连接'}
              title={daemonUp ? '守护进程运行中' : '守护进程未连接'}
            />
          </div>
        </div>
      </aside>

      {/* 右侧内容区 */}
      <div className="flex min-w-0 flex-1 flex-col">
        <main className="mx-auto w-full max-w-[1160px] flex-1 px-6 py-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
