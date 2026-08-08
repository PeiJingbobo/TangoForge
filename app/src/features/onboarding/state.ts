/**
 * 引导流程状态（TF-041）：按 workdir 记录进行中的步骤与完成态。
 * 中途关闭（未完成）→ 下次选择同一目录从上次步骤继续。
 */

const PREFIX = 'tangoforge.onboarding'

export const ONBOARDING_STEP_COUNT = 6

export interface OnboardingState {
  /** 当前步骤（0 起） */
  step: number
  /** 是否完成整个流程 */
  completed: boolean
  /** 开始时间（ISO） */
  startedAt: string
}

function keyOf(workdir: string): string {
  return `${PREFIX}.${workdir}`
}

/** 读取引导状态；无记录 → null。 */
export function getOnboardingState(workdir: string): OnboardingState | null {
  try {
    const raw = localStorage.getItem(keyOf(workdir))
    return raw ? (JSON.parse(raw) as OnboardingState) : null
  } catch {
    return null
  }
}

/** 更新引导状态（未完成时持久化；完成时标记 completed）。 */
export function setOnboardingState(
  workdir: string,
  state: Partial<OnboardingState>,
): OnboardingState {
  const prev = getOnboardingState(workdir) ?? {
    step: 0,
    completed: false,
    startedAt: new Date().toISOString(),
  }
  const next: OnboardingState = { ...prev, ...state }
  try {
    localStorage.setItem(keyOf(workdir), JSON.stringify(next))
  } catch {
    // 存储不可用时静默（引导仍可进行，仅不续走）
  }
  return next
}

/** 删除引导状态（完成/放弃）。 */
export function clearOnboardingState(workdir: string): void {
  try {
    localStorage.removeItem(keyOf(workdir))
  } catch {
    // ignore
  }
}

/** 该目录引导是否已完成（完成 → 不再弹窗）。 */
export function isOnboardingCompleted(workdir: string): boolean {
  return getOnboardingState(workdir)?.completed ?? false
}
