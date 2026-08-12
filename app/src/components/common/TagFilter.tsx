import { useCallback, useEffect, useRef, useState, type WheelEvent } from 'react'
import { ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'

/** 右端是否仍有溢出内容的判定阈值（px） */
const SCROLL_EPSILON = 4

/**
 * 标签多选筛选（看板 / 任务导航复用）：
 * - 单行展示，标签列表可横向滚动；
 * - 高度与 TabsList（h-9）对齐；
 * - 多选：点击切换选中；「全部」清空（空选择 = 不过滤）；
 * - 鼠标滚轮 / 触控板均支持横向滚动（垂直 wheel 增量转水平 scrollLeft）；
 * - 隐藏原生滚动条；未滚到最右时右端悬浮箭头提示可继续选择，点击向右滚动。
 */
export function TagFilter({
  tags,
  selected,
  onChange,
  className,
}: {
  tags: string[]
  selected: Set<string>
  onChange: (next: Set<string>) => void
  className?: string
}) {
  const scrollerRef = useRef<HTMLDivElement>(null)
  const [canScrollMore, setCanScrollMore] = useState(false)

  const updateScrollHint = useCallback(() => {
    const el = scrollerRef.current
    if (!el) return
    setCanScrollMore(el.scrollWidth - el.clientWidth - el.scrollLeft > SCROLL_EPSILON)
  }, [])

  // 标签数量/布局变化后重新检测（初始挂载布局完成后计算一次）
  useEffect(() => {
    updateScrollHint()
  }, [updateScrollHint, tags])

  // 鼠标滚轮 → 水平滚动（触控板纵向手势同样生效）；可滚动时才拦截页面滚动
  const onWheel = (e: WheelEvent<HTMLDivElement>) => {
    const el = scrollerRef.current
    if (!el) return
    if (el.scrollWidth <= el.clientWidth) return
    const delta = Math.abs(e.deltaY) > Math.abs(e.deltaX) ? e.deltaY : e.deltaX
    if (delta === 0) return
    const before = el.scrollLeft
    el.scrollLeft += delta
    if (el.scrollLeft !== before) e.preventDefault()
  }

  // 点击箭头：向右滚动约一屏（不会越过末尾）
  const scrollMore = () => {
    const el = scrollerRef.current
    if (!el) return
    const max = el.scrollWidth - el.clientWidth
    el.scrollTo({ left: Math.min(el.scrollLeft + el.clientWidth * 0.8, max), behavior: 'smooth' })
  }

  if (tags.length === 0) return null

  const toggle = (tag: string) => {
    const next = new Set(selected)
    if (next.has(tag)) next.delete(tag)
    else next.add(tag)
    onChange(next)
  }

  const btnBase =
    'shrink-0 cursor-pointer rounded-full px-3 py-1 text-xs font-medium transition-colors'
  const btnActive = 'bg-primary-50 text-primary-700'
  const btnIdle = 'text-muted-foreground hover:bg-accent'

  return (
    <div className={cn('group relative min-w-0', className)}>
      <div
        ref={scrollerRef}
        onWheel={onWheel}
        onScroll={updateScrollHint}
        aria-label="按标签筛选"
        className="flex h-9 min-w-0 items-center gap-1.5 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        <button
          type="button"
          onClick={() => onChange(new Set())}
          className={cn(btnBase, selected.size === 0 ? btnActive : btnIdle)}
        >
          全部
        </button>
        {tags.map((tag) => (
          <button
            key={tag}
            type="button"
            aria-pressed={selected.has(tag)}
            onClick={() => toggle(tag)}
            className={cn(btnBase, selected.has(tag) ? btnActive : btnIdle)}
          >
            #{tag}
          </button>
        ))}
      </div>

      {/* 未滚到最右：容器 hover 时右端渐变遮罩 + 悬浮箭头淡入（过渡动画） */}
      {canScrollMore && (
        <>
          <div className="pointer-events-none absolute inset-y-0 right-0 w-10 bg-gradient-to-l from-background to-transparent opacity-0 transition-opacity duration-200 group-hover:opacity-100" />
          <button
            type="button"
            aria-label="更多标签"
            title="更多标签"
            onClick={scrollMore}
            className="pointer-events-none absolute top-1/2 right-1 grid size-6 -translate-y-1/2 translate-x-1 place-items-center rounded-full border border-divider bg-card text-muted-foreground opacity-0 shadow-sm transition-all duration-200 group-hover:pointer-events-auto group-hover:translate-x-0 group-hover:opacity-100 hover:bg-accent hover:text-foreground"
          >
            <ChevronRight className="size-3.5" />
          </button>
        </>
      )}
    </div>
  )
}
