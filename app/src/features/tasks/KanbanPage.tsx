import { Badge } from '@/components/ui/badge'

/**
 * 看板视图 —— TF-025 实现（状态机动态列、拖拽流转、虚拟滚动）。
 * 骨架占位。
 */
export function KanbanPage() {
  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <p className="text-caption uppercase tracking-[0.09em] text-muted-foreground">
            项目 / 看板
          </p>
          <h1 className="text-h2 text-foreground">任务看板</h1>
        </div>
        <Badge variant="outline">TF-025 实现</Badge>
      </div>
      <div className="rounded-[14px] border border-dashed border-border p-12 text-center text-body text-muted-foreground">
        看板视图骨架占位：按项目状态机动态生成列、任务卡片拖拽流转、虚拟滚动将在 TF-025 实现。
      </div>
    </div>
  )
}
