import { useRef, useState } from 'react'
import { toast } from 'sonner'
import { Database, Info } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  useConfig,
  useUpdateConfig,
  isConfigInvalid,
  type GlobalConfigView,
} from '@/hooks/useConfig'

/**
 * 知识库全局配置（TF-052，docs/KNOWLEDGE-BASE.md §4.1）：
 * - 总开关 / 实时监听 / 启动扫描 / 防抖窗口 / 嵌入并发 / 索引大小上限 / 默认文档目录；
 * - llm.embedding（模型/协议/超时）+ 向量搜索开关（QA-K23：未配置 Embedding 模型时强制禁用并置灰）；
 * - 实时保存（500ms 防抖），后端校验失败回滚。
 */
export function KnowledgeSection() {
  const { data } = useConfig()
  const updateConfig = useUpdateConfig()
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [draft, setDraft] = useState<GlobalConfigView | null>(null)

  if (!data) return <p className="text-sm text-muted-foreground">配置加载失败。</p>
  const saved = draft ?? data
  const k = saved.knowledge
  const emb = saved.llm.embedding
  // QA-K23：未配置 Embedding 模型 → 向量搜索强制禁用（置灰并提示）。
  const embedConfigured = emb.model.trim().length > 0
  const vectorSearchLocked = !embedConfigured

  const patch = (next: GlobalConfigView) => {
    setDraft(next)
    if (saveTimer.current) clearTimeout(saveTimer.current)
    saveTimer.current = setTimeout(() => {
      updateConfig.mutate(
        {
          llm: { embedding: next.llm.embedding },
          knowledge: next.knowledge,
        },
        {
          onSuccess: () => toast.success('知识库设置已保存'),
          onError: (err) => {
            if (isConfigInvalid(err)) {
              toast.error(err instanceof Error ? err.message : '配置无效', {
                description: '已回滚',
              })
              setDraft(null)
            } else {
              toast.error(err instanceof Error ? err.message : '保存失败')
            }
          },
        },
      )
    }, 500)
  }

  const setK = (field: keyof typeof k, value: unknown) => {
    patch({ ...saved, knowledge: { ...k, [field]: value } })
  }
  const setEmb = (field: keyof typeof emb, value: unknown) => {
    patch({ ...saved, llm: { ...saved.llm, embedding: { ...emb, [field]: value } } })
  }

  return (
    <div className="max-w-2xl space-y-5">
      {/* 总开关 */}
      <div className="rounded-2xl border border-border bg-card p-4">
        <div className="flex items-center justify-between gap-3">
          <div>
            <div className="flex items-center gap-1.5 text-sm font-medium">
              <Database className="size-4 text-muted-foreground" />
              启用知识库
            </div>
            <p className="mt-0.5 text-xs text-muted-foreground">
              关闭后不扫描/不监听/不索引；已索引数据与查询仍可用。
            </p>
          </div>
          <Switch checked={k.enabled} onCheckedChange={(v) => setK('enabled', v)} />
        </div>
      </div>

      {/* 自动索引 */}
      <div className="space-y-4 rounded-2xl border border-border bg-card p-4">
        <div className="text-sm font-medium">自动索引</div>
        <div className="flex items-center justify-between gap-3">
          <div>
            <div className="text-sm">实时监听文件变化</div>
            <p className="text-xs text-muted-foreground">fsnotify 监听已注册文档父目录</p>
          </div>
          <Switch checked={k.fsnotify} onCheckedChange={(v) => setK('fsnotify', v)} />
        </div>
        <div className="flex items-center justify-between gap-3">
          <div>
            <div className="text-sm">启动时全量扫描</div>
            <p className="text-xs text-muted-foreground">守护进程启动时重新索引</p>
          </div>
          <Switch checked={k.startup_scan} onCheckedChange={(v) => setK('startup_scan', v)} />
        </div>
        <div>
          <Label htmlFor="cfg-kb-debounce">防抖窗口（毫秒）</Label>
          <Input
            id="cfg-kb-debounce"
            type="number"
            value={k.debounce_ms}
            onChange={(e) => setK('debounce_ms', Number(e.target.value))}
            className="mt-1.5"
          />
        </div>
        <div>
          <Label htmlFor="cfg-kb-conc">嵌入并发数</Label>
          <Input
            id="cfg-kb-conc"
            type="number"
            value={k.embed_concurrency}
            onChange={(e) => setK('embed_concurrency', Number(e.target.value))}
            className="mt-1.5"
          />
        </div>
        <div>
          <Label htmlFor="cfg-kb-max-size">索引大小上限（字节）</Label>
          <Input
            id="cfg-kb-max-size"
            type="number"
            value={k.max_index_size}
            onChange={(e) => setK('max_index_size', Number(e.target.value))}
            className="mt-1.5"
          />
          <p className="mt-1 text-xs text-muted-foreground">
            超过该大小的文件仅注册 + 摘要，不嵌入向量（默认 512KB）。
          </p>
        </div>
        <div>
          <Label htmlFor="cfg-kb-docdir">默认文档目录（外部文件拷贝落点）</Label>
          <Input
            id="cfg-kb-docdir"
            value={k.default_doc_dir}
            onChange={(e) => setK('default_doc_dir', e.target.value)}
            placeholder=".taskboard/knowledge（相对 workdir）"
            className="mt-1.5 font-mono text-sm"
          />
        </div>
      </div>

      {/* Embedding + 向量搜索 */}
      <div className="space-y-4 rounded-2xl border border-border bg-card p-4">
        <div className="text-sm font-medium">向量嵌入（llm.embedding）</div>
        <div>
          <Label htmlFor="cfg-emb-model">Embedding 模型</Label>
          <Input
            id="cfg-emb-model"
            value={emb.model}
            onChange={(e) => setEmb('model', e.target.value)}
            placeholder="nomic-embed-text / text-embedding-3-small（留空 = 禁用向量）"
            className="mt-1.5 text-sm"
          />
        </div>
        <div>
          <Label>协议类型</Label>
          <div className="mt-1.5 flex flex-wrap gap-1.5">
            {[
              { value: 'openai', label: 'openai' },
              { value: 'ollama', label: 'ollama' },
            ].map((p) => (
              <Badge
                key={p.value}
                role="button"
                variant={emb.api_kind === p.value ? 'default' : 'outline'}
                className="cursor-pointer px-3 py-1.5"
                onClick={() => setEmb('api_kind', p.value)}
              >
                {p.label}
              </Badge>
            ))}
          </div>
        </div>
        <div>
          <Label htmlFor="cfg-emb-base">Embedding 接口地址</Label>
          <Input
            id="cfg-emb-base"
            value={emb.base_url}
            onChange={(e) => setEmb('base_url', e.target.value)}
            placeholder="留空 = 复用 LLM base_url"
            className="mt-1.5 font-mono text-sm"
          />
        </div>
        <div>
          <Label htmlFor="cfg-emb-key">Embedding API Key</Label>
          <Input
            id="cfg-emb-key"
            type="password"
            value={emb.api_key}
            onChange={(e) => setEmb('api_key', e.target.value)}
            placeholder="留空 = 复用 LLM api_key"
            className="mt-1.5"
          />
        </div>
        <div>
          <Label htmlFor="cfg-emb-timeout">请求超时（秒）</Label>
          <Input
            id="cfg-emb-timeout"
            type="number"
            value={emb.timeout_sec}
            onChange={(e) => setEmb('timeout_sec', Number(e.target.value))}
            className="mt-1.5"
          />
        </div>

        <div className="flex items-start justify-between gap-3 border-t border-divider pt-4">
          <div className="min-w-0">
            <div className="flex items-center gap-1.5 text-sm font-medium">
              向量搜索
              {vectorSearchLocked && (
                <Badge variant="outline" className="text-[10px]">
                  需配置 Embedding 模型
                </Badge>
              )}
            </div>
            <p className="mt-0.5 text-xs text-muted-foreground">
              检索 top_k={k.search_top_k} · 阈值 {k.search_threshold}（未配置模型时强制禁用）
            </p>
          </div>
          <Switch
            checked={vectorSearchLocked ? false : k.vector_search}
            disabled={vectorSearchLocked}
            onCheckedChange={(v) => setK('vector_search', v)}
          />
        </div>

        {vectorSearchLocked && (
          <div className="flex items-start gap-2 rounded-xl bg-warning-soft p-3 text-xs text-warning-ink">
            <Info className="mt-0.5 size-4 shrink-0" />
            未配置 llm.embedding 模型（qa-k23）。配置 Embedding 模型后向量搜索自动可用。
          </div>
        )}
      </div>
    </div>
  )
}
