import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Loader2, Monitor, Moon, Sun } from 'lucide-react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { useConfig, useUpdateConfig, isConfigInvalid } from '@/hooks/useConfig'
import { useThemeMode, type ThemeMode } from '@/hooks/useThemeMode'
import { useSkillTemplate, useSkillTemplateWrite } from '@/hooks/useSkills'
import { KnowledgeSection } from '@/features/settings/KnowledgeSection'
import { CLISettingsSection } from '@/features/settings/CLISettingsSection'
import { UpdateSection } from '@/features/settings/UpdateSection'
import { cn } from '@/lib/utils'

/**
 * 全局首选项（TF-029）：LLM / 外观 / 守护进程 分组。
 * 实时保存（无确定按钮）：字段变更即保存；后端校验（config.Validate）失败 → toast 提示并回滚输入。
 * LLM / 守护进程为全局配置（GET/PUT /api/config，仅 UI）；外观偏好本地持久化（localStorage）。
 * TF-033：新增「Skill 模板」tab（默认技能包模板编辑，QA-S4）。
 */
export function SettingsPage() {
  const { isError, error, refetch, isFetching } = useConfig()

  if (isError) {
    return (
      <div className="w-full px-6 py-6">
        <h1 className="text-h2 text-foreground">首选项</h1>
        <div className="mt-6 rounded-2xl border border-destructive-200 bg-destructive-soft p-5">
          <div className="text-sm font-semibold text-destructive-ink">全局配置加载失败</div>
          <p className="mt-1 text-sm text-muted-foreground">
            {error instanceof Error ? error.message : '无法连接守护进程，请确认服务已启动。'}
          </p>
          <Button className="mt-3" onClick={() => void refetch()} disabled={isFetching}>
            {isFetching && <Loader2 className="size-4 animate-spin" />}
            重试
          </Button>
        </div>
      </div>
    )
  }

  return (
    // 标题 + Tab 固定，仅各 tab 内容区内部滚动（TF-042）；宽度与边距和其他页面统一
    <div className="flex h-full w-full min-h-0 flex-col px-6 py-6">
      <div className="shrink-0">
        <h1 className="text-h2 text-foreground">首选项</h1>
        <p className="mt-1 text-caption text-muted-foreground">
          设置实时保存（无确定按钮）；保存前校验配置项值，无效则提示并回滚。
        </p>
      </div>

      <Tabs defaultValue="llm" className="mt-5 flex min-h-0 flex-1 flex-col">
        <TabsList className="w-fit shrink-0">
          <TabsTrigger value="llm">LLM</TabsTrigger>
          <TabsTrigger value="appearance">外观</TabsTrigger>
          <TabsTrigger value="daemon">守护进程</TabsTrigger>
          <TabsTrigger value="skill">Skill 模板</TabsTrigger>
          <TabsTrigger value="knowledge">知识库</TabsTrigger>
          <TabsTrigger value="cli">CLI</TabsTrigger>
          <TabsTrigger value="about">关于</TabsTrigger>
        </TabsList>
        <TabsContent value="llm" className="mt-4 min-h-0 flex-1 overflow-y-auto pr-1 pt-6">
          <LLMSection />
        </TabsContent>
        <TabsContent value="appearance" className="mt-4 min-h-0 flex-1 overflow-y-auto pr-1 pt-6">
          <AppearanceSection />
        </TabsContent>
        <TabsContent value="daemon" className="mt-4 min-h-0 flex-1 overflow-y-auto pr-1 pt-6">
          <DaemonSection />
        </TabsContent>
        <TabsContent value="skill" className="mt-4 min-h-0 flex-1 overflow-y-auto pr-1 pt-6">
          <SkillTemplateSection />
        </TabsContent>
        <TabsContent value="knowledge" className="mt-4 min-h-0 flex-1 overflow-y-auto pr-1 pt-6">
          <KnowledgeSection />
        </TabsContent>
        <TabsContent value="cli" className="mt-4 min-h-0 flex-1 overflow-y-auto pr-1 pt-6">
          <CLISettingsSection />
        </TabsContent>
        <TabsContent value="about" className="mt-4 min-h-0 flex-1 overflow-y-auto pr-1 pt-6">
          <UpdateSection />
        </TabsContent>
      </Tabs>
    </div>
  )
}

/* ---------- Skill 模板（TF-033 QA-S4） ---------- */

