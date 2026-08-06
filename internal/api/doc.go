// Package api 提供 HTTP / WebSocket 路由与处理器（传输层，薄封装）。
//
// 约束（docs/TECHNICAL.md §2.3 / §3.4）：
//   - 只做参数解析与响应格式化，禁止重复业务逻辑；
//   - 与 CLI / MCP 共享同一套业务层实现（接口先行），不得重复造轮子；
//   - 统一响应格式：成功 {"code":0,"data":...}；失败 {"code":非零,"message":...,"detail":...}；
//   - 业务错误码：PROJECT_NOT_FOUND / INVALID_TRANSITION / CIRCULAR_DEPENDENCY /
//     STATUS_IN_USE / PERMISSION_DENIED / IMPORT_FAILED 等；
//   - WebSocket 实时事件（/ws/events?project=<dir>）：task.* / import.* / export.* /
//     project.* / permission.* / skill.* / state_machine.*。
package api
