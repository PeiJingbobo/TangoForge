/** 任务域常量（独立文件避免组件文件导出非组件内容） */
export const PRIORITY_OPTIONS = [
  { value: 0, label: 'P0 无' },
  { value: 1, label: 'P1 低' },
  { value: 2, label: 'P2 中' },
  { value: 3, label: 'P3 高' },
  { value: 4, label: 'P4 很高' },
  { value: 5, label: 'P5 紧急' },
] as const
