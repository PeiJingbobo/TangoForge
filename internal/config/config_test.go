package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultGlobalConfig(t *testing.T) {
	cfg := DefaultGlobalConfig()
	if cfg.Port != DefaultPort {
		t.Errorf("default port = %d, want %d", cfg.Port, DefaultPort)
	}
	if cfg.RemoteAccess {
		t.Error("remote_access should default to false")
	}
	llm := cfg.LLM
	if llm.TimeoutSec != 60 || llm.Retries != 1 || llm.MaxTokens != 4096 || llm.Concurrency != 1 {
		t.Errorf("default llm = %+v", llm)
	}
	// TF-046：embedding 默认（model 空 = 未配置；api_kind/timeout 有默认）。
	if llm.Embedding.APIKind != "openai" || llm.Embedding.TimeoutSec != 60 {
		t.Errorf("default embedding = %+v", llm.Embedding)
	}
	// TF-046/048：knowledge 全局默认（QA-K18 全开）。
	k := cfg.Knowledge
	if !k.EnabledOn() || !k.FSNotifyOn() || !k.StartupScanOn() || !k.VectorSearchOn() {
		t.Errorf("knowledge booleans should default on: %+v", k)
	}
	if k.DebounceMS != 30000 || k.EmbedConcurrency != 1 || k.MaxIndexSize != 524288 ||
		k.SearchTopK != 10 || k.SearchThreshold != 0.3 {
		t.Errorf("knowledge defaults wrong: %+v", k)
	}
}

func TestWithDefaults_KnowledgePartialConfig(t *testing.T) {
	// 只显式关闭 fsnotify，其余未写 → fsnotify=false，其余默认。
	cfg := GlobalConfig{Knowledge: KnowledgeGlobalConfig{FSNotify: boolPtr(false)}}
	got := cfg.WithDefaults()
	if got.Knowledge.FSNotifyOn() {
		t.Error("fsnotify 显式 false 应保留")
	}
	if !got.Knowledge.EnabledOn() || !got.Knowledge.StartupScanOn() || !got.Knowledge.VectorSearchOn() {
		t.Error("未设置开关应补默认 true")
	}
	if got.Knowledge.DebounceMS != 30000 {
		t.Errorf("debounce = %d, want 30000", got.Knowledge.DebounceMS)
	}
	// 显式 enabled=false 保留。
	cfg2 := GlobalConfig{Knowledge: KnowledgeGlobalConfig{Enabled: boolPtr(false)}}
	got2 := cfg2.WithDefaults()
	if got2.Knowledge.EnabledOn() {
		t.Error("enabled 显式 false 应保留")
	}
}

func TestLoadGlobal_MissingFileReturnsDefaults(t *testing.T) {
	// 文件不存在 → 默认值 + nil error（缺失容错）。
	cfg, err := LoadGlobal(filepath.Join(t.TempDir(), "nope", "config.yaml"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if cfg.Port != DefaultPort {
		t.Errorf("port = %d, want %d", cfg.Port, DefaultPort)
	}
}

func TestLoadGlobal_PartialFieldsGetDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// 只写 port 与 llm.model，其余字段缺失。
	content := "port: 20000\nllm:\n  model: qwen2.5\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadGlobal(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Port != 20000 {
		t.Errorf("port = %d, want 20000", cfg.Port)
	}
	if cfg.LLM.Model != "qwen2.5" {
		t.Errorf("model = %q", cfg.LLM.Model)
	}
	if cfg.LLM.TimeoutSec != 60 {
		t.Errorf("timeout should fall back to 60, got %d", cfg.LLM.TimeoutSec)
	}
}

func TestLoadGlobal_InvalidYAMLReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("port: [not-a-number"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadGlobal(path); err == nil {
		t.Error("expected error for invalid yaml")
	}
}

