import { describe, expect, it } from 'vitest'
import { cn } from './utils'

describe('cn()', () => {
  it('合并 class 字符串', () => {
    expect(cn('a', 'b', 'c')).toBe('a b c')
  })

  it('过滤假值参数', () => {
    expect(cn('a', false, undefined, null, 'b')).toBe('a b')
  })

  it('使用 tailwind-merge 消除冲突类', () => {
    expect(cn('px-2', 'px-4')).toBe('px-4')
  })
})
