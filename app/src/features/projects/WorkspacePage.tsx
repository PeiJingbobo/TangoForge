import { useState } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { FolderOpen, Loader2, Plus } from 'lucide-react'
import { useCompleteOnboarding, useProjectCheck, useProjects } from '@/hooks/useProjects'
import { useProjectStore } from '@/stores/project'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { OnboardingWizard } from '@/features/onboarding/OnboardingWizard'
import type { Project } from '@/types/models'

/**
 * 项目概览（TF-029 / TF-041 引导 / TF-043 暂时隐藏）：
 * 欢迎引导 + 导入入口 + 最近项目（仅已完成引导的可见项目）。
 * - 导入项目默认**暂时隐藏**（后端 hidden=1，不在列表展示）；
 * - 走完引导（欢迎页「进入项目」→ POST /api/projects/complete）后列表可见；
 * - 选择目录：check 判定——已注册且引导完成 → 直接进入；否则打开引导续走。
 */
export function WorkspacePage() {
  const { data: projects } = useProjects()
  const checkProject = useProjectCheck()
  const completeOnboarding = useCompleteOnboarding()
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
   * 选择/输入目录后的入口（TF-043）：
   * - check.registered && check.onboarded → 已走完引导，直接进入项目；
   * - 否则（未注册 / 引导未完成）→ 打开引导流程（WorkdirStep 内部完成检查/清空/注册，
   *   完成引导后由 complete 端点置可见）。不再预注册（TF-041 修复）。
   */
  const handleDir = async (workdir: string) => {
    setBusy(true)
    try {
      const r = await checkProject.mutateAsync(workdir)
      if (r.registered && r.onboarded) {
        const p = projects?.find((x) => x.workdir === workdir)
        openProject(
          p ?? {
            id: 0,
            name: workdir.split('/').filter(Boolean).pop() ?? workdir,
            workdir,
            created_at: '',
            last_opened_at: null,
            hidden: false,
          },
        )
      } else {
        setWizardDir(workdir)
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

  // 列表仅含可见项目（已完成引导）→ 点击直接进入。
  const openProjectOrContinue = (p: Project) => openProject(p)

  /** 进入欢迎页即「走完引导」（TF-043 需求 2）：后端标记可见 + 刷新列表；不关闭引导。 */
  const handleWelcome = async (wd: string) => {
    try {
      await completeOnboarding.mutateAsync(wd)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '引导完成标记失败')
    }
  }

  /** 点「进入项目」：关闭引导并进入项目（此时项目已在列表）。 */
  const handleComplete = (wd: string) => {
    const p = projects?.find((x) => x.workdir === wd)
    openProject(
      p ?? {
        id: 0,
        name: wd.split('/').filter(Boolean).pop() ?? wd,
        workdir: wd,
        created_at: '',
        last_opened_at: null,
        hidden: false,
      },
    )
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
          自动创建并识别项目（LLM / 导入 / 权限 / Skill 一键配置）；引导完成前项目暂不展示。
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

      {/* 最近项目（仅已完成引导的可见项目） */}
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
                onClick={() => openProjectOrContinue(p)}
                onKeyDown={(e) => e.key === 'Enter' && openProjectOrContinue(p)}
              >
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-sm font-semibold">{p.name}</div>
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

      {/* 引导流程 */}
      <OnboardingWizard
        open={!!wizardDir}
        onOpenChange={(v) => {
          if (!v) setWizardDir(null)
        }}
        workdir={wizardDir ?? ''}
        onWelcome={(wd) => void handleWelcome(wd)}
        onComplete={(wd) => void handleComplete(wd)}
      />
    </div>
  )
}
