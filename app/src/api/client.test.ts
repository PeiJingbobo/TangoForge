import { describe, it, expect, beforeEach } from 'vitest'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/server'
import { apiRequest, apiGet, ApiError, setUiToken, DAEMON_BASE_URL } from './client'

describe('HTTP 客户端', () => {
  beforeEach(() => {
    setUiToken(null)
  })

  it('成功响应：code=0 返回 data', async () => {
    const data = await apiGet<string>('/ping')
    expect(data).toBe('pong')
  })

  it('携带 X-Project 与 X-UI-Token 头', async () => {
    setUiToken('secret-token')
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/projects`, ({ request }) => {
        expect(request.headers.get('X-Project')).toBe('/data/p1')
        expect(request.headers.get('X-UI-Token')).toBe('secret-token')
        return HttpResponse.json({ code: 0, data: [] })
      }),
    )
    await apiGet('/api/projects', { project: '/data/p1' })
  })

  it('POST 请求体 JSON 序列化', async () => {
    let body: unknown = null
    server.use(
      http.post(`${DAEMON_BASE_URL}/api/projects/import`, async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({ code: 0, data: { id: 9 } })
      }),
    )
    await apiRequest('/api/projects/import', { method: 'POST', body: { workdir: '/x' } })
    expect(body).toEqual({ workdir: '/x' })
  })

  it('业务错误：字符串 code → ApiError（含 message/detail）', async () => {
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/tasks`, () =>
        HttpResponse.json(
          { code: 'PERMISSION_DENIED', message: '权限不足', detail: 'actor=agent' },
          { status: 403 },
        ),
      ),
    )
    const err = await apiGet('/api/tasks').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).code).toBe('PERMISSION_DENIED')
    expect((err as ApiError).status).toBe(403)
    expect((err as ApiError).detail).toBe('actor=agent')
  })

  it('非 JSON 响应（导出内容）→ code 非 0 → 兜底错误', async () => {
    server.use(
      http.get(
        `${DAEMON_BASE_URL}/api/audit/export`,
        () => new HttpResponse('ts|actor', { status: 200 }),
      ),
    )
    // 导出端点走 apiRequest 但响应非 JSON：按协议应返回错误信封；此处验证不抛 JSON 解析异常
    await expect(apiGet('/api/audit/export')).rejects.toBeInstanceOf(ApiError)
  })

  it('网络错误 → NETWORK_ERROR', async () => {
    server.use(http.get(`${DAEMON_BASE_URL}/api/projects`, () => HttpResponse.error()))
    const err = await apiGet('/api/projects').catch((e: unknown) => e)
    expect((err as ApiError).code).toBe('NETWORK_ERROR')
  })

  it('错误码 → 中文映射兜底（无 message 时）', async () => {
    server.use(
      http.get(`${DAEMON_BASE_URL}/api/graph`, () =>
        HttpResponse.json({ code: 'CIRCULAR_DEPENDENCY' }, { status: 422 }),
      ),
    )
    const err = await apiGet('/api/graph').catch((e: unknown) => e)
    expect((err as ApiError).message).toContain('循环依赖')
  })
})
