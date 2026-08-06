import { describe, it, expect } from 'vitest'
import { resolveDragTarget } from './drag-logic'
import type { Task } from '@/types/task'

function mk(id: string, status: string): Task {
  return {
    id,
    project_id: 1,
    parent_id: null,
    title: id,
    description: '',
    status,
    priority: 0,
    tags: [],
    assignee: '',
    depends_on: [],
    archived_from: '',
    source_file: '',
    source_section: '',
    created_at: '2026-08-06T10:00:00+08:00',
    updated_at: '2026-08-06T10:00:00+08:00',
  }
}

const TASKS = new Map([
  ['t1', mk('t1', 'todo')],
  ['t2', mk('t2', 'doing')],
  ['t3', mk('t3', 'done')],
])
const COLS = ['todo', 'doing', 'done']

describe('resolveDragTarget（拖拽目标解析）', () => {
  it('拖到列空白 → 目标为该列', () => {
    expect(resolveDragTarget('t1', 'doing', TASKS, COLS)).toEqual({ taskId: 't1', to: 'doing' })
  })

  it('拖到某任务卡片 → 目标为该卡片所在列', () => {
    expect(resolveDragTarget('t1', 't3', TASKS, COLS)).toEqual({ taskId: 't1', to: 'done' })
  })

  it('同列拖拽 → null（无操作）', () => {
    expect(resolveDragTarget('t1', 'todo', TASKS, COLS)).toBeNull()
    expect(resolveDragTarget('t2', 'doing', TASKS, COLS)).toBeNull()
    expect(resolveDragTarget('t3', 'done', TASKS, COLS)).toBeNull()
  })

  it('无效 active 任务 / 无效 over → null', () => {
    expect(resolveDragTarget('ghost', 'doing', TASKS, COLS)).toBeNull()
    expect(resolveDragTarget('t1', 'ghost', TASKS, COLS)).toBeNull()
  })

  it('拖到 archived 列（看板不渲染，防御性）→ 返回目标', () => {
    expect(
      resolveDragTarget('t1', 'archived', TASKS, ['todo', 'doing', 'done', 'archived']),
    ).toEqual({
      taskId: 't1',
      to: 'archived',
    })
  })
})
