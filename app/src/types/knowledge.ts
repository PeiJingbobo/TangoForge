/**
 * 知识库 DTO（与 internal/knowledge 模型字段级同步，docs/KNOWLEDGE-BASE.md §3）。
 * 端点：/api/knowledge/*（X-Project 必带；权限 knowledge.read/write/index）。
 */

/* ---------- 命名库（knowledge_bases） ---------- */
export interface KnowledgeBase {
  id: number
  project_id: number
  name: string
  description: string
  is_default: boolean
  created_at: string
  updated_at: string
  /** 库内文档数（列表联表填充） */
  doc_count?: number
}

/* ---------- 文档引用（knowledge_documents） ---------- */
export type KnowledgeDocumentStatus = 'ok' | 'missing' | 'indexing' | 'failed'

export interface KnowledgeDocument {
  id: string
  project_id: number
  /** 引用路径（相对 workdir 或绝对，原始形态） */
  path: string
  /** 规范化绝对路径（唯一键） */
  abs_path: string
  /** 相对 workdir 路径（项目内时） */
  rel_path: string
  /** 外部文件拷贝前的原始绝对路径 */
  origin_path: string
  display_name: string
  type: 'text' | 'binary'
  size: number
  mtime: string
  content_hash: string
  /** LLM 摘要缓存 */
  summary: string
  status: KnowledgeDocumentStatus
  /** 0 未嵌入 / 1 已嵌入 / 2 失败 */
  embedded: number
  embedding_model: string
  index_error: string
  /** relink 历史：[{path, relinked_at}] */
  history: { path: string; relinked_at: string }[]
  /** 归档标记（TF-052）：归档后从默认列表/检索隐藏，任务引用与文件保留 */
  archived: boolean
  created_at: string
  updated_at: string
  /** 详情扩展：关联任务数 / 所属库列表 */
  task_count?: number
  kb_ids?: number[]
}

/** 文档原文内容（GET/PUT /documents/:id/content） */
export interface KnowledgeDocumentContent {
  type: 'text' | 'binary'
  content?: string
  path: string
}

/* ---------- 任务详情内嵌摘要（/api/tasks/:id） ---------- */
export interface TaskKnowledgeSummary {
  id: string
  display_name: string
  path: string
  abs_path: string
  rel_path: string
  type: 'text' | 'binary'
  status: KnowledgeDocumentStatus
  summary: string
}

/* ---------- 向量检索（GET /api/knowledge/search） ---------- */
export interface KnowledgeSnippet {
  heading: string
  text: string
  score: number
  seq: number
}

export interface KnowledgeSearchHit {
  document: KnowledgeDocument
  score: number
  chunks: KnowledgeSnippet[]
  missing: boolean
}

export interface KnowledgeSearchResult {
  query: string
  total: number
  items: KnowledgeSearchHit[]
}

/* ---------- 扫描（POST /api/knowledge/scan） ---------- */
export interface KnowledgeScanStats {
  total: number
  indexed: number
  skipped: number
  failed: number
  missing: number
}

/* ---------- 文档列表（GET /api/knowledge/documents） ---------- */
export interface KnowledgeDocumentListResult {
  items: KnowledgeDocument[]
  total: number
  page: number
  size: number
}

/* ---------- 导入草稿流 knowledge_files（parser 透传） ---------- */
/** LLM 建议的应引用知识库文件（path 必填；kb 为库名或省略=默认库） */
export interface KnowledgeFileSuggestion {
  path: string
  kb?: string
  reason?: string
}
