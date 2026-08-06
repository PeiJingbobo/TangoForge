import { Link, Outlet } from 'react-router'
import { ThemeToggle } from '@/components/theme/theme-toggle'
import { useProjectStore } from '@/stores/project'
import { cn } from '@/lib/utils'

/**
 * 应用布局：顶部品牌栏 + 当前项目 + 导航 + 主题切换（docs/UI-VISION.md）。
 */
export function AppLayout() {
  const project = useProjectStore((s) => s.project)
  const encoded = project ? encodeURIComponent(project) : ''

  const navLinks = [
    { to: '/', label: '工作区' },
    ...(project
      ? [
          { to: `/project/${encoded}/kanban`, label: '看板' },
          { to: `/project/${encoded}/nav`, label: '导航' },
          { to: `/project/${encoded}/graph`, label: '全景图' },
        ]
      : []),
    { to: '/settings', label: '设置' },
  ]

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-40 border-b border-divider bg-background/85 backdrop-blur-md">
        <div className="mx-auto flex h-14 w-full max-w-[1160px] items-center justify-between px-6">
          <div className="flex items-center gap-6">
            <Link to="/" className="flex items-center gap-2.5">
              <span className="grid size-7 place-items-center rounded-lg bg-primary text-sm font-extrabold text-primary-foreground">
                T
              </span>
              <span className="text-base font-bold tracking-tight">TangoForge</span>
            </Link>
            {project && (
              <span className="hidden items-center gap-1.5 rounded-full bg-primary-50 px-3 py-1 text-xs font-medium text-primary-700 sm:flex">
                <span className="size-1.5 rounded-full bg-primary-500" />
                {project.split(/[\\/]/).pop()}
              </span>
            )}
            <nav className="hidden items-center gap-1 sm:flex">
              {navLinks.map((l) => (
                <Link
                  key={l.to}
                  to={l.to}
                  className={cn(
                    'rounded-full px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground',
                  )}
                >
                  {l.label}
                </Link>
              ))}
            </nav>
          </div>
          <ThemeToggle />
        </div>
      </header>
      <main className="mx-auto w-full max-w-[1160px] flex-1 px-6 py-8">
        <Outlet />
      </main>
    </div>
  )
}
