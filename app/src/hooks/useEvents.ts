import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { subscribeEvents } from '@/api/ws'
import { useProjectId } from '@/hooks/useProject'
import type { WSEvent } from '@/types/models'

/**
 * WS 事件 → Query 失效映射（docs/TECHNICAL.md §4.3）：
 * - task.*            → 任务列表 / 单任务 / 图
 * - state_machine.*   → 状态机
 * - import.*          → 草稿列表
 * - knowledge 域      → 知识库查询（后端事件 type 无 knowledge. 前缀：
 *   queue_updated / document_added / document_removed / document_relinked /
 *   document_content_edited / document_archived / document_restored /
 *   kb_created / kb_updated / kb_deleted / task_linked / task_unlinked /
 *   index_failed；TF-053 修复：之前按 knowledge. 前缀匹配导致永不命中）。
 * 事件连接由主进程持有（渲染进程仅订阅）；项目切换（pid 变化）自动重订阅。
 */

// 后端 knowledge 域 WS 事件 type（internal/knowledge/*.go fireWrite / queue.go fire，
// 经 hub.Publish(workdir, action) 原样透传，无前缀）。改动后端事件名须同步本集合。
const KNOWLEDGE_EVENTS = new Set([
  'queue_updated',
  'document_added',
  'document_removed',
  'document_relinked',
  'document_content_edited',
  'document_archived',
  'document_restored',
  'document_updated',
  'kb_created',
  'kb_updated',
  'kb_deleted',
  'task_linked',
  'task_unlinked',
  'index_failed',
])

export function useEventInvalidator(project?: string): void {
  const pid = useProjectId(project)
  const qc = useQueryClient()

  useEffect(() => {
    if (!pid) return

    const handleEvent = (e: WSEvent) => {
      const id = typeof e.data?.id === 'string' ? e.data.id : undefined
      if (e.type.startsWith('task.')) {
        qc.invalidateQueries({ queryKey: ['tasks', pid] })
        qc.invalidateQueries({ queryKey: ['graph', pid] })
        if (id) qc.invalidateQueries({ queryKey: ['tasks', pid, id] })
      } else if (e.type === 'state_machine.changed') {
        qc.invalidateQueries({ queryKey: ['state-machine', pid] })
        qc.invalidateQueries({ queryKey: ['tasks', pid] })
      } else if (e.type.startsWith('import.')) {
        qc.invalidateQueries({ queryKey: ['drafts', pid] })
      } else if (e.type.startsWith('knowledge.') || KNOWLEDGE_EVENTS.has(e.type)) {
        qc.invalidateQueries({ queryKey: ['knowledge', pid] })
        // 任务详情内嵌文档摘要 → 任务查询也失效。
        qc.invalidateQueries({ queryKey: ['tasks', pid] })
      }
    }

    return subscribeEvents(pid, handleEvent)
  }, [pid, qc])
}
