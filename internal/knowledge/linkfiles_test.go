package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLinkFiles_Basic(t *testing.T) {
	svc := newTestServiceWithTasks(t, fakeTaskLister{})
	workdir := initProject(t)
	ctx := context.Background()

	// 指定库。
	kb, err := svc.CreateBase(ctx, workdir, "spec", "")
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	// 候选文件。
	_ = writeFile(t, workdir, "kb/a.md", "# a")
	_ = writeFile(t, workdir, "kb/b.md", "# b")

	res, err := svc.LinkFiles(ctx, workdir, []string{"task-1", "task-2"}, []KnowledgeFile{
		{Path: "kb/a.md", KB: "spec", Reason: "接口"},
		{Path: "kb/b.md", Reason: "无库名"},
	}, CopyAuto)
	if err != nil {
		t.Fatalf("link files: %v", err)
	}
	if res.Linked != 2 || res.Dropped != 0 {
		t.Fatalf("result = %+v", res)
	}
	// 两个任务各关联 2 文档。
	for _, tid := range []string{"task-1", "task-2"} {
		docs, err := svc.TaskDocuments(ctx, workdir, tid)
		if err != nil {
			t.Fatalf("docs %s: %v", tid, err)
		}
		if len(docs) != 2 {
			t.Fatalf("任务 %s 应关联 2 文档，got %d", tid, len(docs))
		}
	}
	// 库归属：a 在 spec 库，b 在默认库。
	docA, err := svc.GetDocument(ctx, workdir, mustFindDoc(t, svc, workdir, "kb/a.md"))
	if err != nil {
		t.Fatalf("get a: %v", err)
	}
	if len(docA.KBs) != 1 || docA.KBs[0] != kb.ID {
		t.Fatalf("a 应属 spec 库: %+v", docA.KBs)
	}
}

func TestLinkFiles_Empty(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	res, err := svc.LinkFiles(context.Background(), workdir, []string{"t"}, nil, CopyAuto)
	if err != nil || res.Linked != 0 || res.Dropped != 0 {
		t.Fatalf("空输入应 no-op: %+v err=%v", res, err)
	}
}

func TestLinkFiles_DroppedMissing(t *testing.T) {
	svc := newTestServiceWithTasks(t, fakeTaskLister{})
	workdir := initProject(t)
	ctx := context.Background()
	// 正常 + 缺失文件。
	_ = writeFile(t, workdir, "a.md", "# a")
	res, err := svc.LinkFiles(ctx, workdir, []string{"t1"}, []KnowledgeFile{
		{Path: "a.md"},
		{Path: "missing/file.md"},
		{Path: "  "}, // 空 path → dropped
	}, CopyAuto)
	if err != nil {
		t.Fatalf("link files: %v", err)
	}
	if res.Linked != 1 || res.Dropped != 2 {
		t.Fatalf("result = %+v", res)
	}
}

func TestLinkFiles_InvalidKB(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	_ = writeFile(t, workdir, "a.md", "# a")
	_, err := svc.LinkFiles(ctx, workdir, []string{"t1"}, []KnowledgeFile{
		{Path: "a.md", KB: "不存在"},
	}, CopyAuto)
	if !errors.Is(err, ErrKnowledgeInvalid) {
		t.Fatalf("不存在库应 KNOWLEDGE_INVALID，got %v", err)
	}
}

func TestLinkFiles_DBError(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	_ = writeFile(t, workdir, "a.md", "# a")
	primeCache(t, svc, workdir)
	closeCachedDB(t, svc, workdir)
	if _, err := svc.LinkFiles(ctx, workdir, []string{"t1"}, []KnowledgeFile{{Path: "a.md"}}, CopyAuto); err == nil {
		t.Fatal("连接关闭应报错")
	}
}

func TestLinkFiles_ExternalCopy(t *testing.T) {
	svc := newTestServiceWithTasks(t, fakeTaskLister{})
	workdir := initProject(t)
	ctx := context.Background()
	outDir := t.TempDir()
	ext := filepath.Join(outDir, "external.md")
	_ = os.WriteFile(ext, []byte("x"), 0o644)
	res, err := svc.LinkFiles(ctx, workdir, []string{"t1"}, []KnowledgeFile{{Path: ext}}, CopyAuto)
	if err != nil {
		t.Fatalf("link external: %v", err)
	}
	if res.Linked != 1 {
		t.Fatalf("result = %+v", res)
	}
	docs, _ := svc.TaskDocuments(ctx, workdir, "t1")
	if len(docs) != 1 || docs[0].OriginPath != ext {
		t.Fatalf("外部文件应拷贝: %+v", docs)
	}
}

// mustFindDoc 按文件名查文档 id。
func mustFindDoc(t *testing.T, svc Service, workdir, rel string) string {
	t.Helper()
	docs, err := svc.ListDocuments(context.Background(), workdir, DocumentFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, d := range docs.Items {
		if d.RelPath == rel || d.AbsPath == filepath.Join(workdir, rel) {
			return d.ID
		}
	}
	t.Fatalf("未找到文档 %s", rel)
	return ""
}
