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

import (
	"context"
	"net"
	"net/http"
	"tangoforge/internal/config"
)

// ActorClass 来源类别（audit_log.actor_class 取值）。
const (
	// ClassUI 用户 App UI（回环 + X-UI-Token），不查权限表。
	ClassUI = "ui"
	// ClassAgent Agent / CLI / 本地 HTTP（查权限表，最小信任）。
	ClassAgent = "agent"
	// ClassUnknown 无任何凭据的请求（仍查权限表，安全默认）。
	ClassUnknown = "unknown"
)

// Actor 来源识别结果（docs/TECHNICAL.md §3.3）。
type Actor struct {
	// Name actor 名称：ui / MCP 客户端名 / X-Actor / unknown。
	Name string `json:"name"`
	// Class 来源类别：ui / agent / unknown。
	Class string `json:"class"`
}

// contextKey 上下文键类型（避免与其它包冲突）。
type contextKey struct{ name string }

// actorKey 请求上下文中 Actor 的键。
var actorKey = contextKey{"actor"}

// WithActor 将来源识别结果写入请求上下文。
//
// 业务层写钩子（TF-012 审计 / TF-014 WS 事件）经 ctx 读取 actor，
// 实现「识别一次、全链路可用」，无需为每个入口单独传递（QA P3-1 确认）。
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey, a)
}

// ActorFrom 从上下文读取来源识别结果；未写入时返回 unknown（安全默认）。
func ActorFrom(ctx context.Context) Actor {
	a, ok := ctx.Value(actorKey).(Actor)
	if !ok {
		return Actor{Name: "unknown", Class: ClassUnknown}
	}
	return a
}

// FromMCP 构造 MCP stdio 会话的 agent Actor（TF-016 使用；不经过 HTTP 中间件）。
func FromMCP(clientName string) Actor {
	return Actor{Name: clientName, Class: ClassAgent}
}

// isLoopbackAddr 判断来源地址是否为回环（127.0.0.1 / ::1）。
// 与 internal/api 的 isLoopback 等价；分层约束下 auth 不引用 api，故本地复刻。
func isLoopbackAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Identify 按 docs/TECHNICAL.md §3.3 判定请求来源（5 级优先级）：
//
//  1. 回环 + X-UI-Token 匹配全局配置 ui_token → ui（全权限，不查权限表）；
//  2. 非回环 → 必须携带有效 Bearer（api_token）：
//     缺失 / 错误 / api_token 未配置 → needAuth=true（中间件回 401）；
//     有效 → agent（actor = X-Actor，缺省 unknown）；
//  3. 回环但无有效 UI-Token：携带 X-Actor → agent（CLI 默认 human，最小信任）；
//  4. 无任何可识别凭据 → unknown（仍按权限表检查，安全默认）。
//
// 返回值：识别出的 Actor 与是否需要 401（needAuth）。
// UI 凭据仅回环有效：非回环请求即使携带正确 X-UI-Token 也不视为 ui（落入第 2 分支）。
func Identify(cfg *config.GlobalConfig, r *http.Request) (Actor, bool) {
	if isLoopbackAddr(r.RemoteAddr) {
		// 1. UI 凭据：仅回环来源有效。
		if tok := r.Header.Get("X-UI-Token"); tok != "" && cfg != nil &&
			cfg.UIToken != "" && SecureEqual(tok, cfg.UIToken) {
			return Actor{Name: "ui", Class: ClassUI}, false
		}
		// 3. 本地 HTTP / CLI：X-Actor 头（CLI 默认 human）。
		if name := r.Header.Get("X-Actor"); name != "" {
			return Actor{Name: name, Class: ClassAgent}, false
		}
		// 4. 无任何凭据 → unknown。
		return Actor{Name: "unknown", Class: ClassUnknown}, false
	}

	// 2. 远程请求：必须携带有效 Bearer（api_token），否则 401。
	bearer := BearerToken(r)
	if bearer == "" || cfg == nil || cfg.APIToken == "" || !SecureEqual(bearer, cfg.APIToken) {
		return Actor{}, true
	}
	name := r.Header.Get("X-Actor")
	if name == "" {
		name = "unknown"
	}
	return Actor{Name: name, Class: ClassAgent}, false
}
