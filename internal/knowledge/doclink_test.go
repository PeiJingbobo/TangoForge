package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBases_CRUD(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	// 默认库存在（TF-044 项目初始化已建）。
	bases, err := svc.ListBases(ctx, workdir)
	if err != nil {
		t.Fatalf("list bases: %v", err)
	}
	if len(bases) != 1 || !bases[0].IsDefault {
		t.Fatalf("应恰好 1 个默认库，got %+v", bases)
	}

	// 创建库。
	kb, err := svc.CreateBase(ctx, workdir, "spec", "规格文档")
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	if kb.Name != "spec" || kb.IsDefault {
		t.Fatalf("created kb wrong: %+v", kb)
	}

	// 重名拒绝 → KNOWLEDGE_INVALID。
	if _, err := svc.CreateBase(ctx, workdir, "spec", ""); !errors.Is(err, ErrKnowledgeInvalid) {
		t.Fatalf("重名库应 KNOWLEDGE_INVALID，got %v", err)
	}
	// 空名拒绝。
	if _, err := svc.CreateBase(ctx, workdir, "  ", ""); !errors.Is(err, ErrKnowledgeInvalid) {
		t.Fatalf("空名库应 KNOWLEDGE_INVALID，got %v", err)
	}

	// 重命名。
	newName := "spec-v2"
	if _, err := svc.UpdateBase(ctx, workdir, kb.ID, &newName, nil); err != nil {
		t.Fatalf("update base: %v", err)
	}
	got, err := svc.GetBase(ctx, workdir, kb.ID)
	if err != nil {
		t.Fatalf("get base: %v", err)
	}
	if got.Name != newName {
		t.Fatalf("rename failed: %+v", got)
	}
	// 重命名为已有名 → 拒绝。
	dupName := DefaultKBName
	if _, err := svc.UpdateBase(ctx, workdir, kb.ID, &dupName, nil); !errors.Is(err, ErrKnowledgeInvalid) {
		t.Fatalf("重名为默认库名应拒绝，got %v", err)
	}
	// 空名拒绝。
	empty := " "
	if _, err := svc.UpdateBase(ctx, workdir, kb.ID, &empty, nil); !errors.Is(err, ErrKnowledgeInvalid) {
		t.Fatalf("空名更新应拒绝，got %v", err)
	}

	// 删除库（仅移除边）。
	if err := svc.DeleteBase(ctx, workdir, kb.ID); err != nil {
		t.Fatalf("delete base: %v", err)
	}
	if _, err := svc.GetBase(ctx, workdir, kb.ID); !errors.Is(err, ErrKnowledgeNotFound) {
		t.Fatalf("删除后应 KNOWLEDGE_NOT_FOUND，got %v", err)
	}
	// 删除不存在库 → NOT_FOUND。
	if err := svc.DeleteBase(ctx, workdir, 9999); !errors.Is(err, ErrKnowledgeNotFound) {
		t.Fatalf("删除不存在库应 NOT_FOUND，got %v", err)
	}
}

// TestEnsureDefaultBase_LazyCreate 验证默认库懒创建（存量项目补建，幂等）。
func TestEnsureDefaultBase_LazyCreate(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	// 删除默认库，模拟存量项目无默认库。
	conn := mustProjectDB(t, svc, workdir)
	if _, err := conn.ExecContext(ctx, `DELETE FROM knowledge_bases`); err != nil {
		t.Fatalf("clear bases: %v", err)
	}

	kb, err := svc.EnsureDefaultBase(ctx, workdir)
	if err != nil {
		t.Fatalf("ensure default: %v", err)
	}
	if !kb.IsDefault {
		t.Fatalf("ensure 返回非默认库: %+v", kb)
	}
	// 幂等。
	kb2, err := svc.EnsureDefaultBase(ctx, workdir)
	if err != nil {
		t.Fatalf("ensure default 2: %v", err)
	}
	if kb2.ID != kb.ID {
		t.Fatalf("幂等失败: %d != %d", kb2.ID, kb.ID)
	}
}