function SkillTemplateSection() {
  const { data, isLoading } = useSkillTemplate()
  const write = useSkillTemplateWrite()
  const [draft, setDraft] = useState<string | null>(null)
  const content = draft ?? data?.content ?? ''

  if (isLoading) return <Skeleton className="h-56 w-full" />
  if (!data) return <p className="text-sm text-muted-foreground">Skill 模板加载失败。</p>

  const dirty = draft !== null && draft !== data.content

  const save = () => {
    if (draft === null) return
    write.mutate(draft, {
      onSuccess: () => {
        toast.success('Skill 模板已保存（~/.taskboard-app/skills/_template/SKILL.md）')
        setDraft(null)
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : '保存失败'),
    })
  }

  return (
    <div className="space-y-3">
      <div>
        <Label>默认 Skill 模板</Label>
        <p className="mt-1 text-caption text-muted-foreground">
          新建/内置技能包的骨架（SKILL.md，frontmatter 含
          name/description/version/hosts/when_to_use）。 保存到全局技能库{' '}
          <code className="font-mono text-[11px]">~/.taskboard-app/skills/_template/SKILL.md</code>
          。
        </p>
      </div>
      <textarea
        value={content}
        onChange={(e) => setDraft(e.target.value)}
        className="h-72 w-full resize-y rounded-lg border border-divider bg-background p-3 font-mono text-xs leading-relaxed"
        aria-label="Skill 模板内容"
      />
      <div className="flex items-center gap-2">
        <Button onClick={save} disabled={!dirty || write.isPending}>
          {write.isPending && <Loader2 className="size-4 animate-spin" />}
          保存模板
        </Button>
        {dirty && (
          <Button variant="outline" onClick={() => setDraft(null)}>
            放弃修改
          </Button>
        )}
      </div>
    </div>
  )
}

/* ---------- LLM ---------- */

const API_KINDS = [
  { value: 'openai', label: 'openai（/chat/completions）' },
  { value: 'anthropic', label: 'anthropic（/v1/messages）' },
  { value: 'responses', label: 'responses（/v1/responses）' },
]

function LLMSection() {
  const { data, isLoading } = useConfig()
  const updateConfig = useUpdateConfig()
  const [draft, setDraft] = useState<{
    base_url: string
    model: string
    api_kind: string
    api_key: string
  } | null>(null)
  const saved = draft ?? {
    base_url: data?.llm.base_url ?? '',
    model: data?.llm.model ?? '',
    api_kind: data?.llm.api_kind || 'openai',
    api_key: '',
  }
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(
    () => () => {
      if (saveTimer.current) clearTimeout(saveTimer.current)
    },
    [],
  )

  if (isLoading) return <Skeleton className="h-56 w-full" />
  if (!data) return <p className="text-sm text-muted-foreground">配置加载失败。</p>

  const patch = (next: typeof saved) => {
    setDraft(next)
    if (saveTimer.current) clearTimeout(saveTimer.current)
    saveTimer.current = setTimeout(() => {
      updateConfig.mutate(
        {
          llm: {
            base_url: next.base_url,
            model: next.model,
            api_kind: next.api_kind,
            ...(next.api_key ? { api_key: next.api_key } : {}),
          },
        },
        {
          onSuccess: () => toast.success('LLM 设置已保存'),
          onError: (err) => {
            if (isConfigInvalid(err)) {
              toast.error(err instanceof Error ? err.message : '配置无效', {
                description: '已回滚',
              })
              setDraft(null) // 回滚：以服务端为准
            } else {
              toast.error(err instanceof Error ? err.message : '保存失败')
            }
          },
        },
      )
    }, 500)
  }

  const set = (field: keyof typeof saved, value: string) => patch({ ...saved, [field]: value })

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="cfg-base-url">接口地址（BaseURL）</Label>
        <Input
          id="cfg-base-url"
          value={saved.base_url}
          onChange={(e) => set('base_url', e.target.value)}
          placeholder="https://api.deepseek.com"
          className="mt-1.5 font-mono text-sm"
        />
      </div>
      <div>
        <Label htmlFor="cfg-model">模型名</Label>
        <Input
          id="cfg-model"
          value={saved.model}
          onChange={(e) => set('model', e.target.value)}
          placeholder="deepseek-chat"
          className="mt-1.5 text-sm"
        />
      </div>
      <div>
        <Label>协议类型</Label>
        <div className="mt-1.5 flex flex-wrap gap-1.5">
          {API_KINDS.map((k) => (
            <Badge
              key={k.value}
              role="button"
              variant={saved.api_kind === k.value ? 'default' : 'outline'}
              className="cursor-pointer px-3 py-1.5"
              onClick={() => set('api_kind', k.value)}
            >
              {k.label}
            </Badge>
          ))}
        </div>
      </div>
      <div>
        <Label htmlFor="cfg-api-key">API Key</Label>
        <Input
          id="cfg-api-key"
          type="password"
          value={saved.api_key}
          onChange={(e) => set('api_key', e.target.value)}
          placeholder={
            data.llm.api_key ? '已配置（留空不修改）' : '未配置（可留空，回退 DEEPSEEK_API_KEY）'
          }
          className="mt-1.5 font-mono text-sm"
        />
        {data.llm.api_key && (
          <p className="mt-1 text-caption text-muted-foreground">
            当前：{data.llm.api_key}（掩码展示）
          </p>
        )}
      </div>
      {updateConfig.isPending && <Loader2 className="size-4 animate-spin text-muted-foreground" />}
    </div>
  )
}

