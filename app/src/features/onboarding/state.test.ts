import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import {
  getOnboardingState,
  setOnboardingState,
  clearOnboardingState,
  isOnboardingCompleted,
} from './state'

describe('onboarding state（TF-041 localStorage 持久化）', () => {
  beforeEach(() => localStorage.clear())
  afterEach(() => localStorage.clear())

  it('无记录 → null / 未完成', () => {
    expect(getOnboardingState('/p')).toBeNull()
    expect(isOnboardingCompleted('/p')).toBe(false)
  })

  it('步骤持久化 + 按目录隔离', () => {
    setOnboardingState('/p1', { step: 2 })
    setOnboardingState('/p2', { step: 4 })
    expect(getOnboardingState('/p1')?.step).toBe(2)
    expect(getOnboardingState('/p2')?.step).toBe(4)
  })

  it('完成标记 + 清除', () => {
    setOnboardingState('/p', { completed: true })
    expect(isOnboardingCompleted('/p')).toBe(true)
    clearOnboardingState('/p')
    expect(isOnboardingCompleted('/p')).toBe(false)
  })

  it('部分更新保留已存字段', () => {
    setOnboardingState('/p', { step: 1 })
    setOnboardingState('/p', { completed: true })
    const s = getOnboardingState('/p')
    expect(s?.step).toBe(1)
    expect(s?.completed).toBe(true)
  })
})
