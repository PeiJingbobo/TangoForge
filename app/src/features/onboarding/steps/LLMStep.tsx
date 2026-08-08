import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { CheckCircle2, Loader2, PlugZap, Sparkles } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { LLM_PROVIDERS, providerByKey } from '@/features/onboarding/llm-providers'
import { useConfig, useTestLLM, useUpdateConfig } from '@/hooks/useConfig'

/**
 * 引导 Step 1：LLM 模型接入（TF-041）。
 * 平台快捷选择（base_url/api_kind/模型数组内置字典）→ 输入 APIKey →
 * 测试连接（暂存配置）→ 通过后保存全局配置 → onReady(true)。
 */
export function LLMStep({
  workdir: _workdir,
  onReady,
}: {
  workdir: string
  onReady: (ok: boolean) => void
}) {
  const { data: cfg } = useConfig()
  const testLLM = useTestLLM()
  const updateConfig = useUpdateConfig()

  const saved = cfg?.llm
  const [providerKey, setProviderKey] = useState<string>('deepseek')
  const [baseUrl, setBaseUrl] = useState('')
  const [apiKind, setApiKind] = useState('openai')
  const [model, setModel] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [tested, setTested] = useState(false)
  const [savedOk, setSavedOk] = useState(false)

  // 初始化：有已保存配置 → 预填（apiKey 留空表示沿用，掩码不可逆）。
  useEffect(() => {
    if (!saved) return
    if (!baseUrl && saved.base_url) setBaseUrl(saved.base_url)
    if (!model && saved.model) setModel(saved.model)
    if (saved.api_kind) setApiKind(saved.api_kind)
    const p = LLM_PROVIDERS.find(
      (x) => x.apiKind === saved.api_kind && x.baseUrl === saved.base_url,
    )
    if (p && !providerKey) setProviderKey(p.key)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [saved])

  const provider = providerByKey(providerKey)
  const pickProvider = (key: string) => {
    setProviderKey(key)
    setTested(false)
    const p = providerByKey(key)
    if (p && key !== 'custom') {
      setBaseUrl(p.baseUrl)
      setApiKind(p.apiKind)
      setModel(p.models[0] ?? '')
    }
  }

  const canTest = baseUrl.trim() !== '' && model.trim() !== '' && apiKey.trim() !== ''

  const runTest = () => {
    testLLM.mutate(
      { base_url: baseUrl.trim(), api_key: apiKey.trim(), model: model.trim(), api_kind: apiKind },
      {
        onSuccess: () => {
          setTested(true)
          toast.success('连接成功')
        },
        onError: (e) => {
          setTested(false)
          toast.error(e instanceof Error ? e.message : '连接失败')
        },
      },
    )
  }

  const saveAndContinue = () => {
    updateConfig.mutate(
      {
        llm: {
          base_url: baseUrl.trim(),
          ...(apiKey.trim() ? { api_key: apiKey.trim() } : {}),
          model: model.trim(),
          api_kind: apiKind,
        },
      },
      {
        onSuccess: () => {
          setSavedOk(true)
          onReady(true)
        },
        onError: (e) => toast.error(e instanceof Error ? e.message : '保存配置失败'),
      },
    )
  }

  const skip = () => {
    toast.info('已跳过 LLM 配置，可在「设置 - LLM」中随时配置')
    onReady(true)
  }

  return (
    <div className="space-y-4">
      {/* 平台快捷选择 */}
      <div>
        <Label>模型平台</Label>
        <div className="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-3">
          {LLM_PROVIDERS.map((p) => (
            <button
              key={p.key}
              type="button"
              onClick={() => pickProvider(p.key)}
              className={cn(
                'rounded-lg border px-3 py-2 text-left text-xs font-medium transition-colors',
                providerKey === p.key
                  ? 'border-primary-400 bg-primary-50 text-primary-700'
                  : 'border-divider text-muted-foreground hover:border-primary-300 hover:text-foreground',
              )}
            >
              {p.name}
            </button>
          ))}
        </div>
      </div>

      {/* 连接参数 */}
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <Label htmlFor="ob-base-url">Base URL</Label>
          <Input
            id="ob-base-url"
            value={baseUrl}
            onChange={(e) => {
              setBaseUrl(e.target.value)
              setTested(false)
            }}
            placeholder="https://api.deepseek.com/v1"
            className="mt-1.5 font-mono text-xs"
          />
        </div>
        <div>
          <Label htmlFor="ob-model">模型名称</Label>
          <Input
            id="ob-model"
            value={model}
            onChange={(e) => {
              setModel(e.target.value)
              setTested(false)
            }}
            placeholder="deepseek-chat"
            className="mt-1.5 font-mono text-xs"
          />
        </div>
        <div>
          <Label>兼容类型</Label>
          <Select value={apiKind} onValueChange={(v) => setApiKind(v)}>
            <SelectTrigger className="mt-1.5 h-10 font-mono text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="openai">openai</SelectItem>
              <SelectItem value="anthropic">anthropic</SelectItem>
              <SelectItem value="responses">responses</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div>
          <Label htmlFor="ob-key">API Key</Label>
          <Input
            id="ob-key"
            type="password"
            value={apiKey}
            onChange={(e) => {
              setApiKey(e.target.value)
              setTested(false)
            }}
            placeholder={saved?.api_key ? '已配置（留空沿用）' : 'sk-…'}
            className="mt-1.5 font-mono text-xs"
          />
        </div>
      </div>
      {provider?.note && <p className="text-xs text-muted-foreground">{provider.note}</p>}

      {/* 测试连接 */}
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          variant="outline"
          onClick={runTest}
          disabled={!canTest || testLLM.isPending}
        >
          {testLLM.isPending ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : (
            <PlugZap className="size-3.5" />
          )}
          测试连接
        </Button>
        {tested && (
          <span className="flex items-center gap-1 text-sm text-success">
            <CheckCircle2 className="size-4" /> 连接成功
          </span>
        )}
      </div>

      {/* 操作 */}
      <div className="flex items-center justify-between border-t border-divider pt-4">
        <Button variant="ghost" size="sm" onClick={skip}>
          跳过此步
        </Button>
        <Button
          size="sm"
          onClick={saveAndContinue}
          disabled={!tested || !canTest || updateConfig.isPending || savedOk}
        >
          {updateConfig.isPending ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : (
            <Sparkles className="size-3.5" />
          )}
          {savedOk ? '已保存' : '保存并继续'}
        </Button>
      </div>
    </div>
  )
}
