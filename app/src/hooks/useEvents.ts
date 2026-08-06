import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { connectEvents } from '@/api/ws'
import { useProjectId } from '@/hooks/useProject'
import type { WSEvent } from '@/types/models'

/**
 * WS 事件 → Query 失效映射（docs/TECHNICAL.md §4.3）：
 * - task.*            → 任务列表 / 单任务 / 图
 * - state_machine.*   → 状态机
 * - import.*          → 草稿列表
 * 项目切换（pid 变化）自动断开旧连接、建立新连接。
 */
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
      }
    }

    return connectEvents({ project: pid, onEvent: handleEvent })
  }, [pid, qc])
}
