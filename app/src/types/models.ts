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
  /** 暂时隐藏（TF-043）：引导未完成的项目不在列表展示 */
  hidden?: boolean
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
  /** 简短任务编号（TF-040）：文档自带编号（如 P0），无则空 → 入库自动分配 */
  number?: string
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
  'skill.install',
  'state_machine.read',
  'state_machine.write',
  'audit.read',
  'permission.read',
] as const
export type ActionKey = (typeof ACTION_KEYS)[number]
export type PermissionMap = Record<ActionKey, boolean>

/** 权限动作中文 label（TF-036 权限界面中文化，与 ACTION_KEYS 一一对应） */
export const ACTION_LABELS: Record<ActionKey, string> = {
  'project.read': '查看项目',
  'task.read': '查看任务',
  'task.create': '创建任务',
  'task.update': '更新任务',
  'task.update_status': '流转任务状态',
  'task.delete': '归档任务',
  'task.restore': '还原任务',
  'import.run': '发起导入',
  'import.confirm': '确认导入',
  'export.run': '导出任务',
  'graph.read': '查看全景图',
  'skill.read': '查看技能包',
  'skill.install': '安装技能包',
  'state_machine.read': '查看状态机',
  'state_machine.write': '编辑状态机',
  'audit.read': '查看审计日志',
  'permission.read': '查看权限',
}

/** 权限域中文标题（分组展示用） */
export const ACTION_DOMAIN_LABELS: Record<string, string> = {
  project: '项目',
  task: '任务',
  import: '导入',
  export: '导出',
  graph: '全景图',
  skill: '技能',
  state_machine: '状态机',
  audit: '审计',
  permission: '权限',
}

/* ---------- audit（审计 action 中文化，TF-042） ---------- */

/** 审计动作中文 label（与后端审计 action 全集对应；未知 action 原样回退） */
export const AUDIT_ACTION_LABELS: Record<string, string> = {
  'task.created': '创建任务',
  'task.updated': '更新任务',
  'task.status_changed': '状态流转',
  'task.archived': '归档任务',
  'task.restored': '还原任务',
  'task.deleted': '删除任务',
  'import.draft_ready': '草稿解析',
  'import.draft_confirmed': '导入确认',
  'import.draft_discarded': '丢弃草稿',
  'import.failed': '导入失败',
  'state_machine.changed': '状态机修改',
  'config.updated': '全局配置更新',
  'project_config.updated': '项目配置更新',
  'permission.changed': '权限变更',
  'project.imported': '项目导入',
  'project.renamed': '项目重命名',
  'skill.installed': '技能安装',
  'skill.updated': '技能更新',
  'skill.uninstalled': '技能卸载',
  'skill.package_written': '技能包写入',
  'skill.template_written': '技能模板写入',
  'export.complete': '导出完成',
}

/** 审计动作 → 中文 label（未知 action 原样返回，不丢失信息） */
export const auditActionLabel = (action: string): string => AUDIT_ACTION_LABELS[action] ?? action

/* ---------- skills（TF-033 重设计：技能包模型 + 宿主安装） ---------- */

/** 技能包（SKILL.md，内置 embed + 全局技能库 ~/.taskboard-app/skills/） */
export interface SkillPackage {
  name: string
  version: string
  description: string
  hosts: string[]
  when_to_use: string
  instructions: string
  content: string
  source: 'builtin' | 'user'
  updated_at: string
}

/** 宿主安装结果（单包） */
export interface SkillInstallResult {
  name: string
  host: string
  action: 'install' | 'update' | 'uninstall'
  version: string
  ok: boolean
  error?: string
}

/** 某宿主下单个技能包安装状态 */
export interface InstalledSkill {
  name: string
  version: string
  state: 'missing' | 'current' | 'stale'
}

/** 宿主安装状态 */
export interface HostStatus {
  key: string
  label: string
  scope: 'project' | 'user'
  installed: InstalledSkill[]
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
