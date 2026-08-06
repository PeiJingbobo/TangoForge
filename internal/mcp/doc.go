// Package mcp 负责 MCP 工具注册与执行。
//
// 约束（docs/TECHNICAL.md §3.4 / docs/AGENTS.md §9）：
//   - v1 固定工具集（list_tools / call_tool），不动态注册；
//   - 每个工具必须携带 project（工作目录）参数，未携带或未注册返回 PROJECT_NOT_FOUND；
//   - 与 HTTP / CLI 共享同一套业务层实现（接口先行）。
package mcp
