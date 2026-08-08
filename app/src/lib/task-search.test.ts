import { describe, it, expect } from 'vitest'
import { matchesTaskQuery } from '@/lib/task-search'
import type { Task } from '@/types/task'

const task = (partial: Partial<Task>): Task => ({
  id: 't1',
  project_id: 1,
  parent_id: null,
  title: '前端 token 接入',
  number: 'T012',
  description: '打通 OpenAI 兼容接口的鉴权流程',
  status: 'todo',
  priority: 2,
  tags: [],
  assignee: '',
  depends_on: [],
  archived_from: '',
  source_file: '',
  source_section: '',
  created_at: '2026-08-08T10:00:00+08:00',
  updated_at: '2026-08-08T10:00:00+08:00',
  ...partial,
})

describe('matchesTaskQuery（TF-042：编号/标题/内容三字段搜索）', () => {
  it('空 query 恒匹配', () => {
    expect(matchesTaskQuery(task({}), '')).toBe(true)
    expect(matchesTaskQuery(task({}), '   ')).toBe(true)
  })

  it('按标题匹配（大小写不敏感）', () => {
    expect(matchesTaskQuery(task({}), 'token')).toBe(true)
    expect(matchesTaskQuery(task({}), 'TOKEN')).toBe(true)
    expect(matchesTaskQuery(task({}), '前端')).toBe(true)
  })

  it('按任务编号匹配（T 前缀 + 数字）', () => {
    expect(matchesTaskQuery(task({}), 'T012')).toBe(true)
    expect(matchesTaskQuery(task({}), 't012')).toBe(true)
    expect(matchesTaskQuery(task({}), '012')).toBe(true)
  })

  it('按内容匹配', () => {
    expect(matchesTaskQuery(task({}), '鉴权')).toBe(true)
    expect(matchesTaskQuery(task({}), 'OpenAI')).toBe(true)
    expect(matchesTaskQuery(task({}), '兼容接口')).toBe(true)
  })

  it('不匹配返回 false', () => {
    expect(matchesTaskQuery(task({}), '不存在的内容xyz')).toBe(false)
  })
})
