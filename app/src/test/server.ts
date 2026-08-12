import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { DAEMON_BASE_URL } from '@/api/client'

/** MSW 拦截器：默认 mock 数据（按需在测试内覆盖 handler） */
export const server = setupServer(
  http.get(`${DAEMON_BASE_URL}/ping`, () => HttpResponse.json({ code: 0, data: 'pong' })),

  http.get(`${DAEMON_BASE_URL}/api/projects`, () =>
    HttpResponse.json({
      code: 0,
      data: [
        {
          id: 1,
          name: 'TangoForge',
          workdir: '/data/projects/tangoforge',
          created_at: '2026-08-06T10:00:00+08:00',
          last_opened_at: '2026-08-06T12:00:00+08:00',
        },
      ],
    }),
  ),

  http.post(`${DAEMON_BASE_URL}/api/projects/import`, () =>
    HttpResponse.json({
      code: 0,
      data: {
        id: 2,
        name: 'demo',
        workdir: '/data/projects/demo',
        created_at: '2026-08-06T10:00:00+08:00',
        last_opened_at: null,
      },
    }),
  ),

  // TF-041 引导：目录检查（默认未注册、无元数据）。
  http.post(`${DAEMON_BASE_URL}/api/projects/check`, () =>
    HttpResponse.json({
      code: 0,
      data: { registered: false, onboarded: false, has_meta: false, meta_valid: true },
    }),
  ),

  http.get(`${DAEMON_BASE_URL}/api/tasks`, () =>
    HttpResponse.json({
      code: 0,
      data: {
        tree: [
          {
            id: 'task-1',
            project_id: 1,
            parent_id: null,
            title: '示例任务',
            number: 'T01',
            description: '',
            status: 'todo',
            priority: 0,
            tags: [],
            assignee: '',
            depends_on: [],
            archived_from: '',
            source_file: '',
            source_section: '',
            created_at: '2026-08-06T10:00:00+08:00',
            updated_at: '2026-08-06T10:00:00+08:00',
            children: [],
          },
        ],
        total: 1,
        page: 0,
        size: 0,
      },
    }),
  ),

  http.get(`${DAEMON_BASE_URL}/api/tasks/task-1`, () =>
    HttpResponse.json({
      code: 0,
      data: {
        id: 'task-1',
        project_id: 1,
        parent_id: null,
        title: '示例任务',
        number: 'T01',
        description: '',
        status: 'todo',
        priority: 0,
        tags: [],
        assignee: '',
        depends_on: [],
        archived_from: '',
        source_file: '',
        source_section: '',
        created_at: '2026-08-06T10:00:00+08:00',
        updated_at: '2026-08-06T10:00:00+08:00',
      },
    }),
  ),

  http.get(`${DAEMON_BASE_URL}/api/state-machine`, () =>
    HttpResponse.json({
      code: 0,
      data: {
        States: [
          { Key: 'todo', Label: '待办', Color: '#9aa0a6' },
          { Key: 'doing', Label: '进行中', Color: '#2292d8' },
          { Key: 'done', Label: '已完成', Color: '#22c55e' },
        ],
        Transitions: [
          { From: 'todo', To: ['doing'] },
          { From: 'doing', To: ['done', 'todo'] },
          { From: 'done', To: ['doing'] },
        ],
      },
    }),
  ),

  http.get(`${DAEMON_BASE_URL}/api/graph`, () =>
    HttpResponse.json({
      code: 0,
      data: { nodes: [], edges: [] },
    }),
  ),

  http.post(`${DAEMON_BASE_URL}/api/tasks`, () =>
    HttpResponse.json(
      {
        code: 0,
        data: {
          id: 'task-new',
          project_id: 1,
          parent_id: null,
          title: '新任务',
          number: 'T02',
          description: '',
          status: 'todo',
          priority: 0,
          tags: [],
          assignee: '',
          depends_on: [],
          archived_from: '',
          source_file: '',
          source_section: '',
          created_at: '2026-08-06T10:00:00+08:00',
          updated_at: '2026-08-06T10:00:00+08:00',
        },
      },
      { status: 201 },
    ),
  ),

  // TF-052 知识库默认 mock。
  http.get(`${DAEMON_BASE_URL}/api/knowledge/bases`, () =>
    HttpResponse.json({
      code: 0,
      data: [
        {
          id: 1,
          project_id: 1,
          name: '默认库',
          description: '',
          is_default: true,
          created_at: '2026-08-12T10:00:00+08:00',
          updated_at: '2026-08-12T10:00:00+08:00',
          doc_count: 1,
        },
      ],
    }),
  ),
  http.get(`${DAEMON_BASE_URL}/api/knowledge/documents`, () =>
    HttpResponse.json({
      code: 0,
      data: {
        items: [
          {
            id: 'doc-1',
            project_id: 1,
            path: 'docs/spec.md',
            abs_path: '/data/projects/tangoforge/docs/spec.md',
            rel_path: 'docs/spec.md',
            origin_path: '',
            display_name: 'spec.md',
            type: 'text',
            size: 1024,
            mtime: '2026-08-12T10:00:00+08:00',
            content_hash: 'abc',
            summary: '接口规格说明',
            status: 'ok',
            embedded: 1,
            embedding_model: 'test-embed',
            index_error: '',
            history: [],
            created_at: '2026-08-12T10:00:00+08:00',
            updated_at: '2026-08-12T10:00:00+08:00',
            task_count: 1,
            kb_ids: [1],
          },
        ],
        total: 1,
        page: 0,
        size: 50,
      },
    }),
  ),
  http.get(`${DAEMON_BASE_URL}/api/knowledge/documents/doc-1`, () =>
    HttpResponse.json({
      code: 0,
      data: {
        id: 'doc-1',
        project_id: 1,
        path: 'docs/spec.md',
        abs_path: '/data/projects/tangoforge/docs/spec.md',
        rel_path: 'docs/spec.md',
        origin_path: '',
        display_name: 'spec.md',
        type: 'text',
        size: 1024,
        mtime: '2026-08-12T10:00:00+08:00',
        content_hash: 'abc',
        summary: '接口规格说明',
        status: 'ok',
        embedded: 1,
        embedding_model: 'test-embed',
        index_error: '',
        history: [],
        created_at: '2026-08-12T10:00:00+08:00',
        updated_at: '2026-08-12T10:00:00+08:00',
        task_count: 1,
        kb_ids: [1],
      },
    }),
  ),
  http.get(`${DAEMON_BASE_URL}/api/knowledge/documents/doc-1/content`, () =>
    HttpResponse.json({
      code: 0,
      data: { type: 'text', content: '# 接口规格\n正文内容', path: 'docs/spec.md' },
    }),
  ),
  http.get(`${DAEMON_BASE_URL}/api/knowledge/tasks/task-1`, () =>
    HttpResponse.json({
      code: 0,
      data: [
        {
          id: 'doc-1',
          project_id: 1,
          path: 'docs/spec.md',
          abs_path: '/data/projects/tangoforge/docs/spec.md',
          rel_path: 'docs/spec.md',
          origin_path: '',
          display_name: 'spec.md',
          type: 'text',
          size: 1024,
          mtime: '2026-08-12T10:00:00+08:00',
          content_hash: 'abc',
          summary: '接口规格说明',
          status: 'ok',
          embedded: 1,
          embedding_model: 'test-embed',
          index_error: '',
          history: [],
          created_at: '2026-08-12T10:00:00+08:00',
          updated_at: '2026-08-12T10:00:00+08:00',
        },
      ],
    }),
  ),
  http.get(`${DAEMON_BASE_URL}/api/knowledge/search`, () =>
    HttpResponse.json({
      code: 0,
      data: {
        query: '接口',
        total: 1,
        items: [
          {
            document: {
              id: 'doc-1',
              display_name: 'spec.md',
              path: 'docs/spec.md',
              abs_path: '/data/projects/tangoforge/docs/spec.md',
              rel_path: 'docs/spec.md',
              type: 'text',
              summary: '接口规格说明',
              status: 'ok',
              embedded: 1,
            },
            score: 0.87,
            chunks: [{ heading: '接口', text: '接口变更说明', score: 0.87, seq: 0 }],
            missing: false,
          },
        ],
      },
    }),
  ),
  http.post(`${DAEMON_BASE_URL}/api/knowledge/scan`, () =>
    HttpResponse.json({
      code: 0,
      data: { total: 1, indexed: 0, skipped: 0, failed: 0, missing: 0 },
    }),
  ),
  http.post(`${DAEMON_BASE_URL}/api/knowledge/link`, () =>
    HttpResponse.json({ code: 0, data: { task_id: 'task-1' } }),
  ),
  http.post(`${DAEMON_BASE_URL}/api/knowledge/unlink`, () =>
    HttpResponse.json({ code: 0, data: { ok: true } }),
  ),
)
