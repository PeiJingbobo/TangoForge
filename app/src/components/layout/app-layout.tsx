import { useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router'
import TangoForgeLockup from '@/assets/TangoForge-lockup-transparent.png'
import {
  Boxes,
  FolderKanban,
  FolderOpen,
  Loader2,
  Moon,
  Pencil,
  Settings,
  Sun,
  Trash2,
} from 'lucide-react'
import { toast } from 'sonner'
import { useThemeMode } from '@/hooks/useThemeMode'
import { useProjects, useRemoveProject, useRenameProject } from '@/hooks/useProjects'
import { useDaemonStatus } from '@/hooks/useDaemonStatus'
import { useProjectStore } from '@/stores/project'
import { GlobalTaskDrawer } from '@/features/tasks/TaskDetail'
import { WindowTitleBar } from '@/components/layout/window-titlebar'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import { cn } from '@/lib/utils'
import type { Project } from '@/types/models'

/**
 * 应用布局（TF-029 布局重构）：
 * 左侧全局导航（顶部项目概览 / 中部项目列表 flex-1 可滚动 / 底部一行：亮暗切换图标
 * + 设置图标 + 守护进程指示点）；右侧为内容区（项目内页面由 ProjectPanel 提供二级 tab）。
 * TF-035：项目列表项右键菜单（重命名 / 在文件夹中打开 / 删除项目）。
 */
export function AppLayout() {
  const project = useProjectStore((s) => s.project)
  const setProject = useProjectStore((s) => s.setProject)
  const navigate = useNavigate()
  const { data: projects, isLoading } = useProjects()
  const daemonUp = useDaemonStatus()
  const { mode, setMode } = useThemeMode()

  const [renameTarget, setRenameTarget] = useState<Project | null>(null)
  const [removeTarget, setRemoveTarget] = useState<Project | null>(null)

  const activateProject = (workdir: string) => {
    setProject(workdir)
    navigate(`/project/${encodeURIComponent(workdir)}/kanban`)
  }

  return (
    // h-screen + overflow-hidden：页面级不出现滚动条；滚动收敛到内容区/看板列内部
    <div className="flex h-screen flex-col overflow-hidden">
      {/* 自绘标题栏（TF-038）：mac 交通灯留白 / win 右侧控制按钮；Web 预览不渲染 */}
      <WindowTitleBar />

      {/* 主体：左侧导航 + 右侧内容（标题栏之下撑满剩余高度） */}
      <div className="flex min-h-0 flex-1">
        {/* 左侧全局导航栏 */}
        <aside className="sticky top-0 flex h-full w-60 shrink-0 flex-col border-r border-divider bg-card">
          {/* 品牌（应用 lockup 图） */}
          <div className="px-4 pb-3 pt-4">
            <img
              src={TangoForgeLockup}
              alt="TangoForge"
              className="h-8 w-auto select-none"
              draggable={false}
            />
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
              <span className="text-label uppercase tracking-wider text-muted-foreground">
                项目
              </span>
              {isLoading && (
                <span className="size-3 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-muted-foreground" />
              )}
            </div>
            <div className="min-h-0 flex-1 space-y-1 overflow-y-auto px-2.5 pb-2">
              {!isLoading && (!projects || projects.length === 0) && (
                <p className="px-3 py-2 text-xs text-muted-foreground">
                  暂无项目，到「项目概览」导入工作目录。
                </p>
              )}
              {projects?.map((p) => (
                <ProjectItem
                  key={p.id}
                  project={p}
                  active={project === p.workdir}
                  onActivate={() => activateProject(p.workdir)}
                  onRename={() => setRenameTarget(p)}
                  onReveal={() => {
                    const shell = window.tangoforge?.shell
                    if (!shell) {
                      toast.error('「在文件夹中打开」仅桌面版可用（Web 预览不支持）')
                      return
                    }
                    void shell.revealPath(p.workdir).then((ok) => {
                      if (ok) toast.success(`已在文件夹中打开：${p.workdir}`)
                      else toast.error('打开文件夹失败')
                    })
                  }}
                  onRemove={() => setRemoveTarget(p)}
                />
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

        {/* 右侧内容区：固定视口内，内容超高时由 main 内部滚动（页面级不滚动） */}
        <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <main className="mx-auto w-full max-w-[1160px] min-h-0 flex-1 overflow-y-auto px-6 py-6">
            <Outlet />
          </main>
        </div>
      </div>

      {/* 重命名对话框 */}
      <RenameProjectDialog target={renameTarget} onClose={() => setRenameTarget(null)} />

      {/* 删除项目确认对话框 */}
      <RemoveProjectDialog target={removeTarget} onClose={() => setRemoveTarget(null)} />

      {/* 全局任务详情抽屉（当前页保留，抽屉浮层覆盖） */}
      <GlobalTaskDrawer />
    </div>
  )
}

/* ---------- 项目列表项（TF-035 右键菜单） ---------- */

interface ProjectItemProps {
  project: Project
  active: boolean
  onActivate: () => void
  onRename: () => void
  onReveal: () => void
  onRemove: () => void
}

function ProjectItem({
  project,
  active,
  onActivate,
  onRename,
  onReveal,
  onRemove,
}: ProjectItemProps) {
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <button
          type="button"
          onClick={onActivate}
          className={cn(
            'group flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors',
            active
              ? 'bg-primary-50 font-semibold text-primary-700'
              : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
          )}
        >
          <FolderKanban className="size-4 shrink-0" />
          <span className="truncate">{project.name}</span>
        </button>
      </ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem onClick={onRename}>
          <Pencil className="size-4" />
          重命名
        </ContextMenuItem>
        <ContextMenuItem onClick={onReveal}>
          <FolderOpen className="size-4" />
          在文件夹中打开
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem
          onClick={onRemove}
          className="text-destructive-ink focus:bg-destructive-soft"
        >
          <Trash2 className="size-4" />
          删除项目
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  )
}

/* ---------- 重命名对话框 ---------- */

function RenameProjectDialog({ target, onClose }: { target: Project | null; onClose: () => void }) {
  const rename = useRenameProject()
  const [name, setName] = useState('')

  // target 变化时同步输入框初值。
  const [prevTarget, setPrevTarget] = useState<number | null>(null)
  if (target && target.id !== prevTarget) {
    setPrevTarget(target.id)
    setName(target.name)
  }

  const submit = () => {
    if (!target || !name.trim()) return
    rename.mutate(
      { id: target.id, name: name.trim() },
      {
        onSuccess: () => {
          toast.success(`项目已重命名为「${name.trim()}」`)
          onClose()
        },
        onError: (e) => toast.error(e instanceof Error ? e.message : '重命名失败'),
      },
    )
  }

  return (
    <Dialog
      open={target !== null}
      onOpenChange={(open) => {
        if (!open && !rename.isPending) onClose()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>重命名项目</DialogTitle>
          <DialogDescription>
            仅修改显示名称（注册表记录），不影响磁盘目录与任务数据。
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="rename-input">项目名称</Label>
          <Input
            id="rename-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') submit()
            }}
            placeholder={target?.name ?? ''}
            autoFocus
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={rename.isPending}>
            取消
          </Button>
          <Button onClick={submit} disabled={!name.trim() || rename.isPending}>
            {rename.isPending && <Loader2 className="size-4 animate-spin" />}
            保存
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/* ---------- 删除项目确认对话框 ---------- */

function RemoveProjectDialog({ target, onClose }: { target: Project | null; onClose: () => void }) {
  const remove = useRemoveProject()
  const setProject = useProjectStore((s) => s.setProject)
  const navigate = useNavigate()

  const submit = () => {
    if (!target) return
    remove.mutate(target.id, {
      onSuccess: () => {
        toast.success(`已移除项目「${target.name}」（磁盘数据保留）`)
        // 若删除的是当前选中项目：取消选中并重定向到项目概览页。
        const current = useProjectStore.getState().project
        if (current === target.workdir) {
          setProject(null)
          navigate('/')
        }
        onClose()
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : '删除失败'),
    })
  }

  return (
    <Dialog
      open={target !== null}
      onOpenChange={(open) => {
        if (!open && !remove.isPending) onClose()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>删除项目「{target?.name}」？</DialogTitle>
          <DialogDescription>
            仅移除项目注册记录，
            <span className="font-semibold text-foreground">不会删除磁盘上的任何数据</span>
            （目录、.taskboard/ 元数据与任务均保留）。可稍后重新导入。
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={remove.isPending}>
            取消
          </Button>
          <Button variant="destructive" onClick={submit} disabled={remove.isPending}>
            {remove.isPending && <Loader2 className="size-4 animate-spin" />}
            确认删除
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
