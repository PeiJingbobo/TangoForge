import { create } from 'zustand'
import type { Task } from '@/types/task'

/** 任务详情抽屉模式 */
export type TaskDrawerMode = 'edit' | 'read'

export interface OpenTaskDrawerOptions {
  /** 任务 id（优先级高：内部加载详情并内部保存） */
  taskId?: string
  /** 任务详情对象（传入时直接使用；编辑完成经 onSaved 回调最新详情） */
  task?: Task
  mode?: TaskDrawerMode
  /** 编辑保存成功回调（task 对象模式必需；taskId 模式保存后也会回调最新详情） */
  onSaved?: (task: Task) => void
}

/** 抽屉栈中单层条目（一层 = 一个 Dialog 页面） */
export interface TaskDrawerEntry {
  taskId?: string
  task?: Task
  mode: TaskDrawerMode
  onSaved?: (task: Task) => void
  /** 是否处于打开态；false = 正在播关闭动画（保持挂载，动画结束后移除） */
  open: boolean
}

interface TaskDrawerState {
  /** Dialog 页面堆栈：尾部为当前顶层；从详情内打开关联任务 → push 新层，返回 → pop */
  stack: TaskDrawerEntry[]
  /** 打开抽屉（外部入口：看板/导航/全景图/新建，重置栈为单层） */
  openDrawer: (opts: OpenTaskDrawerOptions) => void
  /** 从任务详情内打开关联任务（依赖/子任务/父任务）：压入新的一层 Dialog */
  pushTask: (opts: OpenTaskDrawerOptions) => void
  /** 弹出顶层（返回上一个任务）：先标记关闭播动画，动画结束后移除；栈空即关闭抽屉 */
  popTask: () => void
  /** 关闭全部层（清空栈） */
  closeDrawer: () => void
}

/** 关闭动画时长（与 sheet.tsx data-[state=closed]:duration-300 对齐，稍留余量） */
const CLOSE_ANIM_MS = 350

function toEntry(opts: OpenTaskDrawerOptions): TaskDrawerEntry {
  return {
    taskId: opts.taskId,
    task: opts.task,
    mode: opts.mode ?? 'edit',
    onSaved: opts.onSaved,
    open: true,
  }
}

/**
 * 全局任务详情抽屉（zustand，Dialog 页面堆栈）：
 * 看板/导航/全景图等入口 openDrawer({ taskId }) 或 openDrawer({ task, onSaved }) 打开根层；
 * 详情内点击依赖/子任务/父任务经 pushTask 压入新层，逐层返回（popTask，带关闭动画）。
 */
export const useTaskDrawerStore = create<TaskDrawerState>((set) => ({
  stack: [],
  openDrawer: (opts) => set({ stack: [toEntry(opts)] }),
  pushTask: (opts) => set((s) => ({ stack: [...s.stack, toEntry(opts)] })),
  popTask: () => {
    set((s) => {
      if (s.stack.length === 0) return s
      const stack = s.stack.map((e, i) => (i === s.stack.length - 1 ? { ...e, open: false } : e))
      return { stack }
    })
    // 关闭动画结束后移除末尾处于关闭态的层（含连点/中途压入新层时只清关闭态尾部）
    window.setTimeout(() => {
      useTaskDrawerStore.setState((s) => {
        if (s.stack.length === 0) return s
        const stack = [...s.stack]
        while (stack.length > 0 && !stack[stack.length - 1].open) stack.pop()
        return stack.length === s.stack.length ? s : { stack }
      })
    }, CLOSE_ANIM_MS)
  },
  closeDrawer: () => set({ stack: [] }),
}))
