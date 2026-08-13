import { useState } from 'react'
import { toast } from 'sonner'
import { FileUp, FolderOpen, Loader2, X } from 'lucide-react'
import { ApiError } from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useKnowledgeBases, useRegisterKnowledgeDocument } from '@/hooks/useKnowledge'
import type { KnowledgeBase } from '@/types/models'

/**
 * 知识库「添加文件」对话框（TF-053 体验补全）：
 * 系统文件选择器多选（Electron）/ 手动路径兜底；可选目标库与拷贝语义。
 * 逐文件注册（后端 abs_path 唯一复用；外部文件按 copy 语义处理），成功 → 触发索引。
 */
export function KnowledgeAddDialog({
  open,
  onOpenChange,
  project,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  project: string
}) {
  const { data: bases, isLoading: basesLoading } = useKnowledgeBases(project)
  const register = useRegisterKnowledgeDocument(project)
  const [paths, setPaths] = useState<string[]>([])
  const [manualPath, setManualPath] = useState('')
  const [kbId, setKbId] = useState<number>(0) // 0 = 默认库
  const [copy, setCopy] = useState('auto')
  const [busy, setBusy] = useState(false)

  const hasDialog = Boolean(window.tangoforge?.dialog)

  const pickFiles = async () => {
    if (!window.tangoforge?.dialog) {
      toast.info('当前为非桌面环境，请手动输入路径')
      return
    }
    const files = await window.tangoforge.dialog.selectFiles(project || undefined)
    if (files && files.length > 0) setPaths((prev) => [...new Set([...prev, ...files])])
  }

  const pickDirectory = async () => {
    if (!window.tangoforge?.dialog) {
      toast.info('当前为非桌面环境，请手动输入路径')
      return
    }
    const dir = await window.tangoforge.dialog.selectDirectory(project || undefined)
    if (dir) setPaths((prev) => [...new Set([...prev, dir])])
  }

  const addManualPath = () => {
    const p = manualPath.trim()
    if (!p) return
    setPaths((prev) => [...new Set([...prev, p])])
    setManualPath('')
  }

  const removePath = (p: string) => setPaths((prev) => prev.filter((x) => x !== p))

  const submit = () => {
    if (paths.length === 0) {
      toast.info('请先选择或填写文件路径')
      return
    }
    setBusy(true)
    // 逐文件注册（后端 abs_path 唯一复用；合并结果提示）。
    let done = 0
    let failed = 0
    const maybeFinish = (count: number) => {
      if (count < paths.length) return
      setBusy(false)
      if (failed === 0) {
        toast.success(`已添加 ${done} 个文件到知识库（将自动索引）`)
        setPaths([])
        onOpenChange(false)
      } else if (done > 0) {
        toast.warning(`已添加 ${done} 个，${failed} 个失败`)
        setPaths([])
        onOpenChange(false)
      }
    }
    paths.forEach((p, i) => {
      register.mutate(
        { path: p, copy, kb_ids: kbId > 0 ? [kbId] : undefined },
        {
          onSuccess: () => {
            done++
            maybeFinish(done + failed)
          },
          onError: (e) => {
            failed++
            const msg =
              e instanceof ApiError ? e.message : e instanceof Error ? e.message : '注册失败'
            if (i === paths.length - 1) toast.error(`「${p}」注册失败：${msg}`)
            maybeFinish(done + failed)
          },
        },
      )
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>添加文件到知识库</DialogTitle>
          <DialogDescription>
            选择本地文件或目录注册进知识库（已注册的路径自动复用）；注册后自动索引。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {/* 文件选择 */}
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={pickFiles} disabled={!hasDialog}>
              <FileUp className="size-3.5" /> 选择文件
            </Button>
            <Button variant="outline" size="sm" onClick={pickDirectory} disabled={!hasDialog}>
              <FolderOpen className="size-3.5" /> 选择目录
            </Button>
            {!hasDialog && (
              <span className="self-center text-xs text-muted-foreground">
                非桌面环境，请手动填写路径
              </span>
            )}
          </div>

          {/* 手动路径 */}
          <div className="flex items-center gap-2">
            <Input
              value={manualPath}
              onChange={(e) => setManualPath(e.target.value)}
              placeholder="磁盘路径（绝对或相对项目目录）"
              className="flex-1 font-mono text-xs"
              onKeyDown={(e) => {
                if (e.key === 'Enter') addManualPath()
              }}
            />
            <Button size="sm" onClick={addManualPath} disabled={!manualPath.trim()}>
              添加
            </Button>
          </div>

          {/* 已选路径列表 */}
          {paths.length > 0 && (
            <div className="max-h-44 space-y-1 overflow-y-auto rounded-xl border border-divider p-2">
              {paths.map((p) => (
                <div
                  key={p}
                  className="flex items-center gap-2 rounded-lg px-2 py-1 hover:bg-accent"
                >
                  <span className="min-w-0 flex-1 truncate font-mono text-xs">{p}</span>
                  <button
                    type="button"
                    onClick={() => removePath(p)}
                    aria-label={`移除 ${p}`}
                    className="grid size-6 shrink-0 place-items-center rounded-full text-muted-foreground hover:text-destructive-ink"
                  >
                    <X className="size-3.5" />
                  </button>
                </div>
              ))}
            </div>
          )}

          {/* 目标库 + 拷贝语义 */}
          <div className="flex flex-wrap items-center gap-4">
            <div className="min-w-40">
              <Label>目标知识库</Label>
              {basesLoading ? (
                <Skeleton className="mt-1 h-8 w-40" />
              ) : (
                <select
                  value={kbId}
                  onChange={(e) => setKbId(Number(e.target.value))}
                  className="mt-1 h-8 w-full rounded-lg border border-border bg-background px-2 text-xs"
                >
                  <option value={0}>默认库</option>
                  {(bases ?? []).map((b: KnowledgeBase) => (
                    <option key={b.id} value={b.id}>
                      {b.name}
                      {b.is_default ? '（默认）' : ''}
                    </option>
                  ))}
                </select>
              )}
            </div>
            <div>
              <Label>外部文件</Label>
              <div className="mt-1 flex flex-wrap gap-1.5">
                {[
                  { value: 'auto', label: 'auto' },
                  { value: 'copy', label: 'copy' },
                  { value: 'none', label: 'none' },
                ].map((c) => (
                  <Badge
                    key={c.value}
                    role="button"
                    variant={copy === c.value ? 'default' : 'outline'}
                    className="cursor-pointer px-2 py-1"
                    onClick={() => setCopy(c.value)}
                  >
                    {c.label}
                  </Badge>
                ))}
              </div>
            </div>
          </div>
          <p className="text-xs text-muted-foreground">
            auto：项目内文件原样引用，项目外文件自动拷贝进默认文档目录。
          </p>

          {/* 提交 */}
          <div className="flex justify-end gap-2 border-t border-divider pt-3">
            <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)} disabled={busy}>
              取消
            </Button>
            <Button
              size="sm"
              onClick={submit}
              disabled={paths.length === 0 || busy}
              aria-label="提交添加"
            >
              {busy ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <FileUp className="size-3.5" />
              )}
              添加 {paths.length > 0 ? `（${paths.length}）` : ''}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
