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

interface TaskDrawerState {
  open: boolean
  taskId?: string
  task?: Task
  mode: TaskDrawerMode
  onSaved?: (task: Task) => void
  openDrawer: (opts: OpenTaskDrawerOptions) => void
  closeDrawer: () => void
}

/**
 * 全局任务详情抽屉（zustand）：
 * 看板/导航/全景图等入口 openDrawer({ taskId }) 或 openDrawer({ task, onSaved })，
 * 由 AppLayout 挂载的 GlobalTaskDrawer 渲染（当前页保留，抽屉浮层覆盖）。
 */
export const useTaskDrawerStore = create<TaskDrawerState>((set) => ({
  open: false,
  taskId: undefined,
  task: undefined,
  mode: 'edit',
  onSaved: undefined,
  openDrawer: ({ taskId, task, mode = 'edit', onSaved }) =>
    set({ open: true, taskId, task, mode, onSaved }),
  closeDrawer: () => set({ open: false, taskId: undefined, task: undefined, onSaved: undefined }),
}))
