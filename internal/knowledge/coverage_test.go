package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIsTextFile(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		ext  string
		data []byte
		want bool
	}{
		{"markdown", ".md", []byte("# hi"), true},
		{"markdown_ext", ".markdown", []byte("# hi"), true},
		{"txt", ".txt", []byte("hi"), true},
		{"go", ".go", []byte("package main"), true},
		{"yaml", ".yaml", []byte("a: 1"), true},
		{"png", ".png", []byte("\x89PNG"), false},
		{"pdf", ".pdf", []byte("%PDF-1.4"), false},
		{"docx", ".docx", []byte("PK\x03\x04"), false},
		{"zip", ".zip", []byte("PK\x03\x04"), false},
		{"exe", ".exe", []byte("MZ"), false},
		{"woff2", ".woff2", []byte("wOF2"), false},
		{"unknown_text_without_nul", ".custom", []byte("plain text without nul bytes here"), true},
		{"unknown_binary_with_nul", ".binx", []byte("abc\x00def"), false},
		{"missing_file", ".nope", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, "f"+c.ext)
			if c.data != nil {
				if err := os.WriteFile(path, c.data, 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
			} else {
				// 确保文件不存在（missing 场景）。
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					t.Fatalf("remove: %v", err)
				}
			}
			if got := isTextFile(path, int64(len(c.data))); got != c.want {
				t.Errorf("isTextFile(%s) = %v, want %v", path, got, c.want)
			}
		})
	}
}

func TestRelink_ExternalCopy(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	// 项目内旧文件。
	oldAbs := writeFile(t, workdir, "old.md", "# old")
	doc, err := svc.RegisterDocument(ctx, workdir, oldAbs, CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// relink 到外部文件（auto → 拷贝）。
	outDir := t.TempDir()
	ext := filepath.Join(outDir, "external.md")
	if err := os.WriteFile(ext, []byte("external content"), 0o644); err != nil {
		t.Fatalf("write ext: %v", err)
	}
	relinked, err := svc.RelinkDocument(ctx, workdir, doc.ID, ext, CopyAuto)
	if err != nil {
		t.Fatalf("relink external: %v", err)
	}
	if relinked.OriginPath != ext {
		t.Fatalf("relink 外部文件应记 origin_path: %q", relinked.OriginPath)
	}
	if !filepath.HasPrefix(relinked.AbsPath, filepath.Join(workdir, ".taskboard", "knowledge")) {
		t.Fatalf("relink 拷贝应落点默认目录: %s", relinked.AbsPath)
	}
	// history 含旧路径。
	if len(relinked.History) != 1 || relinked.History[0].Path != oldAbs {
		t.Fatalf("history 错误: %+v", relinked.History)
	}
}

func TestRelink_DirectoryRejected(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := svc.RelinkDocument(ctx, workdir, doc.ID, workdir, CopyNone); !errors.Is(err, ErrDocumentInvalid) {
		t.Fatalf("relink 到目录应 INVALID，got %v", err)
	}
}

func TestRelink_PathTakenByOther(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc1, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register a: %v", err)
	}
	doc2, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "b.md", "# b"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register b: %v", err)
	}
	// relink a 到 b 已占用的路径 → INVALID。
	if _, err := svc.RelinkDocument(ctx, workdir, doc1.ID, doc2.AbsPath, CopyNone); !errors.Is(err, ErrDocumentInvalid) {
		t.Fatalf("relink 到已占用路径应 INVALID，got %v", err)
	}
}

func TestDeleteBase_KeepsDocAndTaskLink(t *testing.T) {
	svc := newTestServiceWithTasks(t, fakeTaskLister{})
	workdir := initProject(t)
	ctx := context.Background()

	kb, err := svc.CreateBase(ctx, workdir, "spec", "")
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, []int64{kb.ID})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := svc.LinkTask(ctx, workdir, "task-1", doc.ID, "", CopyAuto, nil); err != nil {
		t.Fatalf("link: %v", err)
	}

	// 删除库 → 文档与任务关联保留（QA-K14）。
	if err := svc.DeleteBase(ctx, workdir, kb.ID); err != nil {
		t.Fatalf("delete base: %v", err)
	}
	if _, err := svc.GetDocument(ctx, workdir, doc.ID); err != nil {
		t.Fatalf("删除库后文档应保留: %v", err)
	}
	if docs, err := svc.TaskDocuments(ctx, workdir, "task-1"); err != nil || len(docs) != 1 {
		t.Fatalf("删除库后任务关联应保留: %+v err=%v", docs, err)
	}
	// 库内文档数归零。
	bases, _ := svc.ListBases(ctx, workdir)
	for _, b := range bases {
		if b.ID == kb.ID {
			t.Fatalf("库应已删除: %+v", bases)
		}
	}
}