func TestSaveLoadGlobal_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	want := DefaultGlobalConfig()
	want.Port = 21000
	want.RemoteAccess = true
	want.APIToken = "tok-abc"
	want.LLM.BaseURL = "http://localhost:11434/v1"
	want.LLM.Model = "llama3"

	if err := SaveGlobal(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadGlobal(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Port != want.Port || got.RemoteAccess != want.RemoteAccess ||
		got.APIToken != want.APIToken || got.LLM.BaseURL != want.LLM.BaseURL ||
		got.LLM.Model != want.LLM.Model {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestDefaultProjectConfig(t *testing.T) {
	cfg := DefaultProjectConfig()
	if len(cfg.StateMachine.States) != 3 {
		t.Fatalf("default states = %d, want 3", len(cfg.StateMachine.States))
	}
	keys := map[string]bool{}
	for _, s := range cfg.StateMachine.States {
		keys[s.Key] = true
	}
	for _, k := range []string{"todo", "doing", "done"} {
		if !keys[k] {
			t.Errorf("default state machine missing %q", k)
		}
	}
	if len(cfg.StateMachine.Transitions) != 3 {
		t.Errorf("default transitions = %d, want 3", len(cfg.StateMachine.Transitions))
	}
	if cfg.Export.TemplatePath != "" {
		t.Errorf("template_path should be empty by default")
	}
}

func TestLoadProject_MissingFileReturnsDefaults(t *testing.T) {
	cfg, err := LoadProject(t.TempDir())
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if len(cfg.StateMachine.States) != 3 {
		t.Errorf("states = %d, want default 3", len(cfg.StateMachine.States))
	}
}

func TestLoadProject_CustomStateMachine(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".taskboard"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `
state_machine:
  states:
    - { key: backlog, label: 待排期, color: "#666" }
    - { key: done,    label: 完成,   color: "#0a0" }
  transitions:
    - { from: backlog, to: [done] }
export:
  template_path: "custom.tmpl"
`
	path := ProjectConfigPath(workdir)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadProject(workdir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.StateMachine.States) != 2 {
		t.Errorf("states = %d, want 2", len(cfg.StateMachine.States))
	}
	if cfg.StateMachine.States[0].Key != "backlog" {
		t.Errorf("first state = %q", cfg.StateMachine.States[0].Key)
	}
	if len(cfg.StateMachine.Transitions) != 1 || cfg.StateMachine.Transitions[0].From != "backlog" {
		t.Errorf("transitions = %+v", cfg.StateMachine.Transitions)
	}
	if cfg.Export.TemplatePath != "custom.tmpl" {
		t.Errorf("template_path = %q", cfg.Export.TemplatePath)
	}
}

func TestLoadProject_EmptyStatesFallsBackToDefault(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".taskboard"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "state_machine:\n  states: []\nexport: {}\n"
	if err := os.WriteFile(ProjectConfigPath(workdir), []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadProject(workdir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.StateMachine.States) != 3 {
		t.Errorf("states should fall back to default 3, got %d", len(cfg.StateMachine.States))
	}
}

func TestSaveProject_RoundTrip(t *testing.T) {
	workdir := t.TempDir()
	cfg := DefaultProjectConfig()
	cfg.Export.TemplatePath = "my.tmpl"
	if err := SaveProject(workdir, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadProject(workdir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Export.TemplatePath != "my.tmpl" {
		t.Errorf("template_path = %q", got.Export.TemplatePath)
	}
	if len(got.StateMachine.States) != 3 {
		t.Errorf("states = %d", len(got.StateMachine.States))
	}
}

func TestGenerateToken(t *testing.T) {
	a, err := GenerateToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	b, err := GenerateToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(a) != 32 {
		t.Errorf("token length = %d, want 32", len(a))
	}
	if a == b {
		t.Error("two tokens should differ")
	}
}

// TestWatchGlobal_HotReload 验证 fsnotify 热重载：修改配置后回调收到新值。
func TestWatchGlobal_HotReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// 初始配置：port 19810。
	initial := DefaultGlobalConfig()
	if err := SaveGlobal(path, initial); err != nil {
		t.Fatalf("save initial: %v", err)
	}

	updated := make(chan GlobalConfig, 4)
	stop, err := WatchGlobal(path, func(cfg GlobalConfig) {
		select {
		case updated <- cfg:
		default:
		}
	})
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	defer stop()

	// 修改配置：port → 20001。
	next := initial
	next.Port = 20001
	if err := SaveGlobal(path, next); err != nil {
		t.Fatalf("save updated: %v", err)
	}

	select {
	case cfg := <-updated:
		if cfg.Port != 20001 {
			t.Errorf("reloaded port = %d, want 20001", cfg.Port)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hot reload did not fire within 5s")
	}
}

func TestConfigPaths(t *testing.T) {
	if got := GlobalConfigPath(`C:\Users\me`); got != filepath.Join(`C:\Users\me`, ".taskboard-app", "config.yaml") {
		t.Errorf("GlobalConfigPath = %s", got)
	}
	if got := ProjectConfigPath(`D:\work`); got != filepath.Join(`D:\work`, ".taskboard", "config.yaml") {
		t.Errorf("ProjectConfigPath = %s", got)
	}
}

// ---------- UpdateProjectFile（部分更新，保留未知节，TF-032） ----------

func TestUpdateProjectFile_PreservesUnknownSections(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".taskboard"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `
state_machine:
  states:
    - { key: todo, label: 待办, color: "#9aa0a6" }
export:
  template_path: "custom.tmpl"
future_feature:
  enabled: true
  note: 未知扩展节，必须原样保留
`
	if err := os.WriteFile(ProjectConfigPath(workdir), []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := UpdateProjectFile(workdir, func(cfg *ProjectConfig) {
		cfg.StateMachine.States = []State{
			{Key: "backlog", Label: "待排期", Color: "#666"},
			{Key: "done", Label: "完成", Color: "#0a0"},
		}
		cfg.Export.TemplatePath = "new.tmpl"
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	// 已知节已更新。
	cfg, err := LoadProject(workdir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.StateMachine.States) != 2 || cfg.StateMachine.States[0].Key != "backlog" {
		t.Errorf("states = %+v", cfg.StateMachine.States)
	}
	if cfg.Export.TemplatePath != "new.tmpl" {
		t.Errorf("template_path = %q", cfg.Export.TemplatePath)
	}
	// 未知节原样保留。
	data, err := os.ReadFile(ProjectConfigPath(workdir))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	raw := string(data)
	if !strings.Contains(raw, "future_feature:") || !strings.Contains(raw, "未知扩展节，必须原样保留") {
		t.Errorf("unknown section lost:\n%s", raw)
	}
}

func TestUpdateProjectFile_MissingFileCreatesWithDefaults(t *testing.T) {
	workdir := t.TempDir()
	err := UpdateProjectFile(workdir, func(cfg *ProjectConfig) {
		cfg.Export.TemplatePath = "my.tmpl"
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	cfg, err := LoadProject(workdir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.StateMachine.States) != 3 {
		t.Errorf("states = %d, want default 3", len(cfg.StateMachine.States))
	}
	if cfg.Export.TemplatePath != "my.tmpl" {
		t.Errorf("template_path = %q", cfg.Export.TemplatePath)
	}
}

func TestUpdateProjectFile_EmptyStatesFallsBackToDefault(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".taskboard"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ProjectConfigPath(workdir), []byte("state_machine: {}\nexport: {}\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := UpdateProjectFile(workdir, func(_ *ProjectConfig) {})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	cfg, err := LoadProject(workdir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.StateMachine.States) != 3 {
		t.Errorf("states should fall back to default 3, got %d", len(cfg.StateMachine.States))
	}
}

func TestUpdateProjectFile_InvalidYAMLReturnsError(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".taskboard"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ProjectConfigPath(workdir), []byte("state_machine: [broken"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := UpdateProjectFile(workdir, func(*ProjectConfig) {}); err == nil {
		t.Error("expected error for invalid yaml")
	}
}
