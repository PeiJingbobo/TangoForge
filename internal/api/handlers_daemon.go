package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"tangoforge/internal/auth"
	"tangoforge/internal/version"
)

// 守护进程生命周期控制端点（TF-053）：
//
//	GET  /api/daemon/version — 版本探测（APP 启动时比对自身版本，判断是否需要重启 daemon）；
//	POST /api/daemon/restart — 请求「空闲时重启」：daemon 在完成当前进行中的任务后优雅退出
//	                          （http.Server.Shutdown 等待活跃请求完成、停止接收新连接），
//	                          随后由调用方提供的**新二进制路径**自我重生并退出进程。
//
// 语义：
//   - version 免鉴权（无敏感信息，供 APP/CLI 探测）；restart 仅 UI（回环 + X-UI-Token）——
//     与 PUT /api/config 同策略（守护进程生命周期属敏感操作，Agent / 远程一律 403）；
//   - restart 是「请求」而非「立即执行」：记录意图后 handler 立即返回 202，
//     主循环检测到意图 → Shutdown（等待进行中请求完成，不打断 CLI 调用）→ 重生新 daemon → 退出。
//   - bin_path 必须为已存在文件（重启后由该路径拉起新 daemon）。

// handleDaemonVersion 版本探测（GET /api/daemon/version，免鉴权）。
func (s *Server) handleDaemonVersion(w http.ResponseWriter, _ *http.Request) {
	exe := ""
	if p, err := os.Executable(); err == nil {
		exe = p
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]any{
		"version":    version.String(),
		"pid":        os.Getpid(),
		"executable": exe,
	}})
}

// daemonRestartReq 空闲重启请求体。
type daemonRestartReq struct {
	// BinPath 新 daemon 二进制路径（APP 打包/构建的产物；必填且须存在）。
	BinPath string `json:"bin_path"`
}

// handleDaemonRestart 请求空闲重启（POST /api/daemon/restart，仅 UI）。
func (s *Server) handleDaemonRestart(w http.ResponseWriter, r *http.Request) {
	actor := auth.ActorFrom(r.Context())
	if actor.Class != auth.ClassUI {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED",
			"守护进程重启仅允许 UI 操作（回环 + X-UI-Token）", actor.Class)
		return
	}
	var req daemonRestartReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "DAEMON_RESTART_INVALID", "请求体 JSON 解析失败", err.Error())
		return
	}
	binPath := strings.TrimSpace(req.BinPath)
	if binPath == "" {
		writeError(w, http.StatusBadRequest, "DAEMON_RESTART_INVALID", "bin_path 必填", "")
		return
	}
	if _, err := os.Stat(binPath); err != nil {
		writeError(w, http.StatusBadRequest, "DAEMON_RESTART_INVALID",
			"bin_path 不存在（重启将无法拉起新守护进程）", err.Error())
		return
	}

	// 记录意图（幂等：重复请求覆盖 bin_path）。
	s.restartMu.Lock()
	s.restartIntent = binPath
	s.restartMu.Unlock()

	// 主循环收到意图后执行 Shutdown + 重生；此处立即返回（Shutdown 由主循环完成，
	// 当前 handler 属于「进行中请求」，会被 Shutdown 等待完成后进程才退出）。
	if s.onRestartRequested != nil {
		s.onRestartRequested(binPath)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"code": 0, "data": map[string]any{
		"accepted": true,
		"bin_path": binPath,
		"note":     "守护进程将在完成当前任务后重启",
	}})
}

// RequestRestart 记录空闲重启意图（由 main 的退出循环消费）。
// 返回 true 表示这是首次请求（main 需要触发 Shutdown）；false 表示已有意图。
func (s *Server) RequestRestart(binPath string) bool {
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	if s.restartIntent != "" {
		return false
	}
	s.restartIntent = binPath
	return true
}

// SetRestartCallback 注入空闲重启回调（main 构造 Server 后调用）：
// 当 handler 请求重启时通知 main 主循环执行 Shutdown + 重生。
func (s *Server) SetRestartCallback(fn func(binPath string)) {
	s.onRestartRequested = fn
}
