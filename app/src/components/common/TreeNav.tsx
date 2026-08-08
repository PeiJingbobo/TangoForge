import { useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'
import { matchesTaskQuery } from '@/lib/task-search'
import type { TaskTreeNode } from '@/types/task'
import { TaskNumberBadge } from '@/components/common/TaskNumberBadge'

/**
 * 树形任务导航（TF-026）：折叠/展开、缩进、搜索过滤、当前任务高亮。
 * 数据为后端树结构（与后端树一致，验收项）。
 */
export interface TreeNavProps {
  tree: TaskTreeNode[]
  currentId?: string
  onSelect: (id: string) => void
}

export function TreeNav({ tree, currentId, onSelect }: TreeNavProps) {
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  const [query, setQuery] = useState('')

  const matches = (node: TaskTreeNode): boolean => {
    // 同时匹配编号 / 标题 / 内容（TF-042）
    return matchesTaskQuery(node, query)
  }

  const filterTree = (nodes: TaskTreeNode[]): TaskTreeNode[] => {
    if (!query) return nodes
    return nodes
      .map((n) => ({ ...n, children: filterTree(n.children) }))
      .filter((n) => matches(n) || n.children.length > 0)
  }

  const visible = filterTree(tree)

  const toggle = (id: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const renderNode = (node: TaskTreeNode, depth: number) => {
    const hasChildren = node.children.length > 0
    const isCollapsed = collapsed.has(node.id)
    const isCurrent = node.id === currentId
    const isMatched = matches(node)

    return (
      <div key={node.id}>
        <div
          role="button"
          tabIndex={0}
          aria-label={`任务 ${node.title}`}
          onClick={() => onSelect(node.id)}
          onKeyDown={(e) => e.key === 'Enter' && onSelect(node.id)}
          className={cn(
            'group flex cursor-pointer items-center gap-1.5 rounded-lg py-1.5 pr-2 text-sm transition-colors hover:bg-accent',
            isCurrent && 'bg-primary-50 font-semibold text-primary-700',
            !isMatched && 'text-muted-foreground',
          )}
          style={{ paddingLeft: `${8 + depth * 16}px` }}
        >
          {hasChildren ? (
            <button
              type="button"
              aria-label={isCollapsed ? '展开' : '折叠'}
              onClick={(e) => {
                e.stopPropagation()
                toggle(node.id)
              }}
              className="shrink-0 rounded p-0.5 hover:bg-muted"
            >
              <ChevronRight
                className={cn(
                  'size-3.5 text-muted-foreground transition-transform',
                  !isCollapsed && 'rotate-90',
                )}
              />
            </button>
          ) : (
            <span className="w-4 shrink-0" />
          )}
          <span className="flex min-w-0 items-center gap-1.5">
            <TaskNumberBadge number={node.number} />
            <span className="truncate">{node.title}</span>
          </span>
          {hasChildren && (
            <span className="ml-auto text-caption text-muted-foreground">
              {node.children.length}
            </span>
          )}
        </div>
        {hasChildren && !isCollapsed && (
          <div>{node.children.map((c) => renderNode(c, depth + 1))}</div>
        )}
      </div>
    )
  }

  return (
    // 搜索框固定顶部，仅树形列表内部滚动（TF-042）
    <div className="flex h-full min-h-0 flex-col">
      <input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="搜索编号 / 标题 / 内容…"
        aria-label="搜索任务树"
        className="mb-3 w-full shrink-0 rounded-full border border-input bg-card px-3.5 py-1.5 text-sm outline-none placeholder:text-muted-foreground focus:border-primary-400 focus:ring-[3px] focus:ring-primary-100"
      />
      <div className="min-h-0 flex-1 space-y-0.5 overflow-y-auto pr-1">
        {visible.length === 0 && (
          <p className="px-2 py-4 text-center text-sm text-muted-foreground">
            {query ? '无匹配任务' : '没有任务'}
          </p>
        )}
        {visible.map((n) => renderNode(n, 0))}
      </div>
    </div>
  )
}
