package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"tangoforge/internal/db"
)

// breakProjectDB 删除项目 meta.db（含 WAL/SHM），使后续 projectDB 打开失败
// （指纹校验失效 → os.Stat 失败 → 返回错误），触发 projectDB 错误分支。
func breakProjectDB(t *testing.T, svc Service, workdir string) {
	t.Helper()
	s := svc.(*service)
	s.mu.Lock()
	if conn, ok := s.dbs[filepath.Clean(workdir)]; ok {
		_ = conn.Close()
		delete(s.dbs, filepath.Clean(workdir))
		delete(s.fp, filepath.Clean(workdir))
	}
	s.mu.Unlock()
	_ = os.Remove(db.MetaDBPath(workdir))
	_ = os.Remove(db.MetaDBPath(workdir) + "-wal")
	_ = os.Remove(db.MetaDBPath(workdir) + "-shm")
}

// closeCachedDB 关闭服务缓存的连接（保留缓存项与文件指纹），
// 使后续 SQL 调用命中已关闭连接 → 触发 SQL 执行错误分支。
// 调用前须先建立缓存连接（如 ListBases 一次）。
func closeCachedDB(t *testing.T, svc Service, workdir string) {
	t.Helper()
	s := svc.(*service)
	s.mu.Lock()
	defer s.mu.Unlock()
	if conn, ok := s.dbs[filepath.Clean(workdir)]; ok {
		_ = conn.Close()
	}
}

// primeCache 建立缓存连接（调用一次 ListBases）。
func primeCache(t *testing.T, svc Service, workdir string) {
	t.Helper()
	if _, err := svc.ListBases(context.Background(), workdir); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
}

func TestNewService_NilLogger(t *testing.T) {
	svc := NewService(Options{})
	if svc == nil {
		t.Fatal("nil logger 也应返回服务")
	}
	_ = svc.Close()
}

func TestDeleteBase_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	kb, err := svc.CreateBase(ctx, workdir, "spec", "")
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	closeCachedDB(t, svc, workdir)
	if err := svc.DeleteBase(ctx, workdir, kb.ID); err == nil {
		t.Fatal("连接关闭后删除库应报错")
	}
}

func TestDeleteDocument_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	closeCachedDB(t, svc, workdir)
	if err := svc.DeleteDocument(ctx, workdir, doc.ID); err == nil {
		t.Fatal("连接关闭后删除文档应报错")
	}
}

func TestGetDocument_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	closeCachedDB(t, svc, workdir)
	if _, err := svc.GetDocument(ctx, workdir, doc.ID); err == nil {
		t.Fatal("连接关闭后查询文档应报错")
	}
}

func TestLinkTask_TaskValidationError(t *testing.T) {
	svc := NewService(Options{Logger: discardLogger(), Tasks: errTaskLister{}})
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := svc.LinkTask(ctx, workdir, "task-x", doc.ID, "", CopyAuto, nil); err == nil {
		t.Fatal("任务校验失败应报错")
	}
}

func TestUnlinkTask_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	closeCachedDB(t, svc, workdir)
	if err := svc.UnlinkTask(ctx, workdir, "task-x", doc.ID); err == nil {
		t.Fatal("连接关闭后 unlink 应报错")
	}
}

func TestTaskDocuments_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	primeCache(t, svc, workdir)
	closeCachedDB(t, svc, workdir)
	if _, err := svc.TaskDocuments(ctx, workdir, "task-x"); err == nil {
		t.Fatal("连接关闭后查询任务文档应报错")
	}
}

func TestListDocuments_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	primeCache(t, svc, workdir)
	closeCachedDB(t, svc, workdir)
	if _, err := svc.ListDocuments(ctx, workdir, DocumentFilter{}); err == nil {
		t.Fatal("连接关闭后列表应报错")
	}
}

func TestRegisterDocument_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	abs := writeFile(t, workdir, "a.md", "# a")
	primeCache(t, svc, workdir)
	closeCachedDB(t, svc, workdir)
	if _, err := svc.RegisterDocument(ctx, workdir, abs, CopyAuto, nil); err == nil {
		t.Fatal("连接关闭后注册应报错")
	}
}

func TestRelink_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	newAbs := writeFile(t, workdir, "b.md", "# b")
	closeCachedDB(t, svc, workdir)
	if _, err := svc.RelinkDocument(ctx, workdir, doc.ID, newAbs, CopyNone); err == nil {
		t.Fatal("连接关闭后 relink 应报错")
	}
}

func TestUpdateBase_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	kb, err := svc.CreateBase(ctx, workdir, "spec", "")
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	closeCachedDB(t, svc, workdir)
	name := "x"
	if _, err := svc.UpdateBase(ctx, workdir, kb.ID, &name, nil); err == nil {
		t.Fatal("连接关闭后更新库应报错")
	}
}

func TestGetBase_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	primeCache(t, svc, workdir)
	closeCachedDB(t, svc, workdir)
	if _, err := svc.GetBase(ctx, workdir, 1); err == nil {
		t.Fatal("连接关闭后查询库应报错")
	}
}

func TestCreateBase_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	primeCache(t, svc, workdir)
	closeCachedDB(t, svc, workdir)
	if _, err := svc.CreateBase(ctx, workdir, "spec", ""); err == nil {
		t.Fatal("连接关闭后创建库应报错")
	}
}

func TestEnsureDefaultBase_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	primeCache(t, svc, workdir)
	closeCachedDB(t, svc, workdir)
	if _, err := svc.EnsureDefaultBase(ctx, workdir); err == nil {
		t.Fatal("连接关闭后 ensure 应报错")
	}
}

func TestListBases_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	primeCache(t, svc, workdir)
	closeCachedDB(t, svc, workdir)
	if _, err := svc.ListBases(ctx, workdir); err == nil {
		t.Fatal("连接关闭后列表库应报错")
	}
}

func TestClose_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	if _, err := svc.ListBases(context.Background(), workdir); err != nil {
		t.Fatalf("list: %v", err)
	}
	// Close 后再次 Close 不应 panic（幂等容错）。
	_ = svc.Close()
	_ = svc.Close()
}

// errTaskLister 任务校验永远失败。
type errTaskLister struct{}

func (errTaskLister) Get(ctx context.Context, workdir, id string) (any, error) {
	return nil, errors.New("task not found")
}
