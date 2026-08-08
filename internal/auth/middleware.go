package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"tangoforge/internal/config"
)

// workdirKey 请求上下文中项目工作目录的键。
var workdirKey = contextKey{"workdir"}

// WithWorkdir 将（规范化后的）项目工作目录写入请求上下文。
// api 层 projectMiddleware 调用；auth 权限中间件与 handler 统一从 ctx 读取（显式优于隐式）。
func WithWorkdir(ctx context.Context, workdir string) context.Context {
	return context.WithValue(ctx, workdirKey, workdir)
}

// WorkdirFrom 从上下文读取项目工作目录；未写入时返回空串。
func WorkdirFrom(ctx context.Context) string {
	wd, _ := ctx.Value(workdirKey).(string)
	return wd
}

// IdentifyMiddleware 来源识别中间件（TF-010 落地为 HTTP 层）：
//
//  1. Identify(getCfg(), r) 判定 Actor（5 级优先级，docs/TECHNICAL.md §3.3）；
//  2. needAuth=true（远程缺失/错误 Bearer）→ 401 UNAUTHORIZED；
//  3. 识别结果写入 ctx（WithActor），后续权限中间件 / handler / 写钩子统一读取。
//
// getCfg 为配置提供者：每次请求调用获取最新全局配置，
// 保证 ui_token / api_token 热重载即时生效（不捕获启动时指针）。
func IdentifyMiddleware(getCfg func() *config.GlobalConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, needAuth := Identify(getCfg(), r)
			if needAuth {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED",
					"远程请求必须携带有效的 Authorization: Bearer <api_token>", "")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithActor(r.Context(), actor)))
		})
	}
}

// RequirePermission 权限中间件（TF-011）：
//
//   - actor_class == "ui" → 直接放行（不查权限表，REQUIREMENTS.md §7.1）；
//   - 其余（agent / unknown）→ 查项目 permissions 表对应 action，未授权 403
//     PERMISSION_DENIED，并触发 OnDenied 回调（TF-012 审计 denied 接入）；
//   - workdir 从 ctx 读取（api 层 projectMiddleware 已写入）。
func (s *PermissionStore) RequirePermission(action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := ActorFrom(r.Context())
			workdir := WorkdirFrom(r.Context())

			if actor.Class != ClassUI {
				allowed, err := s.Allowed(r.Context(), workdir, action)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "INTERNAL",
						"权限查询失败", err.Error())
					return
				}
				if !allowed {
					if s.OnDenied != nil {
						s.OnDenied(r.Context(), workdir, action)
					}
					writeError(w, http.StatusForbidden, "PERMISSION_DENIED",
						"无权执行该操作（action="+action+"）", actor.Class)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeError 写统一错误响应（与 internal/api 格式一致，避免 auth 引用 api 造成循环）。
func writeError(w http.ResponseWriter, status int, code, message, detail string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	body := map[string]string{
		"code":    code,
		"message": message,
	}
	if detail != "" {
		body["detail"] = detail
	}
	_ = json.NewEncoder(w).Encode(body)
}
