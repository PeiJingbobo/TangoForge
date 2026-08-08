import { useEffect, useRef, useState } from 'react'
import {
  Check,
  ChevronLeft,
  ChevronRight,
  FolderSearch,
  PartyPopper,
  Rocket,
  Sparkles,
  Wand2,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import {
  getOnboardingState,
  setOnboardingState,
  ONBOARDING_STEP_COUNT,
} from '@/features/onboarding/state'
import { WorkdirStep } from '@/features/onboarding/steps/WorkdirStep'
import { LLMStep } from '@/features/onboarding/steps/LLMStep'
import { ImportStep } from '@/features/onboarding/steps/ImportStep'
import { PermissionsStep } from '@/features/onboarding/steps/PermissionsStep'
import { SkillStep } from '@/features/onboarding/steps/SkillStep'

/**
 * 项目导入引导流程（TF-041）：
 * 步骤条 + 走马灯对话框。6 步：目录确认 → LLM 接入 → 导入草稿 → Agent 权限 → Skill → 欢迎。
 * - 中途关闭：步骤状态持久化（localStorage 按 workdir），下次同一目录从该步骤继续；
 * - 流程完成：标记 completed（不再弹窗），"进入项目" 关闭并进入指定项目。
 */
export interface OnboardingWizardProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** 目标目录（绝对路径）；切换目录时重置为步骤 0 并重新检查 */
  workdir: string
  /** 点「进入项目」触发：关闭引导并进入项目（父组件跳转） */
  onComplete: (workdir: string) => void
  /** 进入欢迎页即触发（TF-043 需求 2：「走完引导」= 到达欢迎页）：
   *  父组件借此标记项目完成（后端 hidden→可见 + 刷新列表），不关闭引导。 */
  onWelcome?: (workdir: string) => void
}

export const ONBOARDING_STEPS = [
  { key: 'workdir', label: '确认目录', icon: FolderSearch },
  { key: 'llm', label: 'LLM 接入', icon: Sparkles },
  { key: 'import', label: '导入草稿', icon: Rocket },
  { key: 'permissions', label: 'Agent 权限', icon: Wand2 },
  { key: 'skill', label: '安装 Skill', icon: Check },
  { key: 'welcome', label: '欢迎', icon: PartyPopper },
] as const

