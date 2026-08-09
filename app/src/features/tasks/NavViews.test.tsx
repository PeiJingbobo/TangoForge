import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TimelineView, StatusView } from './NavViews'
import type { StateMachineState } from '@/types/models'
import type { Task } from '@/types/task'

const STATES: StateMachineState[] = [
  { Key: 'todo', Label: '待办', Color: '#9aa0a6' },
  { Key: 'doing', Label: '进行中', Color: '#1a73e8' },
]

function mk(id: string, title: string, status: string): Task {
  return {
    id,
    project_id: 1,
    parent_id: null,
    title,
    number: '',
    description: '',
    status,
    priority: 0,
    tags: [],
    assignee: '',
    depends_on: [],
    archived_from: '',
    source_file: '',
    source_section: '',
    created_at: '2026-08-09T10:00:00+08:00',
    updated_at: '2026-08-09T10:00:00+08:00',
  }
}

describe('NavViews 任务状态展示（QA 2026-08-09）', () => {
  it('TimelineView：任务行显示状态机 Label 徽标（含彩色圆点）', () => {
    render(<TimelineView tasks={[mk('a', '任务甲', 'doing')]} states={STATES} onOpen={() => {}} />)
    expect(screen.getByText('任务甲')).toBeInTheDocument()
    expect(screen.getByText('进行中')).toBeInTheDocument()
    const dot = document.querySelector('span[style*="background-color: rgb(26, 115, 232)"]')
    expect(dot).not.toBeNull()
  })

  it('StatusView：分组头使用状态机颜色圆点 + 行内状态徽标', () => {
    render(
      <StatusView
        tasks={[mk('a', '任务甲', 'todo'), mk('b', '任务乙', 'doing')]}
        states={STATES}
        onOpen={() => {}}
      />,
    )
    // 「待办/进行中」出现在分组头与任务行状态徽标
    expect(screen.getAllByText('待办').length).toBeGreaterThan(0)
    expect(screen.getAllByText('进行中').length).toBeGreaterThan(0)
  })
})