func TestRegisterDocument_InsideProject(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	abs := writeFile(t, workdir, "docs/spec.md", "# 规格\n内容")

	doc, err := svc.RegisterDocument(ctx, workdir, "docs/spec.md", CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if doc.AbsPath != abs {
		t.Fatalf("abs_path = %q, want %q", doc.AbsPath, abs)
	}
	if doc.RelPath != "docs/spec.md" {
		t.Fatalf("rel_path = %q", doc.RelPath)
	}
	if doc.Type != DocTypeText {
		t.Fatalf("type = %q, want text", doc.Type)
	}
	if doc.Path != "docs/spec.md" {
		t.Fatalf("path = %q（项目内应存相对路径）", doc.Path)
	}

	// 已注册 → 复用（同 id）。
	doc2, err := svc.RegisterDocument(ctx, workdir, "docs/spec.md", CopyNone, nil)
	if err != nil {
		t.Fatalf("register again: %v", err)
	}
	if doc2.ID != doc.ID {
		t.Fatalf("重复注册应复用同记录: %s != %s", doc2.ID, doc.ID)
	}

	// 绝对路径注册同一文件 → 同样复用。
	doc3, err := svc.RegisterDocument(ctx, workdir, abs, CopyAuto, nil)
	if err != nil {
		t.Fatalf("register abs: %v", err)
	}
	if doc3.ID != doc.ID {
		t.Fatalf("绝对路径应复用: %s != %s", doc3.ID, doc.ID)
	}

	// 默认库成员。
	bases, err := svc.ListBases(ctx, workdir)
	if err != nil {
		t.Fatalf("list bases: %v", err)
	}
	if len(bases) != 1 || bases[0].DocCount != 1 {
		t.Fatalf("默认库应有 1 文档，got %+v", bases)
	}
}

func TestRegisterDocument_ExternalCopy(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	// 外部文件（workdir 外）。
	outDir := t.TempDir()
	extAbs := filepath.Join(outDir, "external.md")
	if err := os.WriteFile(extAbs, []byte("外部文档内容"), 0o644); err != nil {
		t.Fatalf("write external: %v", err)
	}

	// auto：外部文件 → 拷贝进 .taskboard/knowledge/，记录 origin_path。
	doc, err := svc.RegisterDocument(ctx, workdir, extAbs, CopyAuto, nil)
	if err != nil {
		t.Fatalf("register auto: %v", err)
	}
	if doc.OriginPath != extAbs {
		t.Fatalf("origin_path = %q, want %q", doc.OriginPath, extAbs)
	}
	kbDir := filepath.Join(workdir, ".taskboard", "knowledge")
	if !strings.HasPrefix(doc.AbsPath, kbDir) {
		t.Fatalf("拷贝落点应位于 .taskboard/knowledge，got %s", doc.AbsPath)
	}
	if _, err := os.Stat(doc.AbsPath); err != nil {
		t.Fatalf("拷贝文件应存在: %v", err)
	}

	// copy：另一个外部文件 → 同样拷贝。
	ext2 := filepath.Join(outDir, "ext2.md")
	_ = os.WriteFile(ext2, []byte("x"), 0o644)
	doc2, err := svc.RegisterDocument(ctx, workdir, ext2, CopyCopy, nil)
	if err != nil {
		t.Fatalf("register copy: %v", err)
	}
	if doc2.OriginPath != ext2 {
		t.Fatalf("copy 模式应记录 origin: %s", doc2.OriginPath)
	}

	// none：不拷贝，原样引用。
	ext3 := filepath.Join(outDir, "ext3.md")
	_ = os.WriteFile(ext3, []byte("y"), 0o644)
	doc3, err := svc.RegisterDocument(ctx, workdir, ext3, CopyNone, nil)
	if err != nil {
		t.Fatalf("register none: %v", err)
	}
	if doc3.OriginPath != "" {
		t.Fatalf("none 模式不应有 origin_path: %q", doc3.OriginPath)
	}
	if doc3.AbsPath != ext3 {
		t.Fatalf("none 模式应原样引用: %s", doc3.AbsPath)
	}

	// 不存在路径 → DOCUMENT_MISSING。
	if _, err := svc.RegisterDocument(ctx, workdir, filepath.Join(outDir, "nope.md"), CopyAuto, nil); !errors.Is(err, ErrDocumentMissing) {
		t.Fatalf("不存在路径应 DOCUMENT_MISSING，got %v", err)
	}
}

func TestRegisterDocument_BinaryOnly(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	abs := writeFile(t, workdir, "assets/logo.png", "\x89PNG\r\n\x1a\nbinary")
	doc, err := svc.RegisterDocument(ctx, workdir, abs, CopyAuto, nil)
	if err != nil {
		t.Fatalf("register binary: %v", err)
	}
	if doc.Type != DocTypeBinary {
		t.Fatalf("二进制应注册为 binary，got %q", doc.Type)
	}
}

func TestRegisterDocument_KBMembership(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	kb, err := svc.CreateBase(ctx, workdir, "spec", "")
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	abs := writeFile(t, workdir, "a.md", "# a")

	// 注册时指定库。
	doc, err := svc.RegisterDocument(ctx, workdir, abs, CopyAuto, []int64{kb.ID})
	if err != nil {
		t.Fatalf("register with kb: %v", err)
	}
	detail, err := svc.GetDocument(ctx, workdir, doc.ID)
	if err != nil {
		t.Fatalf("get document: %v", err)
	}
	if len(detail.KBs) != 1 || detail.KBs[0] != kb.ID {
		t.Fatalf("文档应只属于 spec 库，got %v", detail.KBs)
	}

	// 指定不存在的库 → KNOWLEDGE_NOT_FOUND。
	abs2 := writeFile(t, workdir, "b.md", "# b")
	if _, err := svc.RegisterDocument(ctx, workdir, abs2, CopyAuto, []int64{9999}); !errors.Is(err, ErrKnowledgeNotFound) {
		t.Fatalf("不存在库应 NOT_FOUND，got %v", err)
	}
}

func TestDeleteDocument_Cascades(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	kb, _ := svc.CreateBase(ctx, workdir, "spec", "")
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, []int64{kb.ID})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := svc.LinkTask(ctx, workdir, "task-1", doc.ID, "", CopyAuto, nil); err != nil {
		t.Fatalf("link task: %v", err)
	}

	// 删除文档 → chunks/库边/任务边全部移除。
	if err := svc.DeleteDocument(ctx, workdir, doc.ID); err != nil {
		t.Fatalf("delete document: %v", err)
	}
	conn := mustProjectDB(t, svc, workdir)
	for table, col := range map[string]string{
		"knowledge_base_documents": "document_id",
		"task_documents":           "document_id",
		"knowledge_documents":      "id",
	} {
		var n int
		if err := conn.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM `+table+` WHERE `+col+` = ?`, doc.ID).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Fatalf("删除后 %s 应无残留，got %d", table, n)
		}
	}
	// 删除不存在文档 → NOT_FOUND。
	if err := svc.DeleteDocument(ctx, workdir, "nope"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("删除不存在文档应 NOT_FOUND，got %v", err)
	}
}

func TestRelinkDocument(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	// 注册旧文件并关联任务。
	oldAbs := writeFile(t, workdir, "old.md", "# old content")
	doc, err := svc.RegisterDocument(ctx, workdir, oldAbs, CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := svc.LinkTask(ctx, workdir, "task-1", doc.ID, "", CopyAuto, nil); err != nil {
		t.Fatalf("link task: %v", err)
	}

	// relink 到新路径。
	newAbs := writeFile(t, workdir, "new.md", "# new content")
	relinked, err := svc.RelinkDocument(ctx, workdir, doc.ID, newAbs, CopyNone)
	if err != nil {
		t.Fatalf("relink: %v", err)
	}
	if relinked.AbsPath != newAbs {
		t.Fatalf("relink 后 abs_path = %q, want %q", relinked.AbsPath, newAbs)
	}
	if relinked.Status != DocStatusIndexing {
		t.Fatalf("relink 应重置为 indexing，got %q", relinked.Status)
	}
	if len(relinked.History) != 1 || relinked.History[0].Path != oldAbs {
		t.Fatalf("history 应含旧路径: %+v", relinked.History)
	}
	if relinked.Embedded != EmbedNo {
		t.Fatalf("relink 应重置嵌入: %d", relinked.Embedded)
	}

	// 任务关联保留。
	docs, err := svc.TaskDocuments(ctx, workdir, "task-1")
	if err != nil {
		t.Fatalf("task docs: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != doc.ID {
		t.Fatalf("relink 应保留任务关联: %+v", docs)
	}

	// 新路径不存在 → DOCUMENT_MISSING。
	if _, err := svc.RelinkDocument(ctx, workdir, doc.ID, filepath.Join(workdir, "missing.md"), CopyNone); !errors.Is(err, ErrDocumentMissing) {
		t.Fatalf("缺失新路径应 MISSING，got %v", err)
	}
	// 二进制新路径 → INVALID。
	binAbs := writeFile(t, workdir, "x.png", "\x89PNG")
	if _, err := svc.RelinkDocument(ctx, workdir, doc.ID, binAbs, CopyNone); !errors.Is(err, ErrDocumentInvalid) {
		t.Fatalf("二进制 relink 应 INVALID，got %v", err)
	}
	// 文档不存在 → NOT_FOUND。
	if _, err := svc.RelinkDocument(ctx, workdir, "nope", newAbs, CopyNone); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("relink 不存在文档应 NOT_FOUND，got %v", err)
	}
}

func TestLinkUnlinkTask(t *testing.T) {
	svc := newTestServiceWithTasks(t, fakeTaskLister{})
	workdir := initProject(t)
	ctx := context.Background()

	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// link by document_id。
	if err := svc.LinkTask(ctx, workdir, "task-1", doc.ID, "", CopyAuto, nil); err != nil {
		t.Fatalf("link: %v", err)
	}
	// 幂等重复 link。
	if err := svc.LinkTask(ctx, workdir, "task-1", doc.ID, "", CopyAuto, nil); err != nil {
		t.Fatalf("link again: %v", err)
	}
	// link by path（未注册自动入库）。
	if err := svc.LinkTask(ctx, workdir, "task-2", "", filepath.Join(workdir, "b.md"), CopyAuto, nil); err == nil {
		t.Fatal("link by 不存在 path 应报错")
	}
	bAbs := writeFile(t, workdir, "b.md", "# b")
	if err := svc.LinkTask(ctx, workdir, "task-2", "", bAbs, CopyAuto, nil); err != nil {
		t.Fatalf("link by path: %v", err)
	}

	// 任务关联列表。
	docs1, err := svc.TaskDocuments(ctx, workdir, "task-1")
	if err != nil {
		t.Fatalf("task1 docs: %v", err)
	}
	if len(docs1) != 1 || docs1[0].ID != doc.ID {
		t.Fatalf("task-1 应关联 1 文档: %+v", docs1)
	}

	// 缺 document_id 与 path → 参数错误。
	if err := svc.LinkTask(ctx, workdir, "task-1", "", "", CopyAuto, nil); !errors.Is(err, ErrDocumentInvalid) {
		t.Fatalf("缺参应 INVALID，got %v", err)
	}
	// 缺 task_id → 参数错误。
	if err := svc.LinkTask(ctx, workdir, "", doc.ID, "", CopyAuto, nil); !errors.Is(err, ErrDocumentInvalid) {
		t.Fatalf("缺 task_id 应 INVALID，got %v", err)
	}

	// unlink。
	if err := svc.UnlinkTask(ctx, workdir, "task-1", doc.ID); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if docs2, _ := svc.TaskDocuments(ctx, workdir, "task-1"); len(docs2) != 0 {
		t.Fatalf("unlink 后应无关联: %+v", docs2)
	}
	// 无关联再 unlink → INVALID。
	if err := svc.UnlinkTask(ctx, workdir, "task-1", doc.ID); !errors.Is(err, ErrDocumentInvalid) {
		t.Fatalf("重复 unlink 应 INVALID，got %v", err)
	}
}

func TestListDocuments_Filters(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	kb, _ := svc.CreateBase(ctx, workdir, "spec", "")
	_ = writeFile(t, workdir, "a.md", "# a")
	_ = writeFile(t, workdir, "bb.md", "# bb")
	docA, err := svc.RegisterDocument(ctx, workdir, filepath.Join(workdir, "a.md"), CopyAuto, []int64{kb.ID})
	if err != nil {
		t.Fatalf("register a: %v", err)
	}
	docB, err := svc.RegisterDocument(ctx, workdir, filepath.Join(workdir, "bb.md"), CopyAuto, nil)
	if err != nil {
		t.Fatalf("register b: %v", err)
	}

	// 全部。
	res, err := svc.ListDocuments(ctx, workdir, DocumentFilter{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("total = %d, want 2", res.Total)
	}
	// kb 过滤。
	res, _ = svc.ListDocuments(ctx, workdir, DocumentFilter{KBID: kb.ID})
	if res.Total != 1 || res.Items[0].ID != docA.ID {
		t.Fatalf("kb filter wrong: %+v", res)
	}
	// q 搜索。
	res, _ = svc.ListDocuments(ctx, workdir, DocumentFilter{Q: "bb"})
	if res.Total != 1 || res.Items[0].ID != docB.ID {
		t.Fatalf("q filter wrong: %+v", res)
	}
	// 分页。
	res, _ = svc.ListDocuments(ctx, workdir, DocumentFilter{Page: 0, Size: 1})
	if len(res.Items) != 1 || res.Total != 2 {
		t.Fatalf("page wrong: %+v", res)
	}
	// status 过滤（无 missing 文档时为空）。
	res, _ = svc.ListDocuments(ctx, workdir, DocumentFilter{Status: DocStatusMissing})
	if res.Total != 0 {
		t.Fatalf("missing filter wrong: %+v", res)
	}
}
