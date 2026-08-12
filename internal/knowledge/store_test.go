package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tangoforge/internal/db"
)

func TestIsUniqueViolation(t *testing.T) {
	if !isUniqueViolation(errors.New("UNIQUE constraint failed: x")) {
		t.Error("UNIQUE constraint failed 应识别")
	}
	if !isUniqueViolation(errors.New("constraint failed")) {
		t.Error("constraint failed 应识别")
	}
	if isUniqueViolation(errors.New("other")) {
		t.Error("其它错误不应识别")
	}
	if isUniqueViolation(nil) {
		t.Error("nil 不应识别")
	}
}

func TestResolveDefaultDocDir_CustomConfig(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	// 项目配置自定义 default_doc_dir（相对路径 → 相对 workdir）。
	if err := os.MkdirAll(filepath.Join(workdir, ".taskboard"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := `knowledge:
  default_doc_dir: "kb_files"
`
	cfgPath := filepath.Join(workdir, ".taskboard", "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// 外部文件 auto → 拷贝进自定义目录。
	outDir := t.TempDir()
	ext := filepath.Join(outDir, "ext.md")
	_ = os.WriteFile(ext, []byte("x"), 0o644)
	doc, err := svc.RegisterDocument(ctx, workdir, ext, CopyAuto, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !strings.HasPrefix(doc.AbsPath, filepath.Join(workdir, "kb_files")) {
		t.Fatalf("应拷贝到自定义目录: %s", doc.AbsPath)
	}
}

func TestCopyFile_SrcMissing(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "nope"), filepath.Join(dir, "dest"))
	if err == nil {
		t.Fatal("源文件不存在应报错")
	}
}

func TestCopyFile_DstOpenFail(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	// dst 的父目录不存在 → OpenFile 失败。
	err := copyFile(src, filepath.Join(dir, "no-such-dir", "dest"))
	if err == nil {
		t.Fatal("目标父目录不存在应报错")
	}
	// dst 是目录 → OpenFile 失败。
	dstDir := filepath.Join(dir, "dstdir")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := copyFile(src, dstDir); err == nil {
		t.Fatal("目标为目录应报错")
	}
}

func TestProjectDB_FingerprintReplaced(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	if _, err := svc.ListBases(ctx, workdir); err != nil {
		t.Fatalf("first list: %v", err)
	}
	// 删除重建 meta.db（模拟 macOS 移入回收站后重建）。
	metaPath := db.MetaDBPath(workdir)
	_ = os.Remove(metaPath)
	_ = os.Remove(metaPath + "-wal")
	_ = os.Remove(metaPath + "-shm")
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	conn, err := db.EnsureProject(ctx, metaPath)
	if err != nil {
		t.Fatalf("recreate: %v", err)
	}
	_ = conn.Close()
	// 重新初始化默认库（新库无默认库）。
	if _, err := svc.EnsureDefaultBase(ctx, workdir); err != nil {
		t.Fatalf("ensure default after recreate: %v", err)
	}
	// 服务应自动重开连接（不报错）。
	bases, err := svc.ListBases(ctx, workdir)
	if err != nil {
		t.Fatalf("list after recreate: %v", err)
	}
	if len(bases) != 1 {
		t.Fatalf("新库应有 1 默认库，got %+v", bases)
	}
}

func TestUpdateBase_DescriptionOnly(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	kb, err := svc.CreateBase(ctx, workdir, "spec", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	desc := "新描述"
	got, err := svc.UpdateBase(ctx, workdir, kb.ID, nil, &desc)
	if err != nil {
		t.Fatalf("update desc: %v", err)
	}
	if got.Description != desc || got.Name != "spec" {
		t.Fatalf("描述更新错误: %+v", got)
	}
}

func TestRegisterDocument_CopyFail(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()

	// 外部文件 → 强制拷贝到只读目标目录（auto 下 resolve 失败）。
	// 通过把 .taskboard/knowledge 创建为文件模拟拷贝失败。
	kbDir := filepath.Join(workdir, ".taskboard", "knowledge")
	if err := os.WriteFile(kbDir, []byte("block"), 0o644); err != nil {
		t.Fatalf("block dir: %v", err)
	}
	outDir := t.TempDir()
	ext := filepath.Join(outDir, "ext.md")
	_ = os.WriteFile(ext, []byte("x"), 0o644)
	_, err := svc.RegisterDocument(ctx, workdir, ext, CopyAuto, nil)
	if err == nil {
		t.Fatal("拷贝到被文件占用的目录应失败")
	}
}

func TestRegisterDocument_CopyToNonExistentKB(t *testing.T) {
	svc := newTestService(t)
	workdir := initProject(t)
	ctx := context.Background()
	// 指定不存在库（含默认库兜底不生效路径）。
	doc, err := svc.RegisterDocument(ctx, workdir, writeFile(t, workdir, "a.md", "# a"), CopyAuto, []int64{})
	if err != nil {
		t.Fatalf("register empty kb: %v", err)
	}
	if doc.ID == "" {
		t.Fatal("空 kb_ids 应加入默认库")
	}
}
