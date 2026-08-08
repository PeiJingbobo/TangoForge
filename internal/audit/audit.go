// Package audit 负责审计日志的异步写入与导出。
//
// 约束（docs/TECHNICAL.md §3.6「审计日志（不可篡改）」）：
//   - 所有写操作（Create / Update / Archive / Restore / StatusChange / Import /
//     Export / 权限与状态机修改）必须异步写入 audit_log 表（数据本体）；
//     audit.log 文件仅为按需导出物；
//   - 审计字段：id, ts, actor, actor_class, action, target, result, detail；
//     仅写操作记录，读取不记录；
//   - 本包只提供写入 / 查询 / 导出，**无更新端点**（表只追加）。
//
// 接入方式（QA P3-1）：api 层将 task.Service 的 OnWrite 回调指向本包 Write，
// 权限中间件的 OnDenied 回调同样指向本包（result=denied）；
// actor / actor_class 由调用方从 ctx（auth.ActorFrom）读取后显式传入。
package audit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"tangoforge/internal/db"
	"time"
)

// Entry 一条审计记录（对应 audit_log 行）。
type Entry struct {
	Ts         string `json:"ts"` // RFC3339 本地时区
	Actor      string `json:"actor"`
	ActorClass string `json:"actor_class"` // ui / agent / unknown
	Action     string `json:"action"`      // task.created / ... / 权限 action
	Target     string `json:"target"`      // 任务 ID / workdir / 空
	Result     string `json:"result"`      // ok / denied / error
	Detail     string `json:"detail"`
}

// Result 取值。
const (
	ResultOK     = "ok"
	ResultDenied = "denied"
	ResultError  = "error"
)

// Store 异步审计写入器：channel + 单消费者 goroutine，写库不阻塞业务。
//
// 可靠性（QA 默认项）：缓冲 channel（1024），投递失败（满）时回退同步写，
// 保序不丢；Close 排空队列后退出。
type Store struct {
	mu      sync.Mutex
	dbs     map[string]*sql.DB
	logger  *slog.Logger
	entries chan entryMsg
	done    chan struct{}
	wg      sync.WaitGroup
}

type entryMsg struct {
	workdir string
	entry   Entry
}

// NewStore 构造异步审计存储并启动消费者 goroutine。
func NewStore(logger *slog.Logger) *Store {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Store{
		dbs:     make(map[string]*sql.DB),
		logger:  logger,
		entries: make(chan entryMsg, 1024),
		done:    make(chan struct{}),
	}
	s.wg.Add(1)
	go s.loop()
	return s
}

// projectDB 打开并缓存项目库连接（语义同 task.Service.projectDB）。
func (s *Store) projectDB(workdir string) (*sql.DB, error) {
	clean := filepath.Clean(workdir)
	if !filepath.IsAbs(clean) {
		return nil, fmt.Errorf("%w: %s 不是绝对路径", ErrProjectNotFound, workdir)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if conn, ok := s.dbs[clean]; ok {
		return conn, nil
	}
	if _, err := os.Stat(db.MetaDBPath(clean)); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProjectNotFound, workdir)
	}
	conn, err := db.EnsureProject(context.Background(), db.MetaDBPath(clean))
	if err != nil {
		return nil, fmt.Errorf("audit: open project db %s: %w", clean, err)
	}
	s.dbs[clean] = conn
	return conn, nil
}

// Write 投递一条审计记录（异步，不阻塞业务）。
// actor / actor_class 由调用方显式传入（api 层从 auth.ActorFrom 读取）。
func (s *Store) Write(ctx context.Context, workdir string, e Entry) {
	if e.Ts == "" {
		e.Ts = time.Now().Format(time.RFC3339)
	}
	if e.Result == "" {
		e.Result = ResultOK
	}
	msg := entryMsg{workdir: workdir, entry: e}
	select {
	case s.entries <- msg:
		// 正常投递。
	default:
		// channel 满：回退同步写（保序不丢，极端流量下短暂阻塞可接受）。
		if err := s.insert(ctx, msg); err != nil {
			s.logger.Error("audit sync write failed", "err", err)
		}
	}
}

// loop 消费者：从 channel 取条目写入项目库。
func (s *Store) loop() {
	defer s.wg.Done()
	for {
		select {
		case msg := <-s.entries:
			if err := s.insert(context.Background(), msg); err != nil {
				s.logger.Error("audit write failed", "err", err, "action", msg.entry.Action)
			}
		case <-s.done:
			// 排空剩余队列后退出。
			for {
				select {
				case msg := <-s.entries:
					if err := s.insert(context.Background(), msg); err != nil {
						s.logger.Error("audit drain write failed", "err", err)
					}
				default:
					return
				}
			}
		}
	}
}

