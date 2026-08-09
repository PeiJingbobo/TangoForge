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

// TestFindDaemonNear_Symlink：符号链接执行（~/bin/tangoforge → 仓库 bin/）时，
// os.Executable 返回链接路径 → findDaemonNear 应解析真实路径并命中同目录 daemon
// （QA 2026-08-09：AI 经 shell 运行 tangoforge 触发拉起的根因）。
func TestFindDaemonNear_Symlink(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real-bin")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cli := filepath.Join(realDir, "tangoforge")
	daemon := filepath.Join(realDir, "tangoforge-daemon")
	for _, p := range []string{cli, daemon} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	link := filepath.Join(root, "tangoforge")
	if err := os.Symlink(cli, link); err != nil {
		t.Skipf("symlink 不可用: %v", err)
	}
	// EvalSymlinks 会规范化 /var→/private/var 等，双方统一解析后对比。
	expect, err := filepath.EvalSymlinks(daemon)
	if err != nil {
		t.Fatalf("resolve daemon: %v", err)
	}
	if got := findDaemonNear(link); got != expect {
		t.Fatalf("符号链接执行应解析到真实同目录 daemon，got %q want %q", got, expect)
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
