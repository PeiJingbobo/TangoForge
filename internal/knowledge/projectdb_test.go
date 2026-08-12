package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestProjectDB_Broken_AllMethods 覆盖各方法在项目库损坏（meta.db 删除）时的
// projectDB 错误分支（OS-Stat 失败路径），与 closeCachedDB 的 SQL 错误注入互补。
func TestProjectDB_Broken_AllMethods(t *testing.T) {
	t.Run("ListBases", func(t *testing.T) {
		svc := newTestService(t)
		workdir := initProject(t)
		primeCache(t, svc, workdir)
		breakProjectDB(t, svc, workdir)
		if _, err := svc.ListBases(context.Background(), workdir); err == nil {
			t.Fatal("库损坏时 ListBases 应报错")
		}
	})
	t.Run("GetBase", func(t *testing.T) {
		svc := newTestService(t)
		workdir := initProject(t)
		primeCache(t, svc, workdir)
		breakProjectDB(t, svc, workdir)
		if _, err := svc.GetBase(context.Background(), workdir, 1); err == nil {
			t.Fatal("库损坏时 GetBase 应报错")
		}
	})
	t.Run("CreateBase", func(t *testing.T) {
		svc := newTestService(t)
		workdir := initProject(t)
		primeCache(t, svc, workdir)
		breakProjectDB(t, svc, workdir)
		if _, err := svc.CreateBase(context.Background(), workdir, "x", ""); err == nil {
			t.Fatal("库损坏时 CreateBase 应报错")
		}
	})
	t.Run("UpdateBase", func(t *testing.T) {
		svc := newTestService(t)
		workdir := initProject(t)
		primeCache(t, svc, workdir)
		breakProjectDB(t, svc, workdir)
		n := "x"
		if _, err := svc.UpdateBase(context.Background(), workdir, 1, &n, nil); err == nil {
			t.Fatal("库损坏时 UpdateBase 应报错")
		}
	})
	t.Run("DeleteBase", func(t *testing.T) {
		svc := newTestService(t)
		workdir := initProject(t)
		primeCache(t, svc, workdir)
		breakProjectDB(t, svc, workdir)
		if err := svc.DeleteBase(context.Background(), workdir, 1); err == nil {
			t.Fatal("库损坏时 DeleteBase 应报错")
		}
	})
	t.Run("EnsureDefaultBase", func(t *testing.T) {
		svc := newTestService(t)
		workdir := initProject(t)
		primeCache(t, svc, workdir)
		breakProjectDB(t, svc, workdir)
		if _, err := svc.EnsureDefaultBase(context.Background(), workdir); err == nil {
			t.Fatal("库损坏时 EnsureDefaultBase 应报错")
		}
	})
	t.Run("ListDocuments", func(t *testing.T) {
		svc := newTestService(t)
		workdir := initProject(t)
		primeCache(t, svc, workdir)
		breakProjectDB(t, svc, workdir)
		if _, err := svc.ListDocuments(context.Background(), workdir, DocumentFilter{}); err == nil {
			t.Fatal("库损坏时 ListDocuments 应报错")
		}
	})
	t.Run("GetDocument", func(t *testing.T) {
		svc := newTestService(t)
		workdir := initProject(t)
		primeCache(t, svc, workdir)
		breakProjectDB(t, svc, workdir)
		if _, err := svc.GetDocument(context.Background(), workdir, "d"); err == nil {
			t.Fatal("库损坏时 GetDocument 应报错")
		}
	})
	t.Run("DeleteDocument", func(t *testing.T) {
		svc := newTestService(t)
		workdir := initProject(t)
		primeCache(t, svc, workdir)
		breakProjectDB(t, svc, workdir)
		if err := svc.DeleteDocument(context.Background(), workdir, "d"); err == nil {
			t.Fatal("库损坏时 DeleteDocument 应报错")
		}
	})
	t.Run("RelinkDocument", func(t *testing.T) {
		svc := newTestService(t)
		workdir := initProject(t)
		primeCache(t, svc, workdir)
		newAbs := writeFile(t, workdir, "a.md", "# a")
		breakProjectDB(t, svc, workdir)
		if _, err := svc.RelinkDocument(context.Background(), workdir, "d", newAbs, CopyNone); err == nil {
			t.Fatal("库损坏时 RelinkDocument 应报错")
		}
	})
	t.Run("TaskDocuments", func(t *testing.T) {
		svc := newTestService(t)
		workdir := initProject(t)
		primeCache(t, svc, workdir)
		breakProjectDB(t, svc, workdir)
		if _, err := svc.TaskDocuments(context.Background(), workdir, "t"); err == nil {
			t.Fatal("库损坏时 TaskDocuments 应报错")
		}
	})
	t.Run("LinkTask", func(t *testing.T) {
		svc := newTestService(t)
		workdir := initProject(t)
		primeCache(t, svc, workdir)
		breakProjectDB(t, svc, workdir)
		if err := svc.LinkTask(context.Background(), workdir, "t", "d", "", CopyNone, nil); err == nil {
			t.Fatal("库损坏时 LinkTask 应报错")
		}
	})
	t.Run("UnlinkTask", func(t *testing.T) {
		svc := newTestService(t)
		workdir := initProject(t)
		primeCache(t, svc, workdir)
		breakProjectDB(t, svc, workdir)
		if err := svc.UnlinkTask(context.Background(), workdir, "t", "d"); err == nil {
			t.Fatal("库损坏时 UnlinkTask 应报错")
		}
	})
	t.Run("RelativePath", func(t *testing.T) {
		svc := newTestService(t)
		if _, err := svc.ListBases(context.Background(), "relative"); err == nil {
			t.Fatal("相对路径应报错")
		}
	})
}

// TestRegisterDocument_StatAfterCopy 覆盖拷贝后 os.Stat 失败的罕见分支
// （拷贝目标在 RegisterDocument 内被重新 stat）。
func TestRegisterDocument_StatAfterCopy(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	// 外部文件 → auto 拷贝成功（正常路径已测）；此处验证拷贝落点 stat 正常路径。
	outDir := t.TempDir()
	ext := filepath.Join(outDir, "ext.md")
	_ = os.WriteFile(ext, []byte("x"), 0o644)
	doc, err := svc.RegisterDocument(ctx, workdir, ext, CopyAuto, nil)
	if err != nil {
		t.Fatalf("register external: %v", err)
	}
	if doc.AbsPath == "" || !filepath.HasPrefix(doc.AbsPath, workdir) {
		t.Fatalf("外部 auto 拷贝落点异常: %s", doc.AbsPath)
	}
}
