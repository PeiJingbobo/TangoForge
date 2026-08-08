package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// cliClient CLI 的 HTTP 客户端（TF-021：全部子命令转 HTTP 调用，多端等价）。
//
// 设计（QA P4-1 Q15-A）：
//   - --server 默认 127.0.0.1:19810；--actor 默认 human（X-Actor 头，agent 身份查权限表）；
//   - 任务类子命令 --project 强制（X-Project 头）；project 组子命令无 X-Project（与 HTTP 一致）；
//   - 自动拉起：/ping 失败 → 查找同目录 daemon 二进制并 spawn → 轮询 /ping（≤5s）；
//     找不到 daemon 则提示手动启动（TASKS.md 验收：可先提示）。
type cliClient struct {
	server string
	actor  string
	http   *http.Client
}

// cliGlobal 全局参数（--server / --actor / --json / --no-lift）。
type cliGlobal struct {
	Server string
	Actor  string
	JSON   bool
	// NoLift 禁用自动拉起（QA 2026-08-08 Q5：每次启动自动探活+拉起，--no-lift 可禁用）。
	NoLift bool
}

func newCLIClient(g cliGlobal) *cliClient {
	if g.Server == "" {
		g.Server = "127.0.0.1:19810"
	}
	if g.Actor == "" {
		g.Actor = "human"
	}
	return &cliClient{
		server: "http://" + g.Server,
		actor:  g.Actor,
		// LLM 调用（import preview / export template）可能需 60s+，CLI 侧给足 120s
		//（daemon 侧 LLM 超时独立由配置 TimeoutSec 控制，客户端断开不影响服务端继续执行）。
		http: &http.Client{Timeout: 120 * time.Second},
	}
}

// apiResp 统一响应（成功 {code:0,data}；错误 {code,message,detail}）。
// code 兼容 number 0 与字符串错误码（如 PROJECT_NOT_FOUND）。
type apiResp struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
	Detail  string          `json:"detail"`
	Data    json.RawMessage `json:"data"`
}

// ok 判断业务成功（code==0 数字）。
func (r *apiResp) ok() bool {
	return len(r.Code) == 0 || string(r.Code) == "0"
}

// call 执行 HTTP 请求并返回统一响应；非 2xx 或业务 code!=0 返回错误。
func (c *cliClient) call(method, path string, project string, body any) (*apiResp, error) {
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求: %w", err)
		}
		rd = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.server+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Actor", c.actor)
	req.Header.Set("Content-Type", "application/json")
	if project != "" {
		req.Header.Set("X-Project", project)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败（守护进程未运行？）: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var out apiResp
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("响应解析失败: %v body=%s", err, string(data))
	}
	if resp.StatusCode >= 400 || !out.ok() {
		code := string(out.Code)
		if code == "0" || code == "" {
			code = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return &out, fmt.Errorf("[%s] %s %s", code, out.Message, out.Detail)
	}
	return &out, nil
}

// ensureDaemon 确保守护进程运行：/ping 成功即返回；失败尝试自动拉起（QA 2026-08-08 Q5/Q7）：
//   - 默认每次启动自动探活+拉起（静默，detached 常驻）；--no-lift 禁用自动拉起；
//   - 拉起失败/找不到二进制 → 返回「命令无法完成」类提示（不输出 daemon 日志）。
func (c *cliClient) ensureDaemon(noLift bool) error {
	if c.ping() {
		return nil
	}
	if noLift {
		return errors.New("命令无法完成：守护进程未运行（--no-lift 已禁用自动拉起）")
	}
	daemon := findDaemonBinary()
	if daemon == "" {
		return errors.New("命令无法完成：守护进程未运行且未找到 daemon 二进制，请先启动 App 或运行 tangoforge-daemon")
	}
	cmd := exec.Command(daemon)
	// 完全静默：daemon 日志不接 CLI 输出（QA Q7）。
	cmd.Stdout = nil
	cmd.Stderr = nil
	// detached：CLI 退出后 daemon 常驻（与 App 拉起行为一致）。
	setDaemonDetached(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("命令无法完成：拉起守护进程失败: %w", err)
	}
	_ = cmd.Process.Release()
	// 轮询 /ping（≤5s）。
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if c.ping() {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("命令无法完成：守护进程启动超时（5s），请检查 App 是否正常")
}

// ping 健康检查。
func (c *cliClient) ping() bool {
	resp, err := c.http.Get(c.server + "/ping")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// findDaemonBinary 查找 daemon 二进制（QA 2026-08-08 Q6）：
// 优先级 TANGOFORGE_DAEMON env > CLI 同目录 > PATH。
func findDaemonBinary() string {
	if env := os.Getenv("TANGOFORGE_DAEMON"); env != "" {
		if info, err := os.Stat(env); err == nil && !info.IsDir() {
			return env
		}
	}
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for _, name := range daemonNames() {
			candidate := filepath.Join(dir, name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	for _, name := range daemonNames() {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// daemonNames 平台相关 daemon 二进制名。
func daemonNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"tangoforge-daemon.exe", "tangoforge-daemon"}
	}
	return []string{"tangoforge-daemon"}
}

// requireProject 校验并规范化 --project（必填 + ~ 展开 + 绝对化，防止引号包裹 ~ 或相对路径
// 导致 X-Project 与服务端工作目录不匹配）。
func requireProject(project string) (string, error) {
	if strings.TrimSpace(project) == "" {
		return "", errors.New("缺少必填参数 --project <工作目录>（任务/导入/导出等子命令必须显式指定项目）")
	}
	expanded := expandTilde(project)
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("解析项目路径 %s: %w", project, err)
	}
	return abs, nil
}

// expandTilde 展开开头的 ~ 为用户主目录。
func expandTilde(p string) string {
	if p == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

// printOutput 输出：--json 打印原始 data JSON；否则格式化（调用方提供人类可读文本）。
func printOutput(g cliGlobal, resp *apiResp, human func(data json.RawMessage) string) {
	if g.JSON || resp.Data == nil {
		fmt.Println(string(resp.Data))
		return
	}
	fmt.Println(human(resp.Data))
}
