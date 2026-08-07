import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { DndContext } from '@dnd-kit/core'
import { KanbanColumn } from './kanban-column'
import type { Task } from '@/types/task'

function mk(id: string, title: string): Task {
  return {
    id,
    project_id: 1,
    parent_id: null,
    title,
    description: '',
    status: 'todo',
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

const TASKS = [mk('t1', '任务一'), mk('t2', '任务二'), mk('t3', '任务三')]

function renderColumn(placeholderIndex?: number, isDropTarget?: boolean) {
  return render(
    <DndContext>
      <KanbanColumn
        col={{ Key: 'todo', Label: '待办', Color: '#9aa0a6' }}
        tasks={TASKS}
        placeholderIndex={placeholderIndex}
        isDropTarget={isDropTarget}
        onOpenTask={() => {}}
      />
    </DndContext>,
  )
}

describe('KanbanColumn（占位符）', () => {
  beforeEach(() => {
    // 虚拟滚动 + 动态测量：jsdom 无布局，RO 回调带 target、测量返回估算高度
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(
          private cb: (entries: { target: Element; borderBoxSize?: unknown[] }[]) => void,
        ) {}
        observe(target: Element): void {
          this.cb([{ target, borderBoxSize: [{ inlineSize: 260, blockSize: 108 }] }])
        }
        unobserve(): void {}
        disconnect(): void {}
      },
    )
    Element.prototype.getBoundingClientRect = vi.fn(function (this: Element) {
      if (this.hasAttribute && this.hasAttribute('data-index')) {
        return {
          x: 0,
          y: 0,
          width: 260,
          height: 108,
          top: 0,
          left: 0,
          right: 260,
          bottom: 108,
          toJSON: () => ({}),
        }
      }
      return {
        x: 0,
        y: 0,
        width: 300,
        height: 800,
        top: 0,
        left: 0,
        right: 300,
        bottom: 800,
        toJSON: () => ({}),
      }
    })
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('无占位符：渲染全部任务卡片，无拖拽指示线', () => {
    renderColumn()
    expect(screen.getByRole('button', { name: '任务 任务一' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '任务 任务三' })).toBeInTheDocument()
    expect(document.querySelectorAll('[data-testid="drop-indicator"]').length).toBe(0)
  })

  it('占位符 index=1：指示线渲染（目标第 2 张卡顶部），前 2 张卡保留', () => {
    renderColumn(1)
    expect(document.querySelectorAll('[data-testid="drop-indicator"]').length).toBe(1)
    expect(screen.getByRole('button', { name: '任务 任务一' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '任务 任务二' })).toBeInTheDocument()
  })

  it('占位符 index=0：指示线在最前，任务仍在', () => {
    renderColumn(0)
    expect(document.querySelectorAll('[data-testid="drop-indicator"]').length).toBe(1)
    expect(screen.getByRole('button', { name: '任务 任务一' })).toBeInTheDocument()
  })

  it('占位符 index=3（末尾）：指示线在最后卡片下方，任务全部渲染', () => {
    renderColumn(3)
    expect(document.querySelectorAll('[data-testid="drop-indicator"]').length).toBe(1)
    expect(screen.getByRole('button', { name: '任务 任务三' })).toBeInTheDocument()
  })

  it('TF-036 拖拽目标归属列 → 列高亮边框（卡片间/空白区一致）', () => {
    renderColumn(1, true)
    // 通过容器结构定位列根 div（bg-muted + rounded-[14px]），应带 ring 高亮类。
    const root = [...document.querySelectorAll('div')].find(
      (el) => el.className.includes('bg-muted') && el.className.includes('rounded-[14px]'),
    )
    expect(root).toBeTruthy()
    expect(root!.className).toContain('ring-2')
    expect(root!.className).toContain('ring-primary-300')
  })

  it('TF-036 非拖拽目标 → 无高亮边框', () => {
    renderColumn(undefined, false)
    const root = [...document.querySelectorAll('div')].find(
      (el) => el.className.includes('bg-muted') && el.className.includes('rounded-[14px]'),
    )
    expect(root!.className).not.toContain('ring-primary-300')
  })
})
