package audit

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"tangoforge/internal/db"
	"tangoforge/internal/project"
	"testing"
	"time"
)

// newStore 构造审计存储（日志丢弃）。
func newStore() *Store {
	return NewStore(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// initProject 在临时目录初始化项目（meta.db 含 audit_log 表）。
func initProject(t *testing.T) string {
	t.Helper()
	registry, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	if err := db.Migrate(context.Background(), registry, db.GlobalMigrations); err != nil {
		t.Fatalf("migrate registry: %v", err)
	}
	svc := project.NewService(registry, slog.New(slog.NewTextHandler(io.Discard, nil)))
	dir := t.TempDir()
	if _, err := svc.Import(context.Background(), dir); err != nil {
		t.Fatalf("import project: %v", err)
	}
	return dir
}

// waitFor 轮询等待条件成立（异步落库验证）。
func waitFor(t *testing.T, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestWrite_AsyncPersists(t *testing.T) {
	dir := initProject(t)
	store := newStore()
	t.Cleanup(func() { _ = store.Close() })

	store.Write(t.Context(), dir, Entry{
		Actor: "human", ActorClass: "agent",
		Action: "task.created", Target: "task-1", Result: ResultOK,
	})
	waitFor(t, "audit persisted", func() bool {
		res, err := store.Query(t.Context(), dir, Filter{})
		return err == nil && res.Total == 1
	})

	res, err := store.Query(t.Context(), dir, Filter{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.Total != 1 || len(res.Items) != 1 {
		t.Fatalf("got total=%d items=%d, want 1/1", res.Total, len(res.Items))
	}
	e := res.Items[0]
	if e.Actor != "human" || e.ActorClass != "agent" || e.Action != "task.created" ||
		e.Target != "task-1" || e.Result != ResultOK {
		t.Fatalf("unexpected entry: %+v", e)
	}
	if e.Ts == "" {
		t.Error("ts should be auto-filled")
	}
}

func TestWrite_DeniedRecord(t *testing.T) {
	dir := initProject(t)
	store := newStore()
	t.Cleanup(func() { _ = store.Close() })

	store.Write(t.Context(), dir, Entry{
		Actor: "unknown", ActorClass: "unknown",
		Action: "task.create", Target: dir, Result: ResultDenied, Detail: "denied by permission",
	})
	waitFor(t, "denied record", func() bool {
		res, err := store.Query(t.Context(), dir, Filter{Action: "task.create"})
		return err == nil && res.Total == 1
	})
	res, _ := store.Query(t.Context(), dir, Filter{Action: "task.create"})
	if res.Items[0].Result != ResultDenied {
		t.Fatalf("want denied, got %s", res.Items[0].Result)
	}
}

func TestWrite_AsyncNonBlocking(t *testing.T) {
	dir := initProject(t)
	store := newStore()
	t.Cleanup(func() { _ = store.Close() })

	// 批量投递（channel 缓冲 1024），Write 必须快速返回。
	start := time.Now()
	for i := 0; i < 1000; i++ {
		store.Write(t.Context(), dir, Entry{
			Actor: "human", ActorClass: "agent",
			Action: "task.created", Target: "t", Result: ResultOK,
		})
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("bulk write took %v, should be non-blocking", elapsed)
	}
	waitFor(t, "1000 records", func() bool {
		res, err := store.Query(t.Context(), dir, Filter{})
		return err == nil && res.Total == 1000
	})
}

func TestQuery_FilterAndPagination(t *testing.T) {
	dir := initProject(t)
	store := newStore()
	t.Cleanup(func() { _ = store.Close() })

	for i := 0; i < 5; i++ {
		store.Write(t.Context(), dir, Entry{
			Actor: "human", ActorClass: "agent",
			Action: "task.created", Target: "t", Result: ResultOK,
		})
		store.Write(t.Context(), dir, Entry{
			Actor: "bot", ActorClass: "agent",
			Action: "task.updated", Target: "t", Result: ResultOK,
		})
	}
	waitFor(t, "10 records", func() bool {
		res, err := store.Query(t.Context(), dir, Filter{})
		return err == nil && res.Total == 10
	})

	// filter[actor]
	res, err := store.Query(t.Context(), dir, Filter{Actor: "human"})
	if err != nil || res.Total != 5 {
		t.Fatalf("actor filter: total=%d err=%v, want 5", res.Total, err)
	}
	// filter[action]
	res, err = store.Query(t.Context(), dir, Filter{Action: "task.updated"})
	if err != nil || res.Total != 5 {
		t.Fatalf("action filter: total=%d err=%v, want 5", res.Total, err)
	}
	// 分页：size=4 → 3 页（4/4/2）
	res, err = store.Query(t.Context(), dir, Filter{Page: 1, Size: 4})
	if err != nil || len(res.Items) != 4 || res.Total != 10 {
		t.Fatalf("page1: items=%d total=%d err=%v", len(res.Items), res.Total, err)
	}
	res, err = store.Query(t.Context(), dir, Filter{Page: 3, Size: 4})
	if err != nil || len(res.Items) != 2 {
		t.Fatalf("page3: items=%d err=%v, want 2", len(res.Items), err)
	}
	// 默认 size=100 / 上限 500
	res, _ = store.Query(t.Context(), dir, Filter{Page: 1})
	if res.Size != 100 {
		t.Fatalf("default size=%d, want 100", res.Size)
	}
	res, _ = store.Query(t.Context(), dir, Filter{Page: 1, Size: 9999})
	if res.Size != 500 {
		t.Fatalf("max size=%d, want 500", res.Size)
	}
}

func TestExport_Format(t *testing.T) {
	dir := initProject(t)
	store := newStore()
	t.Cleanup(func() { _ = store.Close() })

	store.Write(t.Context(), dir, Entry{
		Actor: "human", ActorClass: "agent",
		Action: "task.created", Target: "task-1", Result: ResultOK,
	})
	waitFor(t, "export record", func() bool {
		out, err := store.Export(t.Context(), dir)
		return err == nil && strings.Count(out, "\n") == 2 // 表头 + 1 行
	})

	out, err := store.Export(t.Context(), dir)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if lines[0] != "ts|actor|actor_class|action|target|result|detail" {
		t.Fatalf("bad header: %s", lines[0])
	}
	// 行以 ts 开头，随后依次是 actor|actor_class|action|target|result|detail。
	if !strings.HasPrefix(lines[1], "2026-") ||
		!strings.Contains(lines[1], "|human|agent|task.created|task-1|ok|") {
		t.Fatalf("bad row: %s", lines[1])
	}
}

func TestClose_DrainsQueue(t *testing.T) {
	dir := initProject(t)
	store := newStore()

	for i := 0; i < 10; i++ {
		store.Write(t.Context(), dir, Entry{
			Actor: "human", ActorClass: "agent",
			Action: "task.created", Target: "t", Result: ResultOK,
		})
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// 关闭后直接查库验证排空。
	conn, err := db.Open(db.MetaDBPath(dir))
	if err != nil {
		t.Fatalf("open meta.db: %v", err)
	}
	defer func() { _ = conn.Close() }()
	var n int
	if err := conn.QueryRow("SELECT COUNT(*) FROM audit_log").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 10 {
		t.Fatalf("drained count=%d, want 10", n)
	}
}

func TestProjectNotFound(t *testing.T) {
	store := newStore()
	t.Cleanup(func() { _ = store.Close() })
	missing := t.TempDir()
	_, err := store.Query(t.Context(), missing, Filter{})
	if err == nil {
		t.Fatal("unregistered dir query must fail")
	}
	if !strings.Contains(err.Error(), "project not found") {
		t.Fatalf("want project not found, got %v", err)
	}
}

func TestWrite_DefaultResultOK(t *testing.T) {
	dir := initProject(t)
	store := newStore()
	t.Cleanup(func() { _ = store.Close() })
	store.Write(t.Context(), dir, Entry{Actor: "x", ActorClass: "agent", Action: "task.created"})
	waitFor(t, "record", func() bool {
		res, err := store.Query(t.Context(), dir, Filter{})
		return err == nil && res.Total == 1
	})
	res, _ := store.Query(t.Context(), dir, Filter{})
	if res.Items[0].Result != ResultOK {
		t.Fatalf("default result=%s, want ok", res.Items[0].Result)
	}
}
