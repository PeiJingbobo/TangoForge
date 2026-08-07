import { arrayMove } from '@dnd-kit/sortable'
import type { StateMachineState } from '@/types/models'

/**
 * 状态列表拖拽排序（TF-032）：states / targets / rowIds 三者同步重排，
 * 索引保持对齐（targets 按状态索引存储，排序后仍与 states 对应）。
 * 纯函数，供 ProjectSettingsPage 与单元测试复用。
 */
export function reorderStateRows(
  states: StateMachineState[],
  targets: string[][],
  rowIds: string[],
  from: number,
  to: number,
): { states: StateMachineState[]; targets: string[][]; rowIds: string[] } {
  return {
    states: arrayMove(states, from, to),
    targets: arrayMove(targets, from, to),
    rowIds: arrayMove(rowIds, from, to),
  }
}
