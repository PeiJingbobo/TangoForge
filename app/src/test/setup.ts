import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// 每个测试后自动清理 DOM，避免用例间相互污染。
afterEach(() => {
  cleanup()
})
