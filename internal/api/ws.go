package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"tangoforge/internal/auth"
	"tangoforge/internal/db"
)

// Event WebSocket 事件（REQUIREMENTS.md §5.3，结构 {type, project, data, ts}）。
type Event struct {
	Type    string `json:"type"`    // task.created / state_machine.changed / ...
	Project string `json:"project"` // 项目工作目录
	Data    any    `json:"data"`    // 事件载荷（任务 ID 等）
	TS      string `json:"ts"`      // RFC3339 本地时区
}

// hub 事件广播中心：按 project（workdir）分组维护 WS 连接。
type hub struct {
	mu    sync.Mutex
	rooms map[string]map[*wsClient]struct{}
}

// newHub 构造广播中心。
func newHub() *hub {
	return &hub{rooms: make(map[string]map[*wsClient]struct{})}
}

// wsClient 单个 WS 连接（发送缓冲 channel，写失败即断开）。
type wsClient struct {
	conn *websocket.Conn
	send chan []byte
}

// subscribe 注册连接。
func (h *hub) subscribe(workdir string, c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[workdir] == nil {
		h.rooms[workdir] = make(map[*wsClient]struct{})
	}
	h.rooms[workdir][c] = struct{}{}
}

// unsubscribe 注销连接；房间空则删除。
func (h *hub) unsubscribe(workdir string, c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m, ok := h.rooms[workdir]; ok {
		delete(m, c)
		if len(m) == 0 {
			delete(h.rooms, workdir)
		}
	}
}

// Publish 向项目房间广播事件（写钩子接入点，TF-014）。
// 发送失败（连接已断/缓冲满）的连接被移除。
func (h *hub) Publish(workdir, eventType string, data any) {
	ev := Event{Type: eventType, Project: workdir, Data: data, TS: time.Now().Format(time.RFC3339)}
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.rooms[workdir] {
		select {
		case c.send <- payload:
		default:
			// 缓冲满：连接过慢，关闭并移除（慢消费者保护）。
			close(c.send)
			delete(h.rooms[workdir], c)
			_ = c.conn.Close()
		}
	}
}

// upgrader WS 升级器（回环/远程均允许；鉴权由 handleWS 完成）。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// 跨源策略：v1 单机 App/CLI 场景，放行所有 Origin（鉴权靠 Token/回环）。
	CheckOrigin: func(_ *http.Request) bool { return true },
}

const wsWriteWait = 10 * time.Second
const wsPongWait = 60 * time.Second
const wsPingPeriod = (wsPongWait * 9) / 10

// handleWS WebSocket 事件订阅（GET /ws/events?project=<workdir>）。
//
// 鉴权链（与 /api 中间件语义一致，独立实现因为 WS 不在 /api 路由树下）：
//  1. remote_access=false 且非回环 → 403；
//  2. project 注册校验（query 参数）→ 404 PROJECT_NOT_FOUND；
//  3. 来源识别：回环+X-UI-Token→ui；远程须 Bearer（缺失/错误→401）；回环 X-Actor→agent；无→unknown；
//  4. 权限：ui 放行，其余查 task.read → 未授权 403；
//  5. 通过后升级为 WS 并订阅 project 房间。
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	// 1. 来源过滤（与 remoteAccessMiddleware 一致）。
	if !s.currentConfig().RemoteAccess && !isLoopback(r.RemoteAddr) {
		writeError(w, http.StatusForbidden, "REMOTE_ACCESS_DISABLED", "remote access disabled", "")
		return
	}
	// 2. 项目注册校验。
	workdir := filepath.Clean(r.URL.Query().Get("project"))
	if workdir == "" {
		writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND",
			"缺少项目标识：/ws/events?project=<工作目录绝对路径>", "")
		return
	}
	ok, err := db.ProjectExists(r.Context(), s.registry, workdir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "项目注册校验失败", err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "该目录尚未导入为项目", workdir)
		return
	}
	// 3. 来源识别。
	actor, needAuth := auth.Identify(s.currentConfigPtr(), r)
	if needAuth {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED",
			"远程连接必须携带有效的 Authorization: Bearer <api_token>", "")
		return
	}
	// 4. 权限：WS 建连校验项目 task.read（REQUIREMENTS.md §5 API 表）。
	if actor.Class != auth.ClassUI {
		allowed, err := s.perms.Allowed(r.Context(), workdir, "task.read")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "权限查询失败", err.Error())
			return
		}
		if !allowed {
			if s.perms.OnDenied != nil {
				s.perms.OnDenied(r.Context(), workdir, "task.read")
			}
			writeError(w, http.StatusForbidden, "PERMISSION_DENIED",
				"无权订阅事件（action=task.read）", actor.Class)
			return
		}
	}
	// 5. 升级 + 订阅。
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // 升级失败：响应已由 upgrader 写出
	}
	ctx := auth.WithActor(r.Context(), actor)
	client := &wsClient{conn: conn, send: make(chan []byte, 64)}
	s.hub.subscribe(workdir, client)
	s.logger.Info("ws connected", "project", workdir, "actor", actor.Name)

	// 读泵：处理 pong 与关闭；写泵：从 send 通道写消息。
	go s.wsWritePump(client)
	s.wsReadPump(ctx, workdir, client)
}

// wsReadPump 读取循环：保活（pong）+ 检测对端关闭；读线程退出后关闭连接。
func (s *Server) wsReadPump(_ context.Context, workdir string, c *wsClient) {
	defer func() {
		s.hub.unsubscribe(workdir, c)
		_ = c.conn.Close()
	}()
	_ = c.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

// wsWritePump 写入循环：定时 ping + 发送事件；send 关闭时终止。
func (s *Server) wsWritePump(c *wsClient) {
	ticker := time.NewTicker(wsPingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, more := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if !more {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
