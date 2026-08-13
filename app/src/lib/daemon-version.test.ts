import { describe, it, expect } from 'vitest'
import { shouldRestartDaemon } from './daemon-version'

describe('shouldRestartDaemon（TF-053 版本比对）', () => {
  it('版本一致 → 不重启', () => {
    expect(shouldRestartDaemon('0.6.5', '0.6.5')).toBe(false)
  })

  it('版本不一致 → 重启', () => {
    expect(shouldRestartDaemon('0.6.5', '0.6.4')).toBe(true)
  })

  it('running 为空（无法探测）→ 不重启（避免误杀）', () => {
    expect(shouldRestartDaemon('0.6.5', null)).toBe(false)
    expect(shouldRestartDaemon('0.6.5', '')).toBe(false)
  })

  it('required 为空（dev/未注入）→ 不重启', () => {
    expect(shouldRestartDaemon('', '0.6.5')).toBe(false)
  })
})