func TestRegister_PathOutsideWorkdirNone(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	outDir := t.TempDir()
	ext := filepath.Join(outDir, "external.md")
	_ = os.WriteFile(ext, []byte("x"), 0o644)

	doc, err := svc.RegisterDocument(ctx, workdir, ext, CopyNone, nil)
	if err != nil {
		t.Fatalf("register none: %v", err)
	}
	if doc.RelPath != "" {
		t.Fatalf("外部文件 rel_path 应为空，got %q", doc.RelPath)
	}
	if doc.AbsPath != filepath.Clean(ext) {
		t.Fatalf("外部 none 引用应为原始路径: %s", doc.AbsPath)
	}
}

func TestOnWriteHook(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	var actions []string
	svc.SetOnWrite(func(_ context.Context, _ string, action, _ string) {
		actions = append(actions, action)
	})
	kb, err := svc.CreateBase(ctx, workdir, "spec", "")
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, []int64{kb.ID})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := svc.LinkTask(ctx, workdir, "t1", doc.ID, "", CopyAuto, nil); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := svc.UnlinkTask(ctx, workdir, "t1", doc.ID); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if err := svc.DeleteDocument(ctx, workdir, doc.ID); err != nil {
		t.Fatalf("delete doc: %v", err)
	}
	if err := svc.DeleteBase(ctx, workdir, kb.ID); err != nil {
		t.Fatalf("delete base: %v", err)
	}
	want := []string{"kb_created", "document_added", "task_linked", "task_unlinked", "document_removed", "kb_deleted"}
	if len(actions) != len(want) {
		t.Fatalf("onwrite actions = %v, want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Fatalf("action[%d] = %q, want %q", i, actions[i], want[i])
		}
	}
}

func TestCodeOf(t *testing.T) {
	if got := CodeOf(ErrDocumentNotFound); got != CodeDocumentNotFound {
		t.Fatalf("CodeOf = %q", got)
	}
	if got := CodeOf(errors.New("other")); got != "" {
		t.Fatalf("非知识库错误应返回空，got %q", got)
	}
}

func TestProjectDB_Errors(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// 未初始化目录 → 文档错误（DOCUMENT_INVALID 包装）。
	if _, err := svc.ListBases(ctx, filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("未初始化目录应报错")
	}
	// 相对路径 → 错误。
	if _, err := svc.ListBases(ctx, "relative"); err == nil {
		t.Fatal("相对路径应报错")
	}
}

func TestClose(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	if _, err := svc.ListBases(context.Background(), workdir); err != nil {
		t.Fatalf("list: %v", err)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// 关闭后可重新打开（懒重开）。
	if _, err := svc.ListBases(context.Background(), workdir); err != nil {
		t.Fatalf("list after close: %v", err)
	}
}

func TestUpdateContent(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# 原始"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 编辑 → 写盘 + 重置索引状态。
	if err := svc.UpdateContent(ctx, workdir, doc.ID, "# 更新后"); err != nil {
		t.Fatalf("update content: %v", err)
	}
	data, _ := os.ReadFile(doc.AbsPath)
	if string(data) != "# 更新后" {
		t.Fatalf("写盘未更新: %s", string(data))
	}
	got, _ := svc.GetDocument(ctx, workdir, doc.ID)
	if got.Status != DocStatusIndexing {
		t.Fatalf("编辑后应标记 indexing，got %q", got.Status)
	}
	// 文档不存在 → NOT_FOUND。
	if err := svc.UpdateContent(ctx, workdir, "nope", "x"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("不存在文档应 NOT_FOUND，got %v", err)
	}
}

func TestUpdateContent_BinaryRejected(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "x.png", "\x89PNG"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := svc.UpdateContent(ctx, workdir, doc.ID, "text"); !errors.Is(err, ErrDocumentInvalid) {
		t.Fatalf("二进制编辑应 INVALID，got %v", err)
	}
}

func TestUpdateContent_WriteFail(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 删除父目录 → 写盘失败（目录不存在）。
	_ = os.RemoveAll(filepath.Dir(doc.AbsPath))
	if err := svc.UpdateContent(ctx, workdir, doc.ID, "x"); err == nil {
		t.Fatal("目录删除后写盘应失败")
	}
}

func TestTaskListerAdapter(t *testing.T) {
	adapter := TaskListerAdapter(func(_ context.Context, _, _ string) (any, error) {
		return "task", nil
	})
	v, err := adapter.Get(context.Background(), "/w", "t1")
	if err != nil || v != "task" {
		t.Fatalf("adapter get: %v %v", v, err)
	}
}

func TestServiceInterfaceConformance(t *testing.T) {
	// 编译期断言：*service 实现全部接口方法。
	_ = newTestService(t)
}
