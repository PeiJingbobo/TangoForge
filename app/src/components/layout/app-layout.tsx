import { Link, Outlet } from 'react-router'
import { ThemeToggle } from '@/components/theme/theme-toggle'

/**
 * 应用布局（TF-022 骨架版）：
 * 顶部品牌栏 + 导航占位 + 主题切换；正式侧栏/工作区布局随 TF-024 项目界面落地。
 */
const NAV_LINKS = [
  { to: '/', label: '工作区' },
  { to: '/project/demo/kanban', label: '看板（占位）' },
  { to: '/settings', label: '设置' },
]

export function AppLayout() {
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
            <nav className="hidden items-center gap-1 sm:flex">
              {NAV_LINKS.map((l) => (
                <Link
                  key={l.to}
                  to={l.to}
                  className="rounded-full px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
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
