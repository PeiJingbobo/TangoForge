package knowledge

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"tangoforge/internal/db"
)

// newTestService 构造知识库服务（sqlite 临时文件库 + 空日志）。
func newTestService(t *testing.T) Service {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(Options{Logger: logger})
}

// newTestServiceWithTasks 构造带任务校验服务的知识库服务。
func newTestServiceWithTasks(t *testing.T, tasks TaskLister) Service {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(Options{Logger: logger, Tasks: tasks})
}

// initProject 初始化临时项目（meta.db 全表 + 默认库），返回 workdir。
func initProject(t *testing.T) string {
	t.Helper()
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".taskboard"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	conn, err := db.EnsureProject(context.Background(), db.MetaDBPath(workdir))
	if err != nil {
		t.Fatalf("ensure project: %v", err)
	}
	_ = conn.Close()
	// 模拟项目初始化：创建默认库（真实流程由 project.initProjectDir.ensureDefaultKB 完成）。
	svc := NewService(Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if _, err := svc.EnsureDefaultBase(context.Background(), workdir); err != nil {
		t.Fatalf("ensure default base: %v", err)
	}
	_ = svc.Close()
	return workdir
}

// writeFile 在 workdir 下写入文本文件（自动建目录），返回绝对路径。
func writeFile(t *testing.T, workdir, rel string, content string) string {
	t.Helper()
	abs := filepath.Join(workdir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return abs
}

// fakeTaskLister 实现 TaskLister 的桩（任意 id 视为存在）。
type fakeTaskLister struct{}

func (fakeTaskLister) Get(ctx context.Context, workdir, id string) (any, error) {
	return nil, nil
}

// discardLogger 返回静默日志器。
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mustProjectDB 打开项目 meta.db 连接（测试断言底层数据用）。
func mustProjectDB(t *testing.T, svc Service, workdir string) *sql.DB {
	t.Helper()
	conn, err := db.Open(db.MetaDBPath(workdir))
	if err != nil {
		t.Fatalf("open meta.db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
