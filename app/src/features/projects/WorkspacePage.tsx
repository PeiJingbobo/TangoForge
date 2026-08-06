import { Button } from '@/components/ui/button'
import { FileUp, FolderPlus } from 'lucide-react'

/**
 * 工作区（项目列表）—— TF-024 实现。
 * 骨架占位：展示 UI-VISION 的空态规范（标题→说明→操作按钮阅读流）。
 */
export function WorkspacePage() {
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center text-center">
      <div className="mb-6 grid size-16 place-items-center rounded-[20px] bg-primary-50 text-primary-600">
        <FileUp className="size-8" />
      </div>
      <h1 className="text-h1 text-foreground">这个工作区还是空的</h1>
      <p className="mt-3 max-w-md text-body text-muted-foreground">
        从 Markdown 文档导入任务，或创建空白项目开始。解析结果会先进入草稿，确认后才入库。
      </p>
      <div className="mt-8 flex items-center gap-3">
        <Button>
          <FileUp />从 Markdown 导入
        </Button>
        <Button variant="outline">
          <FolderPlus />
          创建空白项目
        </Button>
      </div>
    </div>
  )
}
