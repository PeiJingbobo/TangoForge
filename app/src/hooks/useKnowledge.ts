import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { qk } from '@/hooks/keys'
import { useProjectId } from '@/hooks/useProject'
import type {
  KnowledgeBase,
  KnowledgeDocument,
  KnowledgeDocumentContent,
  KnowledgeDocumentListResult,
  KnowledgeScanStats,
  KnowledgeSearchResult,
} from '@/types/models'

/** 知识库列表（含文档数） */
export function useKnowledgeBases(project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: [...qk.knowledge(pid ?? ''), 'bases'],
    queryFn: () => apiRequest<KnowledgeBase[]>('/api/knowledge/bases', { project: pid }),
    enabled: !!pid,
  })
}

/** 文档列表（kb/status/q 过滤；有 indexing 文档时轮询刷新以展示嵌入进度） */
export function useKnowledgeDocuments(
  filter?: { kb_id?: number; status?: string; q?: string },
  project?: string,
) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: [...qk.knowledge(pid ?? ''), 'documents', filter ?? {}],
    queryFn: () =>
      apiRequest<KnowledgeDocumentListResult>('/api/knowledge/documents', {
        project: pid,
        query: {
          size: 200,
          'filter[kb_id]': filter?.kb_id ?? undefined,
          'filter[status]': filter?.status,
          q: filter?.q,
        },
      }),
    enabled: !!pid,
    // TF-052：存在正在嵌入（indexing）文档时每 2s 轮询，展示嵌入完成进度。
    refetchInterval: (query) => {
      const hasIndexing = query.state.data?.items.some((d) => d.status === 'indexing')
      return hasIndexing ? 2000 : false
    },
  })
}

/** 文档详情 */
export function useKnowledgeDocument(id: string | undefined, project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: [...qk.knowledge(pid ?? ''), 'document', id ?? ''],
    queryFn: () =>
      apiRequest<KnowledgeDocument>(`/api/knowledge/documents/${id}`, { project: pid }),
    enabled: !!pid && !!id,
  })
}

/** 文档原文（阅读） */
export function useKnowledgeContent(id: string | undefined, project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: [...qk.knowledge(pid ?? ''), 'content', id ?? ''],
    queryFn: () =>
      apiRequest<KnowledgeDocumentContent>(`/api/knowledge/documents/${id}/content`, {
        project: pid,
      }),
    enabled: !!pid && !!id,
  })
}

/** 向量检索（未配置 embedding → EMBEDDING_NOT_CONFIGURED） */
export function useKnowledgeSearch(
  query: string,
  options?: { kb_id?: number; top_k?: number },
  project?: string,
) {
  const pid = useProjectId(project)
  const enabled = !!pid && query.trim().length > 0
  return useQuery({
    queryKey: [...qk.knowledge(pid ?? ''), 'search', query, options ?? {}],
    queryFn: () =>
      apiRequest<KnowledgeSearchResult>('/api/knowledge/search', {
        project: pid,
        query: { q: query, kb_id: options?.kb_id ?? undefined, top_k: options?.top_k ?? 10 },
      }),
    enabled,
    retry: false,
  })
}

/** 手动触发扫描 */
export function useKnowledgeScan(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      apiRequest<KnowledgeScanStats>('/api/knowledge/scan', {
        method: 'POST',
        project: pid,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.knowledge(pid ?? '') })
    },
  })
}

/** 创建库 */
export function useCreateKnowledgeBase(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { name: string; description?: string }) =>
      apiRequest<KnowledgeBase>('/api/knowledge/bases', { method: 'POST', project: pid, body }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.knowledge(pid ?? '') })
    },
  })
}

/** 更新库（重命名/描述） */
export function useUpdateKnowledgeBase(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { id: number; name?: string; description?: string }) =>
      apiRequest<KnowledgeBase>(`/api/knowledge/bases/${body.id}`, {
        method: 'PATCH',
        project: pid,
        body: { name: body.name, description: body.description },
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.knowledge(pid ?? '') })
    },
  })
}

/** 删除库（仅移除库↔文档边） */
export function useDeleteKnowledgeBase(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      apiRequest<{ id: number }>(`/api/knowledge/bases/${id}`, { method: 'DELETE', project: pid }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.knowledge(pid ?? '') })
    },
  })
}

/** 注册文档 */
export function useRegisterKnowledgeDocument(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { path: string; copy?: string; kb_ids?: number[] }) =>
      apiRequest<KnowledgeDocument>('/api/knowledge/documents', {
        method: 'POST',
        project: pid,
        body,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.knowledge(pid ?? '') })
    },
  })
}

/** 编辑文档原文（写盘 → 重索引） */
export function useEditKnowledgeContent(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { id: string; content: string }) =>
      apiRequest<{ id: string }>(`/api/knowledge/documents/${body.id}/content`, {
        method: 'PUT',
        project: pid,
        body: { content: body.content },
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.knowledge(pid ?? '') })
      void qc.invalidateQueries({ queryKey: ['tasks', pid ?? ''] })
    },
  })
}

/** 重新链接（relink） */
export function useRelinkKnowledgeDocument(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { id: string; new_path: string; copy?: string }) =>
      apiRequest<KnowledgeDocument>(`/api/knowledge/documents/${body.id}/relink`, {
        method: 'POST',
        project: pid,
        body: { new_path: body.new_path, copy: body.copy },
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.knowledge(pid ?? '') })
    },
  })
}

/** 删除文档（解除引用） */
export function useDeleteKnowledgeDocument(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      apiRequest<{ id: string }>(`/api/knowledge/documents/${id}`, {
        method: 'DELETE',
        project: pid,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.knowledge(pid ?? '') })
      void qc.invalidateQueries({ queryKey: ['tasks', pid ?? ''] })
    },
  })
}

/** 任务关联文档（添加/移除） */
export function useLinkTaskDocument(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { task_id: string; document_id?: string; path?: string; copy?: string }) =>
      apiRequest<{ task_id: string }>('/api/knowledge/link', {
        method: 'POST',
        project: pid,
        body,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks', pid ?? ''] })
      void qc.invalidateQueries({ queryKey: qk.knowledge(pid ?? '') })
    },
  })
}

/** 解除任务关联 */
export function useUnlinkTaskDocument(project?: string) {
  const pid = useProjectId(project)
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { task_id: string; document_id: string }) =>
      apiRequest<{ ok: boolean }>('/api/knowledge/unlink', {
        method: 'POST',
        project: pid,
        body,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks', pid ?? ''] })
      void qc.invalidateQueries({ queryKey: qk.knowledge(pid ?? '') })
    },
  })
}

/** 任务关联的文档列表 */
export function useTaskDocuments(taskId: string | undefined, project?: string) {
  const pid = useProjectId(project)
  return useQuery({
    queryKey: [...qk.knowledge(pid ?? ''), 'task-docs', taskId ?? ''],
    queryFn: () =>
      apiRequest<KnowledgeDocument[]>(`/api/knowledge/tasks/${taskId}`, { project: pid }),
    enabled: !!pid && !!taskId,
  })
}
