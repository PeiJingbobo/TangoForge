/**
 * 后端统一响应包装（docs/TASK-SEMANTICS.md §10）
 * 成功：{ code: 0, data: T }；失败：{ code: "<业务码>", message, detail? }
 */

export type ErrorCode =
  | 'PROJECT_NOT_FOUND'
  | 'TASK_NOT_FOUND'
  | 'TASK_INVALID'
  | 'INVALID_TRANSITION'
  | 'STATUS_IN_USE'
  | 'CIRCULAR_DEPENDENCY'
  | 'DELETE_NOT_ALLOWED'
  | 'PERMISSION_DENIED'
  | 'IMPORT_FAILED'
  | 'LLM_TRUNCATED'
  | 'SKILL_NOT_FOUND'
  | 'STATE_MACHINE_INVALID'
  | 'REMOTE_DISABLED'
  | 'UNAUTHORIZED'
  | 'NOT_FOUND'
  | 'BAD_REQUEST'
  | 'INTERNAL_ERROR'
  | 'NETWORK_ERROR'
  | 'TIMEOUT'

/** 统一响应信封（code 成功为数字 0，失败为字符串业务码） */
export interface ApiEnvelope<T> {
  code: number | string
  data?: T
  message?: string
  detail?: string
}

/** 本地错误码 → 兜底中文提示（后端 message 优先，此处仅本地错误与兜底） */
export const ERROR_MESSAGES: Record<string, string> = {
  PROJECT_NOT_FOUND: '项目未找到或未注册',
  TASK_NOT_FOUND: '任务不存在',
  TASK_INVALID: '任务参数不合法',
  INVALID_TRANSITION: '非法状态流转',
  STATUS_IN_USE: '状态已被任务占用，无法移除',
  CIRCULAR_DEPENDENCY: '存在循环依赖，操作被拒绝',
  DELETE_NOT_ALLOWED: '仅归档任务可物理删除',
  PERMISSION_DENIED: '权限不足',
  IMPORT_FAILED: '导入解析失败',
  LLM_TRUNCATED: 'LLM 输出被截断，请增大 max_tokens 或换用非推理模型',
  SKILL_NOT_FOUND: 'Skill 不存在',
  STATE_MACHINE_INVALID: '状态机定义不合法',
  REMOTE_DISABLED: '远程访问已关闭',
  UNAUTHORIZED: '未认证，需要有效令牌',
  NOT_FOUND: '资源不存在',
  BAD_REQUEST: '请求参数错误',
  INTERNAL_ERROR: '服务内部错误',
  NETWORK_ERROR: '网络错误，无法连接守护进程',
  TIMEOUT: '请求超时',
}

/** 请求上下文：项目目录以 X-Project 头显式传递（禁止隐式"当前项目"） */
export interface RequestContext {
  project?: string
}
