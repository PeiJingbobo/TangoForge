import { ApiEnvelope, ERROR_MESSAGES } from '@/types/api'

/** 守护进程监听端口（TASK-SEMANTICS §6.1，全局配置可热重载调整，默认 19810） */
export const DAEMON_BASE_URL = 'http://127.0.0.1:19810'

/** UI 会话凭据：TF-024 由 Electron preload 注入；测试与 CLI 模式可为空 */
let uiToken: string | null = null
export function setUiToken(token: string | null): void {
  uiToken = token
}
export function getUiToken(): string | null {
  return uiToken
}

/** 业务错误：携带后端业务码、HTTP 状态与细节 */
export class ApiError extends Error {
  readonly code: string
  readonly status: number
  readonly detail?: string

  constructor(code: string, message: string, detail?: string, status = 0) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.detail = detail
  }
}

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  /** JSON 请求体 */
  body?: unknown
  /** 项目工作目录 → X-Project 头（/api/projects 组可省略） */
  project?: string
  /** 查询参数：键为最终字符串（如 filter[status]），值 undefined 跳过 */
  query?: Record<string, string | number | boolean | undefined>
  headers?: Record<string, string>
  /** 覆盖默认超时（ms），默认 30s */
  timeoutMs?: number
}

function buildQuery(query?: RequestOptions['query']): string {
  if (!query) return ''
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined) continue
    params.set(k, String(v))
  }
  const s = params.toString()
  return s ? `?${s}` : ''
}

/**
 * 统一 HTTP 客户端：携带 X-UI-Token / X-Project，解析 {code,data} 信封。
 * 成功 → 返回 data；失败（业务码或网络错误）→ 抛 ApiError。
 */
export async function apiRequest<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...opts.headers,
  }
  if (uiToken) headers['X-UI-Token'] = uiToken
  if (opts.project) headers['X-Project'] = opts.project

  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), opts.timeoutMs ?? 30_000)

  let res: Response
  try {
    res = await fetch(`${DAEMON_BASE_URL}${path}${buildQuery(opts.query)}`, {
      method: opts.method ?? 'GET',
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      signal: controller.signal,
    })
  } catch (err) {
    throw new ApiError(
      'NETWORK_ERROR',
      ERROR_MESSAGES.NETWORK_ERROR,
      err instanceof Error ? err.message : undefined,
    )
  } finally {
    clearTimeout(timer)
  }

  // 审计导出为 text/plain 等非 JSON 响应
  const text = await res.text()
  let envelope: ApiEnvelope<T> | null = null
  try {
    envelope = JSON.parse(text) as ApiEnvelope<T>
  } catch {
    // 非 JSON（如导出文件内容）
  }

  if (!envelope || envelope.code !== 0) {
    const code = typeof envelope?.code === 'string' ? envelope.code : mapHttpStatus(res.status)
    const message = envelope?.message || ERROR_MESSAGES[code] || `请求失败（${res.status}）`
    throw new ApiError(code, message, envelope?.detail, res.status)
  }
  return envelope.data as T
}

function mapHttpStatus(status: number): string {
  switch (status) {
    case 401:
      return 'UNAUTHORIZED'
    case 403:
      return 'PERMISSION_DENIED'
    case 404:
      return 'NOT_FOUND'
    case 422:
      return 'BAD_REQUEST'
    case 500:
      return 'INTERNAL_ERROR'
    default:
      return 'INTERNAL_ERROR'
  }
}

/** 便捷 GET（无 body） */
export function apiGet<T>(
  path: string,
  opts?: Omit<RequestOptions, 'method' | 'body'>,
): Promise<T> {
  return apiRequest<T>(path, { ...opts, method: 'GET' })
}
