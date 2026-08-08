package api

import (
	"net/http"
	"path/filepath"
	"tangoforge/internal/auth"
	"tangoforge/internal/db"
)

// remoteAccessMiddleware 来源过滤（QA Q8 内存标志热切换）：
// remote_access=false 时拒绝非回环来源（403），开关由 setConfig 即时更新，无需重启。
func (s *Server) remoteAccessMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.currentConfig().RemoteAccess && !isLoopback(r.RemoteAddr) {
			writeError(w, http.StatusForbidden, "REMOTE_ACCESS_DISABLED",
				"remote access disabled", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware 浏览器跨源支持（TF-028 桌面端联调发现）：
// daemon 仅监听 127.0.0.1 回环；Electron 渲染进程来源为 dev server（http://localhost:5173）
// 或生产 file://（Origin: null），fetch 需 CORS 放行 + 自定义头（X-Project / X-UI-Token）。
// 策略：回显 Origin（回环监听，无第三方来源风险）；OPTIONS 预检直接 204。
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Project, X-UI-Token")
		w.Header().Set("Access-Control-Max-Age", "600")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// projectMiddleware 项目注册校验（REQUIREMENTS.md §5.2）：
// 每个 /api/* 请求必须显式携带项目工作目录（X-Project 头或 ?project= 查询参数），
// 且该目录已在全局注册表注册，否则返回 404 PROJECT_NOT_FOUND。
// 校验通过后将规范化 workdir 写入 ctx（auth.WithWorkdir），
// 供权限中间件（RequirePermission）与 handler 统一读取（QA P3-2）。
func (s *Server) projectMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workdir := projectFromRequest(r)
		if workdir == "" {
			writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND",
				"缺少项目标识：请携带 X-Project 请求头或 ?project= 查询参数（工作目录绝对路径）", "")
			return
		}
		// 路径规范化：Clean + Windows 大小写不敏感比较（QA 默认项）。
		workdir = filepath.Clean(workdir)
		ok, err := db.ProjectExists(r.Context(), s.registry, workdir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL",
				"项目注册校验失败", err.Error())
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND",
				"该目录尚未导入为项目，请先执行项目导入", workdir)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithWorkdir(r.Context(), workdir)))
	})
}
