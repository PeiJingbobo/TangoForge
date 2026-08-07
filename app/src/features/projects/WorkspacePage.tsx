import { useState } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { FolderOpen, Loader2, Plus } from 'lucide-react'
import { useImportProject, useProjects } from '@/hooks/useProjects'
import { useProjectStore } from '@/stores/project'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import type { Project } from '@/types/models'

/**
 * 项目概览（TF-029：公共一级「项目概览」）：
 * 欢迎引导 + 导入入口 + 最近项目（点击激活进入看板）。
 * 项目列表主体在左侧全局导航（AppLayout）。
 */
export function WorkspacePage() {
  const { data: projects } = useProjects()
  const importProject = useImportProject()
  const setProject = useProjectStore((s) => s.setProject)
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
      openProject(p)
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

  return (
    <div className="mx-auto max-w-2xl">
      <div className="flex items-center gap-3">
        <span className="grid size-12 place-items-center rounded-2xl bg-primary-50 text-primary-600">
          <FolderOpen className="size-6" />
        </span>
        <div>
          <h1 className="text-h2 text-foreground">项目概览</h1>
          <p className="text-caption text-muted-foreground">
            在左侧激活项目后，右侧出现该项目的看板 / 导航 / 全景图 / 导入导出等功能。
          </p>
        </div>
      </div>

      {/* 导入 */}
      <div className="mt-6">
        <Separator className="mb-5" />
        <h2 className="text-h3 text-foreground">导入工作目录</h2>
        <p className="mt-1 text-body text-muted-foreground">
          选择项目工作目录，进入项目初始化流程
          <code className="mx-1 rounded bg-muted px-1 py-0.5 font-mono text-xs">.taskboard/</code>
          自动创建并识别项目。
        </p>
        <div className="mt-4 flex flex-wrap items-center gap-2">
          <Button onClick={() => void pickDirectory()} disabled={busy}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : <FolderOpen className="size-4" />}
            选择目录导入
          </Button>
          <span className="text-caption text-muted-foreground">或</span>
          <form
            className="flex flex-1 gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              if (manualPath.trim()) void importDir(manualPath.trim())
            }}
          >
            <Input
              value={manualPath}
              onChange={(e) => setManualPath(e.target.value)}
              placeholder="/Users/you/projects/backlog"
              className="h-9 flex-1 text-sm"
              aria-label="目录路径"
            />
            <Button
              type="submit"
              variant="outline"
              size="sm"
              aria-label="导入该路径"
              disabled={busy || !manualPath.trim()}
            >
              <Plus className="size-4" />
              导入
            </Button>
          </form>
        </div>
      </div>

      {/* 最近项目 */}
      {projects && projects.length > 0 && (
        <div className="mt-8">
          <Separator className="mb-5" />
          <h2 className="text-h3 text-foreground">最近项目</h2>
          <div className="mt-3 flex flex-col gap-2">
            {projects.map((p) => (
              <div
                key={p.id}
                className="flex cursor-pointer items-center justify-between rounded-xl border border-divider px-4 py-3 transition-colors hover:border-primary-300"
                role="button"
                tabIndex={0}
                onClick={() => openProject(p)}
                onKeyDown={(e) => e.key === 'Enter' && openProject(p)}
              >
                <div className="min-w-0">
                  <div className="text-sm font-semibold">{p.name}</div>
                  <div className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                    {p.workdir}
                  </div>
                </div>
                <Button variant="ghost" size="sm">
                  打开
                </Button>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
