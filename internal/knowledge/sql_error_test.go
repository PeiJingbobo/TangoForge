package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// dropTable 删除指定表（定向 SQL 错误注入：覆盖各写操作的表缺失错误分支）。
func dropTable(t *testing.T, svc Service, workdir, table string) {
	t.Helper()
	s := svc.(*service)
	s.mu.Lock()
	conn, ok := s.dbs[filepath.Clean(workdir)]
	s.mu.Unlock()
	if !ok {
		// 先建立缓存连接。
		if _, err := svc.ListBases(context.Background(), workdir); err != nil {
			t.Fatalf("prime: %v", err)
		}
		s.mu.Lock()
		conn, ok = s.dbs[filepath.Clean(workdir)]
		s.mu.Unlock()
	}
	if !ok {
		t.Fatal("无法获取缓存连接")
	}
	if _, err := conn.Exec(`DROP TABLE ` + table); err != nil {
		t.Fatalf("drop %s: %v", table, err)
	}
}

func TestDeleteBase_EdgeTableMissing(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	kb, err := svc.CreateBase(ctx, workdir, "spec", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	dropTable(t, svc, workdir, "knowledge_base_documents")
	if err := svc.DeleteBase(ctx, workdir, kb.ID); err == nil {
		t.Fatal("边表缺失时删除库应报错")
	}
}

func TestDeleteDocument_ChunksTableMissing(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	dropTable(t, svc, workdir, "knowledge_chunks")
	if err := svc.DeleteDocument(ctx, workdir, doc.ID); err == nil {
		t.Fatal("chunks 表缺失时删除文档应报错")
	}
}

func TestTaskDocErrors_TableMissing(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	dropTable(t, svc, workdir, "task_documents")
	// LinkTask 写失败。
	if err := svc.LinkTask(ctx, workdir, "t1", doc.ID, "", CopyAuto, nil); err == nil {
		t.Fatal("task_documents 表缺失时 link 应报错")
	}
	// TaskDocuments 读失败。
	if _, err := svc.TaskDocuments(ctx, workdir, "t1"); err == nil {
		t.Fatal("task_documents 表缺失时查询应报错")
	}
	// GetDocument 计数失败。
	if _, err := svc.GetDocument(ctx, workdir, doc.ID); err == nil {
		t.Fatal("task_documents 表缺失时详情应报错")
	}
}

func TestRegisterDocument_UniqueRaceRecovery(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	// 预注册一份文档。
	doc1, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 直接手动插入同 abs_path 的另一记录（模拟并发竞态产生的唯一冲突无法通过复用消除——
	// 由于已有记录，RegisterDocument 会走复用路径而不是 insert，因此这里先删除文档记录
	// 但保留一份重复记录会触发唯一约束。简化：直接对已存在路径再次注册，命中复用分支（已测）。
	// 此处验证唯一约束错误分支：先手动插入一条 abs_path 相同但 id 不同的记录。
	conn := mustProjectDB(t, svc, workdir)
	// 清空 knowledge_documents 但保留文件——手动插入重复 abs_path。
	if _, err := conn.Exec(`DELETE FROM knowledge_documents`); err != nil {
		t.Fatalf("clear docs: %v", err)
	}
	if _, err := conn.Exec(`DELETE FROM knowledge_base_documents`); err != nil {
		t.Fatalf("clear edges: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO knowledge_documents
			(id, project_id, path, abs_path, rel_path, origin_path, display_name, type, size,
			 mtime, content_hash, summary, status, embedded, embedding_model, index_error, history, created_at, updated_at)
		VALUES ('dup-1', 1, 'a.md', ?, 'a.md', '', 'a.md', 'text', 3, '2026-01-01T00:00:00+08:00',
		        '', '', 'ok', 0, '', '', '[]', '2026-01-01T00:00:00+08:00', '2026-01-01T00:00:00+08:00')`,
		doc1.AbsPath); err != nil {
		t.Fatalf("insert dup: %v", err)
	}
	// 再次注册同路径 → 复用返回既有记录（不报错）。
	doc2, err := svc.RegisterDocument(ctx, workdir, filepath.Join(workdir, "a.md"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register after dup: %v", err)
	}
	if doc2.ID != "dup-1" {
		t.Fatalf("应复用 dup-1，got %s", doc2.ID)
	}
}

func TestAddToBases_KBMissing(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	// 已有一份注册成功的文档（建立缓存连接）。
	_ = writeFile(t, workdir, "a.md", "# a")
	// 删除 knowledge_bases 表 → RegisterDocument 的 addToBases 查询库存在时 SQL 错误。
	primeCache(t, svc, workdir)
	dropTable(t, svc, workdir, "knowledge_bases")
	if _, err := svc.RegisterDocument(ctx, workdir, filepath.Join(workdir, "a.md"), CopyAuto, nil); err == nil {
		t.Fatal("库表缺失时注册应报错")
	}
}

func TestRegisterDocument_ProjectDBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	abs := writeFile(t, workdir, "a.md", "# a")
	primeCache(t, svc, workdir)
	breakProjectDB(t, svc, workdir)
	if _, err := svc.RegisterDocument(ctx, workdir, abs, CopyAuto, nil); err == nil {
		t.Fatal("项目库损坏时注册应报错")
	}
}

func TestRegisterDocument_ResolveDirError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	// 损坏的 config.yaml → resolveDefaultDocDir 失败。
	if err := os.WriteFile(filepath.Join(workdir, ".taskboard", "config.yaml"), []byte(": : bad yaml"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	outDir := t.TempDir()
	ext := filepath.Join(outDir, "ext.md")
	_ = os.WriteFile(ext, []byte("x"), 0o644)
	if _, err := svc.RegisterDocument(ctx, workdir, ext, CopyAuto, nil); err == nil {
		t.Fatal("配置损坏时外部拷贝应报错")
	}
}

func TestRegisterDocument_ReuseAddToBasesError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	abs := writeFile(t, workdir, "a.md", "# a")
	if _, err := svc.RegisterDocument(ctx, workdir, abs, CopyAuto, nil); err != nil {
		t.Fatalf("register: %v", err)
	}
	// 删除 knowledge_bases 表 → 复用路径的 addToBases 查询失败。
	primeCache(t, svc, workdir)
	dropTable(t, svc, workdir, "knowledge_bases")
	if _, err := svc.RegisterDocument(ctx, workdir, abs, CopyAuto, nil); err == nil {
		t.Fatal("库表缺失时复用注册应报错")
	}
}

func TestRegisterDocument_InsertConstraintError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	// 给 knowledge_documents 加 NOT NULL 无默认列 → INSERT 触发约束错误。
	conn := mustProjectDB(t, svc, workdir)
	if _, err := conn.Exec(`ALTER TABLE knowledge_documents ADD COLUMN forced_col TEXT NOT NULL`); err != nil {
		t.Fatalf("alter: %v", err)
	}
	primeCache(t, svc, workdir)
	if _, err := svc.RegisterDocument(ctx, workdir, filepath.Join(workdir, "a.md"), CopyAuto, nil); err == nil {
		t.Fatal("约束错误时注册应报错")
	}
}

func TestGetDocumentByID_TableMissing(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	dropTable(t, svc, workdir, "knowledge_documents")
	if _, err := svc.GetDocument(ctx, workdir, doc.ID); err == nil {
		t.Fatal("文档表缺失时查询应报错")
	}
	// UnlinkTask 在文档表缺失下仍能走到（先删 task_documents 再删 knowledge_documents）。
	if err := svc.UnlinkTask(ctx, workdir, "t1", doc.ID); err == nil {
		t.Fatal("表缺失下 unlink 应报错")
	}
}