/* ---------- 外观 ---------- */

const MODE_OPTIONS: { key: ThemeMode; label: string; icon: typeof Sun }[] = [
  { key: 'light', label: '浅色', icon: Sun },
  { key: 'dark', label: '深色', icon: Moon },
  { key: 'system', label: '跟随系统', icon: Monitor },
]

function AppearanceSection() {
  const { mode, accent, setMode, setAccent, presets } = useThemeMode()

  return (
    <div className="space-y-5">
      <div>
        <Label>界面模式</Label>
        <div className="mt-1.5 flex flex-wrap gap-1.5">
          {MODE_OPTIONS.map((m) => {
            const Icon = m.icon
            return (
              <Badge
                key={m.key}
                role="button"
                variant={mode === m.key ? 'default' : 'outline'}
                className="cursor-pointer gap-1.5 px-3 py-1.5"
                onClick={() => setMode(m.key)}
              >
                <Icon className="size-3.5" />
                {m.label}
              </Badge>
            )
          })}
        </div>
      </div>
      <div>
        <Label>主强调色</Label>
        <div className="mt-2 flex flex-wrap items-center gap-2">
          {presets.map((p) => (
            <button
              key={p.key}
              type="button"
              title={p.label}
              onClick={() => setAccent(p.key)}
              aria-label={`主色 ${p.label}`}
              className={cn(
                'size-7 rounded-full border-2 border-transparent transition-transform hover:scale-110',
                accent === p.key && 'border-foreground',
              )}
              style={{ backgroundColor: p.hex }}
            />
          ))}
          <label className="flex cursor-pointer items-center gap-2 rounded-full border border-dashed border-border px-3 py-1.5 text-xs text-muted-foreground hover:border-primary-300">
            自定义
            <input
              type="color"
              value={presets.some((p) => p.key === accent) ? presets[0].hex : accent}
              onChange={(e) => setAccent(e.target.value)}
              className="size-4 cursor-pointer border-0 bg-transparent p-0"
              aria-label="自定义主色"
            />
          </label>
        </div>
      </div>
      <p className="text-caption text-muted-foreground">
        外观偏好保存在本机（localStorage），不写入全局配置。
      </p>
    </div>
  )
}

/* ---------- 守护进程 ---------- */

function DaemonSection() {
  const { data, isLoading } = useConfig()
  const updateConfig = useUpdateConfig()
  const [draft, setDraft] = useState<{ port: string; remote_access: boolean } | null>(null)
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(
    () => () => {
      if (saveTimer.current) clearTimeout(saveTimer.current)
    },
    [],
  )

  if (isLoading) return <Skeleton className="h-40 w-full" />
  if (!data) return <p className="text-sm text-muted-foreground">配置加载失败。</p>

  const current = draft ?? { port: String(data.port), remote_access: data.remote_access }

  const commit = (next: { port: string; remote_access: boolean }) => {
    setDraft(next)
    if (saveTimer.current) clearTimeout(saveTimer.current)
    saveTimer.current = setTimeout(() => {
      const port = Number(next.port)
      updateConfig.mutate(
        { port, remote_access: next.remote_access },
        {
          onSuccess: () => toast.success('守护进程设置已保存'),
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

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="cfg-port">监听端口</Label>
        <Input
          id="cfg-port"
          value={current.port}
          onChange={(e) => commit({ ...current, port: e.target.value })}
          inputMode="numeric"
          className="mt-1.5 w-40 font-mono text-sm"
        />
        <p className="mt-1 text-caption text-muted-foreground">
          端口变更将动态重绑监听（热重载）。
        </p>
      </div>
      <label className="flex cursor-pointer items-center gap-2.5">
        <input
          type="checkbox"
          checked={current.remote_access}
          onChange={(e) => commit({ ...current, remote_access: e.target.checked })}
          className="size-4 accent-[var(--primary-500)]"
          aria-label="允许远程访问"
        />
        <span className="text-sm">允许远程访问（需配置 api_token）</span>
      </label>
      {updateConfig.isPending && <Loader2 className="size-4 animate-spin text-muted-foreground" />}
    </div>
  )
}
