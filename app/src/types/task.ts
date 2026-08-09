/**
 * 任务域 DTO（与后端 internal/task 字段级同步，docs/TASK-SEMANTICS.md）。
 *
 * 同步约定（AGENTS.md DoD）：后端 Task 结构体 / CreateInput / UpdateInput /
 * ListFilter / ListResult 变更时，本文件必须同步更新。
 * 语义要点：
 *  - project_id 固定为 1（项目库内文档性冗余）；
 *  - priority 接受 number 0-5 或字符串别名（low/normal/high/urgent 等），后端归一化为 0-5；
 *  - Update 禁止携带 status（状态更新走独立 ChangeStatus 接口）；
 *  - parent_id 三重态：undefined=不改 / null=置为顶层 / string=改父。
 */

/** 任务实体（对应后端 Task）。 */
export interface Task {
  /** UUID v4，服务端生成 */
  id: string
  /** 固定 1（项目库内冗余，勿依赖） */
  project_id: number
  /** 父任务 ID；null = 顶层任务 */
  parent_id: string | null
  title: string
  /** 简短唯一编号（TF-040，如 T01；创建/导入自动分配，文档编号可沿用） */
  number: string
  description: string
  /** 项目状态机 key（todo/doing/done/archived…） */
  status: string
  /** 0-5，0=无优先级/最低，5=最高 */
  priority: number
  tags: string[]
  /** 自由文本指派，可为空串 */
  assignee: string
  /** 被依赖任务 ID 数组 */
  depends_on: string[]
  /** 树形层级深度（前端 flattenTree 计算注入；0 = 顶层任务，QA 2026-08-09 看板缩进） */
  level?: number
  /** 归档前状态（归档/还原专用） */
  archived_from: string
  /** LLM 导入映射，内部字段 */
  source_file: string
  source_section: string
  /** RFC3339 */
  created_at: string
  updated_at: string
}

/** 树形节点：Task 字段平铺 + children（List 非分页模式返回）。 */
export type TaskTreeNode = Task & {
  children: TaskTreeNode[]
}

/** 创建任务入参（对应后端 CreateInput）。 */
export interface CreateTaskInput {
  /** null/缺省 = 顶层任务 */
  parent_id?: string | null
  /** 必填，去空白后非空 */
  title: string
  /** 简短编号（TF-040）；缺省 = 后端自动分配 T{n} */
  number?: string
  description?: string
  /** 缺省 = todo；必须存在于项目状态机 */
  status?: string
  /** number 0-5 或字符串别名（后端归一化）；缺省 = 0 */
  priority?: number | string
  tags?: string[]
  assignee?: string
  /** TF-005 仅存储，环校验 TF-008 */
  depends_on?: string[]
}

/** 更新任务入参（对应后端 UpdateInput；禁止携带 status）。 */
export interface UpdateTaskInput {
  title?: string
  description?: string
  priority?: number | string
  /** undefined = 不改；[] = 清空 */
  tags?: string[]
  assignee?: string
  /** undefined = 不改；[] = 清空 */
  depends_on?: string[]
  /** 三重态：undefined = 不改 / null = 置为顶层 / string = 改父 */
  parent_id?: string | null
}

/** 列表过滤/分页参数（对应后端 ListFilter）。 */
export interface TaskListFilter {
  /** 单值状态过滤；缺省 = 排除 archived；"archived" = 仅归档 */
  status?: string
  /** 匹配 title/description（大小写不敏感） */
  q?: string
  /** 0 = 返回全量任务树；>0 = 扁平分页（page 从 1 起） */
  page?: number
  /** 分页大小，默认 100，上限 500 */
  size?: number
}

/** 列表返回（对应后端 ListResult；树形或扁平分页二选一）。 */
export interface TaskListResult {
  /** 非分页模式：全量任务树 */
  tree?: TaskTreeNode[]
  /** 分页模式：扁平 items */
  items?: Task[]
  total: number
  page: number
  size: number
}

/** 状态变更入参（独立接口，Q8）。 */
export interface ChangeStatusInput {
  status: string
}
