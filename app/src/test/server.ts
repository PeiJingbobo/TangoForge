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
)
