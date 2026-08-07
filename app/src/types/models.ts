/**
 * 后端模型 DTO（与 internal/api handlers 的 json tag 字段级同步）。
 * 语义要点见 docs/TASK-SEMANTICS.md：
 * - 时间均为 RFC3339 本地时区字符串；
 * - state-machine 是唯一 PascalCase 结构（无 json tag）；
 * - /api/projects 豁免 X-Project；其余端点必须携带。
 */
import type { Task } from './task'

/* ---------- projects（/api/projects，豁免 X-Project） ---------- */
export interface Project {
  id: number
  name: string
  workdir: string
  created_at: string
  last_opened_at: string | null
}

export interface ProjectImportRequest {
  /** 绝对路径；不存在或已初始化时后端按语义处理 */
  workdir: string
}

/* ---------- state-machine（⚠️ PascalCase） ---------- */
export interface StateMachineState {
  Key: string
  Label: string
  Color: string
}
export interface StateMachineTransition {
  From: string
  To: string[]
}
export interface StateMachineDTO {
  States: StateMachineState[]
  Transitions: StateMachineTransition[]
}

/* ---------- project-config（GET/PUT /api/project-config，TF-032，⚠️ PascalCase） ---------- */
export interface ExportConfigDTO {
  TemplatePath: string
}
/** 项目 config.yaml 全量视图（state_machine + export；PUT 为全量覆盖，仅 UI） */
export interface ProjectConfigDTO {
  StateMachine: StateMachineDTO
  Export: ExportConfigDTO
}

/* ---------- graph（GET /api/graph） ---------- */
export interface GraphEdge {
  from: string
  to: string
  /** parent: from=父 to=子；dependency: from=被依赖 to=依赖者 */
  type: 'parent' | 'dependency'
}
export interface GraphData {
  nodes: Task[]
  edges: GraphEdge[]
}

/* ---------- import 草稿流（TF-018） ---------- */
export interface ParseInput {
  /** 四形态取一：单文件 / 多文件合并 / 目录递归 / 原始内容+source_file */
  file_path?: string
  file_paths?: string[]
  directory?: string
  content?: string
  source_file?: string
}

export interface ImportDraft {
  id: string
  source_file: string
  status: 'pending' | 'confirmed' | 'discarded'
  task_count: number
  created_at: string
}

export interface ImportConfirmResult {
  draft_id: string
  source_file: string
  created: number
  /** 文件级全量覆盖：旧 source_file 任务被归档数 */
  archived: number
  /** 无法解析的依赖引用数（标题已修改/引用失效），已忽略继续导入 */
  dropped_deps: number
}

/** 草稿解析任务（虚拟任务体系：状态机 key / 优先级 0-5 / 依赖经临时 id 引用） */
export interface ParsedTask {
  /** 草稿内临时唯一编号（LLM 生成；依赖 depends_on 引用该 id，与标题解耦） */
  id: string
  title: string
  description: string
  status: string
  priority: number
  tags: string[]
  assignee: string
  /** 被依赖任务的临时 id（旧草稿可能为标题引用，渲染层会规范化） */
  depends_on: string[]
  children?: ParsedTask[]
}

/** 草稿明细（审阅界面数据源：完整任务树） */
export interface DraftDetail extends ImportDraft {
  tasks: ParsedTask[]
}

/* ---------- export（TF-019） ---------- */
export interface RenderOptions {
  template_mode?: 'default' | 'llm'
  target?: 'overwrite' | 'copy'
  /** overwrite 必填；copy 缺省 {workdir}/.taskboard/export.md */
  path?: string
}
export interface RenderResult {
  content: string
  path: string
}
export interface TemplateGenerateResult {
  template: string
  path: string
}

/* ---------- permissions（仅 UI 可写） ---------- */
export const ACTION_KEYS = [
  'project.read',
  'task.read',
  'task.create',
  'task.update',
  'task.update_status',
  'task.delete',
  'task.restore',
  'import.run',
  'import.confirm',
  'export.run',
  'graph.read',
  'skill.read',
  'state_machine.read',
  'state_machine.write',
  'audit.read',
  'permission.read',
] as const
export type ActionKey = (typeof ACTION_KEYS)[number]
export type PermissionMap = Record<ActionKey, boolean>

/* ---------- skills ---------- */
export interface Skill {
  name: string
  version: string
  description: string
  instructions: string
  content: string
  updated_at: string
}

/* ---------- audit ---------- */
export interface AuditEntry {
  ts: string
  actor: string
  actor_class: 'ui' | 'agent' | 'unknown'
  action: string
  target: string
  result: 'ok' | 'denied' | 'error'
  detail: string
}
export interface AuditQueryResult {
  items: AuditEntry[]
  total: number
  page: number
  size: number
}

/* ---------- WS 事件（GET /ws/events?project=） ---------- */
export const WS_EVENT_TYPES = [
  'task.created',
  'task.updated',
  'task.status_changed',
  'task.archived',
  'task.restored',
  'task.deleted',
  'state_machine.changed',
  'export.complete',
  'import.draft_ready',
  'import.draft_confirmed',
  'import.draft_discarded',
  'import.failed',
] as const
export type WSEventType = (typeof WS_EVENT_TYPES)[number]

export interface WSEvent {
  type: string
  project: string
  data: Record<string, unknown>
  ts: string
}

export type { Task, TaskTreeNode, TaskListFilter, TaskListResult } from './task'
export type { CreateTaskInput, UpdateTaskInput, ChangeStatusInput } from './task'