export function OnboardingWizard({
  open,
  onOpenChange,
  workdir,
  onComplete,
  onWelcome,
}: OnboardingWizardProps) {
  const [step, setStep] = useState(0)
  const [ready, setReady] = useState<boolean[]>(() => Array(ONBOARDING_STEP_COUNT).fill(false))
  // 防止同一目录重复触发 onWelcome（每目录仅一次）。
  const welcomedRef = useRef<string | null>(null)

  // 打开时：恢复该目录上次引导步骤（未完成续走）。
  useEffect(() => {
    if (!open || !workdir) return
    const prev = getOnboardingState(workdir)
    setStep(prev && !prev.completed ? Math.min(prev.step, ONBOARDING_STEP_COUNT - 1) : 0)
    setReady(Array(ONBOARDING_STEP_COUNT).fill(false))
  }, [open, workdir])

  // 步骤变化 → 持久化（中途关闭后下次续走）。
  useEffect(() => {
    if (open && workdir) setOnboardingState(workdir, { step })
  }, [open, step, workdir])

  // TF-043 需求 2：进入欢迎页（最后一步）即视为「走完引导」→ 通知父组件标记完成
  // （后端 hidden→可见、刷新列表）；不关闭引导，点「进入项目」才跳转。
  const isWelcome = step === ONBOARDING_STEP_COUNT - 1
  useEffect(() => {
    if (!open || !workdir || !isWelcome || !onWelcome) return
    if (welcomedRef.current === workdir) return
    welcomedRef.current = workdir
    onWelcome(workdir)
  }, [open, workdir, isWelcome, onWelcome])

  const setReadyAt = (idx: number, ok: boolean) => {
    setReady((prev) => {
      const next = [...prev]
      next[idx] = ok
      return next
    })
  }

  const current = ONBOARDING_STEPS[step]
  const isLast = step === ONBOARDING_STEP_COUNT - 1
  const canNext = ready[step]

  const next = () => {
    if (isLast) return
    setStep((s) => Math.min(s + 1, ONBOARDING_STEP_COUNT - 1))
  }
  const back = () => setStep((s) => Math.max(s - 1, 0))

  const complete = () => {
    setOnboardingState(workdir, { completed: true }) // 标记完成，之后不再弹引导
    onComplete(workdir)
    onOpenChange(false)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        // 中途关闭：步骤已持久化（useEffect），下次同一目录自动续走。
        onOpenChange(v)
      }}
    >
      <DialogContent className="flex max-h-[90vh] w-[min(94vw,760px)] flex-col gap-0 overflow-hidden p-0">
        {/* 步骤条 */}
        <div className="border-b border-divider px-6 pb-0 pt-5">
          <div className="flex items-center gap-1">
            {ONBOARDING_STEPS.map((s, i) => {
              const Icon = s.icon
              const active = i === step
              const done = i < step
              return (
                <div key={s.key} className="flex min-w-0 flex-1 flex-col items-center gap-1.5">
                  <div
                    className={cn(
                      'flex h-8 w-8 items-center justify-center rounded-full border text-xs font-semibold transition-colors',
                      done && 'border-primary-500 bg-primary-500 text-primary-foreground',
                      active && 'border-primary-500 bg-primary-50 text-primary-700',
                      !done && !active && 'border-border text-muted-foreground',
                    )}
                  >
                    {done ? <Check className="size-4" /> : <Icon className="size-4" />}
                  </div>
                  <span
                    className={cn(
                      'mb-2 w-full truncate text-center text-[11px] leading-tight',
                      active || done ? 'font-semibold text-foreground' : 'text-muted-foreground',
                    )}
                  >
                    {s.label}
                  </span>
                </div>
              )
            })}
          </div>
        </div>

        {/* 走马灯：当前步骤内容（滚动区） */}
        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
          <DialogHeader className="mb-4">
            <DialogTitle className="flex items-center gap-2">
              <span className="text-primary-600">
                Step {step + 1}/{ONBOARDING_STEP_COUNT}
              </span>
              {current.label}
            </DialogTitle>
            <DialogDescription>
              {step === 0 && '确认工作目录：检查是否已导入 / 历史元数据合法性，随后初始化。'}
              {step === 1 && '接入大模型：选择平台快捷配置或自定义，测试连接通过后保存。'}
              {step === 2 && '导入现有 Markdown 草稿（可选）：预览后确认，或跳过。'}
              {step === 3 && '配置 Agent 权限：允许 AI 代理访问哪些项目能力。'}
              {step === 4 && '为工作目录安装技能包：增强 Agent 的项目协作能力（可跳过）。'}
              {step === 5 && '全部完成！点击「进入项目」开始使用 TangoForge。'}
            </DialogDescription>
          </DialogHeader>

          {step === 0 && <WorkdirStep workdir={workdir} onReady={(ok) => setReadyAt(0, ok)} />}
          {step === 1 && <LLMStep workdir={workdir} onReady={(ok) => setReadyAt(1, ok)} />}
          {step === 2 && <ImportStep workdir={workdir} onReady={(ok) => setReadyAt(2, ok)} />}
          {step === 3 && <PermissionsStep workdir={workdir} onReady={(ok) => setReadyAt(3, ok)} />}
          {step === 4 && <SkillStep workdir={workdir} onReady={(ok) => setReadyAt(4, ok)} />}
          {step === 5 && (
            <div className="flex flex-col items-center gap-3 py-8 text-center">
              <PartyPopper className="size-12 text-primary-500" />
              <p className="text-sm text-muted-foreground">
                项目「{workdir.split('/').filter(Boolean).pop()}
                」已完成全部引导设置，已加入项目列表。
              </p>
              <p className="text-xs text-muted-foreground/70">
                点击下方「进入项目」开始使用 TangoForge。
              </p>
            </div>
          )}
        </div>

        {/* 底部导航 */}
        <div className="flex items-center justify-between border-t border-divider px-6 py-3.5">
          <Button variant="ghost" size="sm" onClick={back} disabled={step === 0}>
            <ChevronLeft className="size-4" /> 上一步
          </Button>
          <span className="text-caption text-muted-foreground">关闭后可从当前步骤继续</span>
          {isLast ? (
            <Button size="sm" onClick={complete}>
              <Rocket className="size-4" /> 进入项目
            </Button>
          ) : (
            <Button size="sm" onClick={next} disabled={!canNext}>
              下一步 <ChevronRight className="size-4" />
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