// insert 单条落库（INSERT，只追加）。
func (s *Store) insert(ctx context.Context, msg entryMsg) error {
	conn, err := s.projectDB(msg.workdir)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx,
		`INSERT INTO audit_log (ts, actor, actor_class, action, target, result, detail)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		msg.entry.Ts, msg.entry.Actor, msg.entry.ActorClass,
		msg.entry.Action, msg.entry.Target, msg.entry.Result, msg.entry.Detail)
	if err != nil {
		return fmt.Errorf("audit: insert %s: %w", msg.entry.Action, err)
	}
	return nil
}

// Filter 审计查询过滤条件。
type Filter struct {
	Actor  string // 精确匹配
	Action string // 精确匹配
	Page   int    // 从 1 起；<=0 表示全量
	Size   int    // 默认 100，上限 500
}

// QueryResult 查询结果（ts 倒序）。
type QueryResult struct {
	Items []Entry `json:"items"`
	Total int     `json:"total"`
	Page  int     `json:"page"`
	Size  int     `json:"size"`
}

// Query 查询审计记录（filter[actor] / filter[action] + 分页，ts 倒序）。
func (s *Store) Query(ctx context.Context, workdir string, f Filter) (QueryResult, error) {
	conn, err := s.projectDB(workdir)
	if err != nil {
		return QueryResult{}, err
	}

	var conds []string
	var args []any
	if f.Actor != "" {
		conds = append(conds, "actor = ?")
		args = append(args, f.Actor)
	}
	if f.Action != "" {
		conds = append(conds, "action = ?")
		args = append(args, f.Action)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_log"+where, args...).Scan(&total); err != nil {
		return QueryResult{}, fmt.Errorf("audit: count: %w", err)
	}

	size := f.Size
	if size <= 0 {
		size = 100
	}
	if size > 500 {
		size = 500
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	start := (page - 1) * size

	rows, err := conn.QueryContext(ctx,
		"SELECT ts, actor, actor_class, action, target, result, detail FROM audit_log"+
			where+" ORDER BY id DESC, ts DESC LIMIT ? OFFSET ?",
		append(args, size, start)...)
	if err != nil {
		return QueryResult{}, fmt.Errorf("audit: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Entry, 0, size)
	for rows.Next() {
		var e Entry
		var detail sql.NullString
		if err := rows.Scan(&e.Ts, &e.Actor, &e.ActorClass, &e.Action, &e.Target, &e.Result, &detail); err != nil {
			return QueryResult{}, fmt.Errorf("audit: scan: %w", err)
		}
		if detail.Valid {
			e.Detail = detail.String
		}
		items = append(items, e)
	}
	return QueryResult{Items: items, Total: total, Page: page, Size: size}, rows.Err()
}

// Export 生成 audit.log 文本（按需导出物；数据本体仍为 audit_log 表）。
// 行格式：ts|actor|actor_class|action|target|result|detail（含表头）。
func (s *Store) Export(ctx context.Context, workdir string) (string, error) {
	conn, err := s.projectDB(workdir)
	if err != nil {
		return "", err
	}
	rows, err := conn.QueryContext(ctx,
		`SELECT ts, actor, actor_class, action, target, result, detail FROM audit_log ORDER BY id ASC, ts ASC`)
	if err != nil {
		return "", fmt.Errorf("audit: export query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var b strings.Builder
	b.WriteString("ts|actor|actor_class|action|target|result|detail\n")
	for rows.Next() {
		var e Entry
		var detail sql.NullString
		if err := rows.Scan(&e.Ts, &e.Actor, &e.ActorClass, &e.Action, &e.Target, &e.Result, &detail); err != nil {
			return "", fmt.Errorf("audit: export scan: %w", err)
		}
		d := ""
		if detail.Valid {
			d = detail.String
		}
		fmt.Fprintf(&b, "%s|%s|%s|%s|%s|%s|%s\n",
			e.Ts, e.Actor, e.ActorClass, e.Action, e.Target, e.Result, d)
	}
	return b.String(), rows.Err()
}

// Close 停止消费者并排空队列。
func (s *Store) Close() error {
	close(s.done)
	s.wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for wd, conn := range s.dbs {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("audit: close %s: %w", wd, err)
		}
		delete(s.dbs, wd)
	}
	return firstErr
}

// ErrProjectNotFound 供调用方识别项目未导入（api 层映射 404）。
var ErrProjectNotFound = errors.New("audit: project not found")
