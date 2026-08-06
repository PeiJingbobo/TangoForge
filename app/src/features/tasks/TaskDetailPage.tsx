import { useParams } from 'react-router'
import { Badge } from '@/components/ui/badge'

/**
 * 任务详情 —— TF-026 实现（标题→详情→操作按钮阅读流、依赖、子任务）。
 * 骨架占位。
 */
export function TaskDetailPage() {
  const { taskId } = useParams()
  return (
    <div>
      <p className="text-caption uppercase tracking-[0.09em] text-muted-foreground">
        项目 / 任务详情
      </p>
      <h1 className="text-h2 text-foreground">任务 #{taskId}</h1>
      <div className="mt-6 rounded-[14px] border border-dashed border-border p-12 text-center text-body text-muted-foreground">
        任务详情骨架占位：阅读流（标题 → 详情 → 操作按钮）、属性栏、子任务与依赖将在 TF-026 实现。
        <div className="mt-4">
          <Badge variant="outline">TF-026 实现</Badge>
        </div>
      </div>
    </div>
  )
}
