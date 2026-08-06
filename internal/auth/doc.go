// Package auth 负责来源识别（ui/agent/unknown）、Token 校验与权限中间件。
//
// 约束（docs/TECHNICAL.md §3.3「来源识别 / 权限中间件」）：
//   - Actor 判定优先级：
//     1) X-UI-Token + 回环来源 → ui（全权限）；
//     2) MCP stdio 会话 → agent（actor = 客户端名称）；
//     3) 远程 Bearer Token → agent（actor = X-Actor 或 unknown）；
//     4) 本地 HTTP / CLI（X-Actor 头）→ agent（最小信任，查权限表）；
//     5) 无法识别 → unknown（查权限表并记录审计）。
//   - 所有 /api/* 路由必须经过本包中间件；actor_class == "ui" 直接放行，其余查 permissions 表；
//   - AI Agent（MCP）在任何情况下不可修改权限表（PUT /api/permissions 额外校验 X-UI-Token + 回环 IP）。
package auth
