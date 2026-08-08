package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFindDaemonBinary_EnvPrecedence：TANGOFORGE_DAEMON 优先（QA 2026-08-08 Q6）。
func TestFindDaemonBinary_EnvPrecedence(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "tangoforge-daemon.exe")
	if err := os.WriteFile(fake, []byte("x"), 0o755); err != nil {
		t.Fatalf("write fake daemon: %v", err)
	}
	t.Setenv("TANGOFORGE_DAEMON", fake)
	if got := findDaemonBinary(); got != fake {
		t.Fatalf("env 应优先返回 %q，got %q", fake, got)
	}

	// env 指向不存在文件 → 回退（此处同目录/PATH 无命中 → 空）。
	t.Setenv("TANGOFORGE_DAEMON", filepath.Join(dir, "missing"))
	got := findDaemonBinary()
	if got != "" {
		t.Fatalf("无效 env 应回退为空，got %q（若测试环境 PATH 有 daemon 则跳过此断言）", got)
	}
}

// TestExtractGlobal_NoLift：--no-lift 解析。
func TestExtractGlobal_NoLift(t *testing.T) {
	g, rest := extractGlobal([]string{"list", "--project", "/x", "--no-lift", "--json"})
	if !g.NoLift {
		t.Fatal("--no-lift 应被解析")
	}
	if !g.JSON {
		t.Fatal("--json 应被解析")
	}
	if len(rest) != 3 || rest[0] != "list" {
		t.Fatalf("rest = %v", rest)
	}
}

// TestEnsureDaemon_NoLift：--no-lift 下守护进程未运行 → 「命令无法完成」提示（QA Q7）。
func TestEnsureDaemon_NoLift(t *testing.T) {
	c := newCLIClient(cliGlobal{Server: "127.0.0.1:1"}) // 必 ping 失败端口
	err := c.ensureDaemon(true)
	if err == nil {
		t.Fatal("应返回错误")
	}
	if !strings.Contains(err.Error(), "命令无法完成") {
		t.Fatalf("错误应含「命令无法完成」: %v", err)
	}
}
