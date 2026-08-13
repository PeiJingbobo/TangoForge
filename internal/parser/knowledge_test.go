package parser

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"tangoforge/internal/config"
	"tangoforge/internal/db"
	"tangoforge/internal/knowledge"
	"tangoforge/internal/task"
	"testing"
)

// discardLogger 返回静默日志器。
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newParserTasks 构造任务服务（knowledge 测试确认入库用）。
func newParserTasks(t *testing.T) task.Service {
	t.Helper()
	ts := task.NewService(task.Options{})
	t.Cleanup(func() { _ = ts.Close() })
	return ts
}

// taskListFilter 列表过滤（非分页树形）。
type taskListFilter = task.ListFilter

// mustParserConn 打开项目 meta.db 连接。
func mustParserConn(t *testing.T, workdir string) *sql.DB {
	t.Helper()
	conn, err := db.Open(db.MetaDBPath(workdir))
	if err != nil {
		t.Fatalf("open meta.db: %v", err)
	}
	return conn
}

// newParserWithKnowledge 构造带 knowledge 服务的 parser。
func newParserWithKnowledge(t *testing.T, llmURL string) (*Service, knowledge.Service, string) {
	t.Helper()
	workdir := initParserProject(t)
	ksvc := knowledge.NewService(knowledge.Options{Logger: discardLogger()})
	t.Cleanup(func() { _ = ksvc.Close() })
	_, _ = ksvc.EnsureDefaultBase(context.Background(), workdir)

	taskSvc := newParserTasks(t)
	events := &[]string{}
	svc := NewService(Options{
		LLM: func() config.LLMConfig {
			return config.LLMConfig{BaseURL: llmURL, Model: "mock", APIKind: "openai", TimeoutSec: 5, Retries: 0}
		},
		Tasks:     taskSvc,
		Knowledge: ksvc,
		OnEvent: func(_ context.Context, _ string, action, _ string) {
			*events = append(*events, action)
		},
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc, ksvc, workdir
}

func TestParse_KnowledgeFilesParsed(t *testing.T) {
	llmOut := `{"tasks":[{"title":"接口改造","status":"todo"}],
	  "knowledge_files":[{"path":"docs/spec/api.md","kb":"spec","reason":"接口变更说明"}]}`
	srv := mockLLM(t, llmOut)
	svc, _, workdir := newParserWithKnowledge(t, srv.URL)

	// 候选知识库文件（供 knowledge_files 引用）。
	if err := os.MkdirAll(filepath.Join(workdir, "docs", "spec"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(workdir, "docs", "spec", "api.md"), []byte("# API"), 0o644)

	// knowledge 输入提供候选目录。
	draft, err := svc.Parse(context.Background(), workdir, ParseInput{
		Content:    "# 任务文档",
		SourceFile: "tasks.md",
		Knowledge: &KnowledgeInput{
			Directory: "docs/spec",
		},
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// 草稿已生成（knowledge_files 校验通过）。
	if draft.ID == "" {
		t.Fatal("draft id 应为空")
	}
	// 通过 parsed_json 检查 knowledge_files。
	conn := mustParserConn(t, workdir)
	defer func() { _ = conn.Close() }()
	var parsed string
	if err := conn.QueryRow(`SELECT parsed_json FROM import_drafts WHERE id = ?`, draft.ID).Scan(&parsed); err != nil {
		t.Fatalf("query: %v", err)
	}
	var pr ParseResult
	if err := json.Unmarshal([]byte(parsed), &pr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(pr.KnowledgeFiles) != 1 || pr.KnowledgeFiles[0].Path != "docs/spec/api.md" {
		t.Fatalf("knowledge_files = %+v", pr.KnowledgeFiles)
	}
}

func TestParse_KnowledgeFilesMissingPath(t *testing.T) {
	// knowledge_files 缺 path → 整次失败。
	llmOut := `{"tasks":[{"title":"t","status":"todo"}],"knowledge_files":[{"kb":"spec"}]}`
	srv := mockLLM(t, llmOut)
	svc, _, workdir := newParserWithKnowledge(t, srv.URL)
	_, err := svc.Parse(context.Background(), workdir, ParseInput{Content: "# x", SourceFile: "a.md"})
	if err == nil || !strings.Contains(err.Error(), "缺少 path") {
		t.Fatalf("缺 path 应整次失败，got %v", err)
	}
}

func TestConfirm_KnowledgeFilesLinked(t *testing.T) {
	llmOut := `{"tasks":[{"title":"接口改造","status":"todo"}],
	  "knowledge_files":[{"path":"kb/api.md","reason":"接口文档"}]}`
	srv := mockLLM(t, llmOut)
	svc, ksvc, workdir := newParserWithKnowledge(t, srv.URL)
	_ = os.MkdirAll(filepath.Join(workdir, "kb"), 0o755)
	_ = os.WriteFile(filepath.Join(workdir, "kb", "api.md"), []byte("# API"), 0o644)

	draft, err := svc.Parse(context.Background(), workdir, ParseInput{
		Content: "# 任务\n## 接口改造", SourceFile: "t.md",
		Knowledge: &KnowledgeInput{Directory: "kb"},
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res, err := svc.Confirm(context.Background(), workdir, draft.ID)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("created = %d", res.Created)
	}
	if res.DroppedKnowledge != 0 {
		t.Fatalf("dropped_knowledge = %d", res.DroppedKnowledge)
	}
	// 任务应关联到文档。
	tasks, err := newParserTasks(t).List(context.Background(), workdir, taskListFilter{})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	_ = tasks
	// 直接查任务库。
	conn := mustParserConn(t, workdir)
	defer func() { _ = conn.Close() }()
	var taskID string
	if err := conn.QueryRow(`SELECT id FROM tasks LIMIT 1`).Scan(&taskID); err != nil {
		t.Fatalf("query task: %v", err)
	}
	docs, err := ksvc.TaskDocuments(context.Background(), workdir, taskID)
	if err != nil {
		t.Fatalf("task docs: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("任务应关联 1 文档，got %+v", docs)
	}
	// 文档应注册。
	if len(docs[0].AbsPath) == 0 || docs[0].RelPath != "kb/api.md" {
		t.Fatalf("文档路径错误: %+v", docs[0])
	}
}

func TestConfirm_KnowledgeFilesDropped(t *testing.T) {
	// 路径缺失 → dropped 计数（不阻断）。
	llmOut := `{"tasks":[{"title":"t","status":"todo"}],
	  "knowledge_files":[{"path":"missing/file.md"}]}`
	srv := mockLLM(t, llmOut)
	svc, _, workdir := newParserWithKnowledge(t, srv.URL)
	draft, err := svc.Parse(context.Background(), workdir, ParseInput{Content: "# x", SourceFile: "a.md"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res, err := svc.Confirm(context.Background(), workdir, draft.ID)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("created = %d", res.Created)
	}
	if res.DroppedKnowledge != 1 {
		t.Fatalf("dropped_knowledge = %d, want 1", res.DroppedKnowledge)
	}
}

func TestConfirm_KnowledgeFilesInvalidKB(t *testing.T) {
	// 引用了不存在的库 → 整次失败（QA-K11）。
	llmOut := `{"tasks":[{"title":"t","status":"todo"}],
	  "knowledge_files":[{"path":"a.md","kb":"不存在库"}]}`
	srv := mockLLM(t, llmOut)
	svc, _, workdir := newParserWithKnowledge(t, srv.URL)
	_ = os.WriteFile(filepath.Join(workdir, "a.md"), []byte("# a"), 0o644)
	draft, err := svc.Parse(context.Background(), workdir, ParseInput{Content: "# x", SourceFile: "a.md"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := svc.Confirm(context.Background(), workdir, draft.ID); err == nil {
		t.Fatal("不存在库应整次失败")
	}
}
