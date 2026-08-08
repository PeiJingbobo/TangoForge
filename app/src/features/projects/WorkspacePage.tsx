import { useState } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { FolderOpen, Loader2, Plus } from 'lucide-react'
import { useImportProject, useProjects } from '@/hooks/useProjects'
import { useProjectStore } from '@/stores/project'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { OnboardingWizard } from '@/features/onboarding/OnboardingWizard'
import { getOnboardingState, isOnboardingCompleted } from '@/features/onboarding/state'
import type { Project } from '@/types/models'

/**
 * 项目概览（TF-029 / TF-041 引导）：
 * 欢迎引导 + 导入入口 + 最近项目（点击激活进入看板）。
 * 选择目录后进入「项目导入引导流程」（OnboardingWizard，6 步走马灯）；
 * 中途关闭 → 步骤持久化，下次同一目录续走；完成后直接进入项目。
 */
export function WorkspacePage() {
  const { data: projects } = useProjects()
  const importProject = useImportProject()
  const setProject = useProjectStore((s) => s.setProject)
  const navigate = useNavigate()

  const [manualPath, setManualPath] = useState('')
  const [busy, setBusy] = useState(false)
  const [wizardDir, setWizardDir] = useState<string | null>(null)

  const openProject = (p: Project) => {
    setProject(p.workdir)
    navigate(`/project/${encodeURIComponent(p.workdir)}/kanban`)
  }

  /**
   * 选择/输入目录后的入口：已完成引导或已注册项目 → 直接打开；
   * 否则打开引导流程（未完成时从上次步骤续走）。
   */
  const handleDir = async (workdir: string) => {
    setBusy(true)
    try {
      const p = await importProject.mutateAsync({ workdir })
      if (isOnboardingCompleted(workdir)) {
        openProject(p)
      } else {
        setWizardDir(workdir) // 打开引导（续走或从头）
      }
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
    if (dir) await handleDir(dir)
  }

  // 最近项目点击：未完成引导 → 续走；已完成 → 直接进入。
  const openProjectOrContinue = (p: Project) => {
    if (!isOnboardingCompleted(p.workdir) && getOnboardingState(p.workdir)) {
      setWizardDir(p.workdir)
      return
    }
    openProject(p)
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
          选择项目工作目录，进入项目引导流程
          <code className="mx-1 rounded bg-muted px-1 py-0.5 font-mono text-xs">.taskboard/</code>
          自动创建并识别项目（LLM / 导入 / 权限 / Skill 一键配置）。
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
              if (manualPath.trim()) void handleDir(manualPath.trim())
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
            {projects.map((p) => {
              const pending = !isOnboardingCompleted(p.workdir) && !!getOnboardingState(p.workdir)
              return (
                <div
                  key={p.id}
                  className="flex cursor-pointer items-center justify-between rounded-xl border border-divider px-4 py-3 transition-colors hover:border-primary-300"
                  role="button"
                  tabIndex={0}
                  onClick={() => openProjectOrContinue(p)}
                  onKeyDown={(e) => e.key === 'Enter' && openProjectOrContinue(p)}
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 text-sm font-semibold">
                      {p.name}
                      {pending && (
                        <span className="rounded-full bg-warning-soft px-2 py-px text-[10px] font-medium text-warning-ink">
                          引导未完成
                        </span>
                      )}
                    </div>
                    <div className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                      {p.workdir}
                    </div>
                  </div>
                  <Button variant="ghost" size="sm">
                    {pending ? '继续引导' : '打开'}
                  </Button>
                </div>
              )
            })}
          </div>
        </div>
      )}

      {/* 引导流程 */}
      <OnboardingWizard
        open={!!wizardDir}
        onOpenChange={(v) => {
          if (!v) setWizardDir(null)
        }}
        workdir={wizardDir ?? ''}
        onComplete={(wd) => {
          const p = projects?.find((x) => x.workdir === wd)
          if (p) openProject(p)
        }}
      />
    </div>
  )
}
