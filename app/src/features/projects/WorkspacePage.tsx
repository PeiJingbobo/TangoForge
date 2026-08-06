import { useState } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { FolderOpen, Loader2, Plus, Trash2 } from 'lucide-react'
import { useImportProject, useProjects, useRemoveProject } from '@/hooks/useProjects'
import { useSetProject } from '@/hooks/useProject'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import type { Project } from '@/types/models'

/**
 * 工作区：项目列表 + 导入引导（docs/UI-VISION.md 场景 A 行式分组）。
 * 导入 = 选目录 → POST /api/projects/import（未初始化目录自动初始化 .taskboard/）。
 */
export function WorkspacePage() {
  const { data: projects, isLoading, isError, refetch } = useProjects()
  const importProject = useImportProject()
  const removeProject = useRemoveProject()
  const setProject = useSetProject()
  const navigate = useNavigate()

  const [manualPath, setManualPath] = useState('')
  const [busy, setBusy] = useState(false)

  const openProject = (p: Project) => {
    setProject(p.workdir)
    navigate(`/project/${encodeURIComponent(p.workdir)}/kanban`)
  }

  const importDir = async (workdir: string) => {
    setBusy(true)
    try {
      const p = await importProject.mutateAsync({ workdir })
      toast.success(`项目「${p.name}」已就绪`, { description: p.workdir })
      setProject(p.workdir)
      navigate(`/project/${encodeURIComponent(p.workdir)}/kanban`)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '导入失败')
    } finally {
      setBusy(false)
    }
  }

  const pickDirectory = async () => {
    if (!window.tangoforge) {
      toast.info('当前为非桌面环境，请输入目录路径')
      return
    }
    const dir = await window.tangoforge.dialog.selectDirectory()
    if (dir) await importDir(dir)
  }

  const removeProjectRecord = async (p: Project) => {
    if (!window.confirm(`移除「${p.name}」的注册记录？（不会删除磁盘上的 .taskboard 数据）`)) return
    try {
      await removeProject.mutateAsync(p.id)
      toast.success('已移除项目记录')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '移除失败')
    }
  }

  return (
    <div className="flex gap-8">
      {/* 左侧：项目列表（行式分组，非卡片） */}
      <aside className="w-72 shrink-0">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-label uppercase tracking-wider text-muted-foreground">项目</h2>
          <Button variant="ghost" size="sm" onClick={() => void pickDirectory()} disabled={busy}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : <Plus className="size-4" />}
            导入
          </Button>
        </div>
        <Separator className="mb-4" />

        {isLoading && (
          <div className="space-y-2">
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-10 rounded-lg" />
            ))}
          </div>
        )}

        {isError && (
          <div className="rounded-lg border border-destructive-soft bg-destructive-soft/40 p-4 text-sm text-destructive-ink">
            无法连接守护进程
            <Button variant="ghost" size="sm" className="mt-2 block" onClick={() => void refetch()}>
              重试
            </Button>
          </div>
        )}

        {!isLoading && !isError && projects && projects.length === 0 && (
          <p className="text-sm text-muted-foreground">
            还没有项目，导入一个 Markdown 工作目录开始。
          </p>
        )}

        {!isLoading && !isError && projects && projects.length > 0 && (
          <ul className="space-y-0.5">
            {projects.map((p) => (
              <li key={p.id}>
                <div
                  role="button"
                  tabIndex={0}
                  onClick={() => openProject(p)}
                  onKeyDown={(e) => e.key === 'Enter' && openProject(p)}
                  className="group relative flex cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2.5 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
                >
                  <span className="size-2 shrink-0 rounded-[4px] bg-border group-hover:bg-primary-400" />
                  <span className="truncate font-medium">{p.name}</span>
                  <button
                    type="button"
                    aria-label={`移除 ${p.name}`}
                    onClick={(e) => {
                      e.stopPropagation()
                      void removeProjectRecord(p)
                    }}
                    className="ml-auto hidden rounded p-1 text-muted-foreground hover:text-destructive-ink group-hover:block"
                  >
                    <Trash2 className="size-3.5" />
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}

        {/* 手动路径兜底（Web 模式 / 目录选择不可用） */}
        <form
          className="mt-6"
          onSubmit={(e) => {
            e.preventDefault()
            if (manualPath.trim()) void importDir(manualPath.trim())
          }}
        >
          <label className="field-label text-xs text-muted-foreground">或直接输入目录路径</label>
          <div className="flex gap-2">
            <Input
              value={manualPath}
              onChange={(e) => setManualPath(e.target.value)}
              placeholder="/Users/you/projects/backlog"
              className="h-9 text-sm"
            />
            <Button
              type="submit"
              size="sm"
              variant="outline"
              aria-label="导入该路径"
              disabled={busy || !manualPath.trim()}
            >
              导入
            </Button>
          </div>
        </form>
      </aside>

      {/* 右侧：空态 / 概览 */}
      <div className="min-w-0 flex-1">
        {!isLoading && projects && projects.length === 0 && (
          <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-border py-24 text-center">
            <div className="mb-5 grid size-14 place-items-center rounded-2xl bg-primary-50 text-primary-600">
              <FolderOpen className="size-7" />
            </div>
            <h1 className="text-h2 text-foreground">从工作目录开始</h1>
            <p className="mt-2 max-w-sm text-body text-muted-foreground">
              导入一个包含 Markdown 任务文档的目录，TangoForge 会自动初始化
              <code className="mx-1 rounded bg-muted px-1 py-0.5 font-mono text-xs">
                .taskboard/
              </code>
              并解析任务。
            </p>
            <Button className="mt-6" onClick={() => void pickDirectory()} disabled={busy}>
              {busy ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <FolderOpen className="size-4" />
              )}
              选择目录导入
            </Button>
          </div>
        )}

        {!isLoading && projects && projects.length > 0 && (
          <div className="rounded-2xl border border-border bg-card p-8">
            <h1 className="text-h2 text-foreground">工作区概览</h1>
            <p className="mt-2 text-body text-muted-foreground">
              共 {projects.length}{' '}
              个项目。点击左侧项目进入看板；导入更多目录将自动初始化项目元数据。
            </p>
            <div className="mt-6 flex flex-col gap-3">
              {projects.map((p) => (
                <div
                  key={p.id}
                  className="flex items-center justify-between rounded-xl border border-divider px-4 py-3"
                >
                  <div className="min-w-0">
                    <div className="text-sm font-semibold">{p.name}</div>
                    <div className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                      {p.workdir}
                    </div>
                  </div>
                  <Button variant="outline" size="sm" onClick={() => openProject(p)}>
                    打开看板
                  </Button>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
