// Package api 是 TangoForge 传输层：HTTP 路由与中间件（薄封装，不做业务判断）。
//
// 分层铁律（AGENTS.md §3.2）：本包只做参数解析与响应格式化，禁止重复业务逻辑；
// 业务实现由 task / project 等业务层提供，中间件所需的最小数据查询直接复用 db 包原生 SQL。
//
// TF-003 范围：/ping 健康检查、来源过滤（remote_access）、X-Project 注册校验、
// 端口热重载（QA Q8 完整热重载）；真实业务端点由 TF-013 挂载。
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"

	"tangoforge/internal/config"
)

// Server 守护进程 HTTP 服务：持有可热切换的全局配置与全局注册表库连接。
type Server struct {
	cfgMu sync.RWMutex
	cfg   *config.GlobalConfig

	registry *sql.DB
	logger   *slog.Logger

	httpSrv  *http.Server
	lnMu     sync.Mutex
	listener net.Listener
}

// NewServer 构造 HTTP 服务。
//
// cfg 为初始配置指针；热重载（setConfig / ReloadPort）会原子替换内部状态。
// registry 为全局注册表库连接（已迁移），用于 X-Project 注册校验。
func NewServer(cfg *config.GlobalConfig, registry *sql.DB, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		cfg:      cfg,
		registry: registry,
		logger:   logger,
	}
	s.httpSrv = &http.Server{Handler: s.Handler()}
	return s
}

// SetConfig 原子替换当前配置（remote_access 等内存标志立即生效；供热重载回调调用）。
func (s *Server) SetConfig(cfg *config.GlobalConfig) {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	s.cfg = cfg
}

// currentConfig 返回当前配置副本（供中间件读取，避免并发读写竞态）。
func (s *Server) currentConfig() config.GlobalConfig {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	if s.cfg == nil {
		return config.DefaultGlobalConfig()
	}
	return *s.cfg
}

// Handler 组装路由与中间件链。
//
// /ping 不经过任何中间件（健康检查）；/api/* 依次经过：
//  1. 来源过滤（非回环且未开启 remote_access → 403）；
//  2. X-Project 注册校验（未携带或未注册 → 404 PROJECT_NOT_FOUND）。
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Get("/ping", s.handlePing)

	r.Route("/api", func(r chi.Router) {
		r.Use(s.remoteAccessMiddleware)
		r.Use(s.projectMiddleware)
		// TF-013 起在此挂载真实业务端点；当前未匹配路由由 chi 默认 404 兜底。
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "api root: 无端点定义", "")
		})
	})
	return r
}

// addr 返回当前配置下的监听地址（始终 0.0.0.0，由中间件按来源 IP 动态过滤，QA Q8）。
func (s *Server) addr() string {
	return net.JoinHostPort("0.0.0.0", strconv.Itoa(s.currentConfig().Port))
}

// Serve 启动监听并服务请求（阻塞）。端口被占用时返回错误（单实例锁第二道防线）。
func (s *Server) Serve() error {
	ln, err := net.Listen("tcp", s.addr())
	if err != nil {
		return fmt.Errorf("listen %s: %w（可能已有守护进程在运行）", s.addr(), err)
	}
	s.lnMu.Lock()
	s.listener = ln
	s.lnMu.Unlock()
	s.logger.Info("daemon listening", "addr", s.addr())
	if err := s.httpSrv.Serve(ln); err != nil &&
		!errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}

// ReloadPort 热重载监听端口（QA Q8）：先监听新端口（失败则保留旧端口），
// 成功后将新 listener 交给 http.Server 服务并关闭旧 listener。
func (s *Server) ReloadPort(port int) error {
	if port <= 0 {
		return fmt.Errorf("invalid port %d", port)
	}
	addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("reload listen %s: %w", addr, err)
	}
	s.lnMu.Lock()
	old := s.listener
	s.listener = ln
	s.lnMu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	go func() { _ = s.httpSrv.Serve(ln) }()
	s.logger.Info("daemon port reloaded", "addr", addr)
	return nil
}

// Alive 报告是否仍有正在服务的监听器。
// 端口热重载替换主监听器后返回 true（守护进程继续常驻）。
func (s *Server) Alive() bool {
	s.lnMu.Lock()
	defer s.lnMu.Unlock()
	return s.listener != nil
}

// Shutdown 优雅关闭全部监听与连接。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

// handlePing 健康检查（GET /ping，不经过中间件）。
func (s *Server) handlePing(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]string{"status": "ok"},
	})
}

// projectFromRequest 从 X-Project 头或 ?project= 查询参数提取项目工作目录。
func projectFromRequest(r *http.Request) string {
	if p := r.Header.Get("X-Project"); p != "" {
		return p
	}
	return r.URL.Query().Get("project")
}

// isLoopback 判断来源地址是否为回环（127.0.0.1 / ::1）。
func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// writeJSON 写统一 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeError 写统一错误响应（QA Q9：{code, message, detail}，code 为业务错误码）。
func writeError(w http.ResponseWriter, status int, code, message, detail string) {
	body := map[string]string{
		"code":    code,
		"message": message,
	}
	if detail != "" {
		body["detail"] = detail
	}
	writeJSON(w, status, body)
}
