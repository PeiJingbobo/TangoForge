package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMergeFiles_MultiFile：多文件合并 + source_file=公共父目录。
func TestMergeFiles_MultiFile(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "docs")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	f1 := filepath.Join(sub, "a.md")
	f2 := filepath.Join(sub, "b.md")
	if err := os.WriteFile(f1, []byte("# 任务A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("# 任务B\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	content, source, err := mergeFiles(base, []string{"docs/a.md", "docs/b.md"})
	if err != nil {
		t.Fatalf("mergeFiles: %v", err)
	}
	// 文件注释头 + 内容合并。
	if !strings.Contains(content, "<!-- file: "+f1+" -->") || !strings.Contains(content, "# 任务A") || !strings.Contains(content, "# 任务B") {
		t.Fatalf("合并内容异常:\n%s", content)
	}
	// 多文件 source_file = 公共父目录。
	if source != sub {
		t.Fatalf("source_file=%s 期望公共父目录 %s", source, sub)
	}
}

// TestMergeFiles_SingleFile：单文件 source_file=该文件。
func TestMergeFiles_SingleFile(t *testing.T) {
	base := t.TempDir()
	f := filepath.Join(base, "a.md")
	if err := os.WriteFile(f, []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, source, err := mergeFiles(base, []string{"a.md"})
	if err != nil {
		t.Fatal(err)
	}
	if source != f {
		t.Fatalf("source_file=%s", source)
	}
}

// TestScanMarkdownFiles_Recursive：目录递归扫描（排序、忽略非 md）。
func TestScanMarkdownFiles_Recursive(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{
		filepath.Join(base, "b.md"),
		filepath.Join(base, "a.md"),
		filepath.Join(base, "c.markdown"),
		filepath.Join(base, "skip.txt"),
		filepath.Join(sub, "d.md"),
	}
	for _, f := range files {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := scanMarkdownFiles(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("扫描到 %d 个文件（应 4）: %v", len(got), got)
	}
	// 字典序（含子目录按路径排序）。
	want := []string{filepath.Join(base, "a.md"), filepath.Join(base, "b.md"),
		filepath.Join(base, "c.markdown"), filepath.Join(sub, "d.md")}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("顺序异常: got=%v want=%v", got, want)
		}
	}
}

// TestScanMarkdownFiles_NotDir：非目录报错。
func TestScanMarkdownFiles_NotDir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "x.md")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := scanMarkdownFiles(f); err == nil {
		t.Fatal("非目录应报错")
	}
}

// TestCommonParentDir：公共父目录计算。
func TestCommonParentDir(t *testing.T) {
	base := t.TempDir()
	cases := []struct {
		paths []string
		want  string
	}{
		{[]string{filepath.Join(base, "a", "x.md"), filepath.Join(base, "a", "b", "y.md")}, filepath.Join(base, "a")},
		{[]string{filepath.Join(base, "x.md"), filepath.Join(base, "y.md")}, base},
		{[]string{filepath.Join(base, "a.md")}, base}, // 单文件 = 父目录
	}
	for _, c := range cases {
		if got := commonParentDir(c.paths); got != c.want {
			t.Fatalf("commonParentDir(%v)=%s want=%s", c.paths, got, c.want)
		}
	}
}

// TestParse_DirectoryMode：目录导入全流程（mock LLM → 草稿，source_file=目录）。
func TestParse_DirectoryMode(t *testing.T) {
	srv := mockLLM(t, `{"tasks":[{"title":"目录任务","status":"todo"}]}`)
	svc, _ := newParser(t, srv.URL)
	workdir := initParserProject(t)

	docs := filepath.Join(workdir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "b.md"), []byte("# B\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	draft, err := svc.Parse(context.Background(), workdir, ParseInput{Directory: "docs"})
	if err != nil {
		t.Fatalf("Parse(directory): %v", err)
	}
	if draft.SourceFile != docs {
		t.Fatalf("source_file=%s 期望目录 %s", draft.SourceFile, docs)
	}
	if draft.Status != "pending" || draft.TaskCount != 1 {
		t.Fatalf("draft: %+v", draft)
	}
}
