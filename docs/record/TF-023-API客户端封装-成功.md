# TF-023 API 客户端封装与类型对齐 — 任务总结

> 结果：成功　|　日期：2026-08-06　|　执行人：ai

## 1. 任务范围
React 前端与 daemon 的全部通信底座：HTTP 客户端、WS 客户端、DTO 类型、TanStack Query hooks。

## 2. 交付内容
- `src/api/client.ts`：fetch 封装（X-UI-Token/X-Project 头、统一 `{code,data}` 信封、业务码→ApiError、错误码中文映射、30s 超时、query 嵌套键组装）
- `src/api/ws.ts`：EventSocket（指数退避重连 1s→30s、disconnect 幂等终止、状态回调）；jsdom 无 WebSocket → FakeWebSocket 测试
- `src/types/models.ts`：Project/StateMachine(PascalCase)/Graph/ImportDraft/Export/Permission(16 action)/Skill/Audit/WSEvent 全 DTO；`src/types/api.ts` 错误码与映射
- `src/hooks/`：keys.ts + useProject（store 解析）+ useProjects/useTasks(CRUD+状态+归档)/useGraph/useStateMachine/useImports/useExports/usePermissions/useSkills/useAudit/useEvents（WS→Query 失效映射）
- `src/stores/project.ts`：zustand + persist（当前项目 workdir 持久化）
- `src/test/server.ts` + setup 改造：MSW server 全局挂载

## 3. 验证结果
- `pnpm typecheck` / `pnpm lint` 全绿；`pnpm test` **35 个用例全过**（client 7 + ws 6 + hooks 6 + theme 13 + utils 3）
- 覆盖：MSW 拦截 HTTP（头断言/错误映射/网络错误）、FakeWebSocket（连接/重连退避/disconnect）、hooks 渲染与 mutation、INVALID_TRANSITION 透传

## 4. 遗留问题与后续
- 无。UI token 注入（TF-024）；WS 事件消费（TF-025 起页面层）
