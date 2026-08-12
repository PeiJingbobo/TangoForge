// Command daemon 是 TangoForge 守护进程入口（单守护进程 · 多项目，常驻后台）。
//
// 架构约定（QA Q5 已确认）：守护进程不绑定单一工作目录，所有项目路径由
// 每次请求的 X-Project / ?project= / MCP 参数显式指定。
//
// 启动流程（TF-003）：
//  1. 加载全局配置（~/.taskboard-app/config.yaml）；
//  2. 首次启动生成 ui_token 并持久化；
//  3. 单实例锁：PID 文件记录 + 端口占用检测（Server.Serve 兜底）；
//  4. 打开全局注册表库（registry.db，自动迁移）；
//  5. 启动 HTTP 服务（/ping + /api/* 中间件链）；
//  6. 全局配置热重载：remote_access 内存切换 + 端口动态重绑（QA Q8）。
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"tangoforge/internal/api"
	"tangoforge/internal/config"
	"tangoforge/internal/db"
	"time"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve home dir: %v\n", err)
		os.Exit(1)
	}

	configPath := flag.String("config", config.GlobalConfigPath(home), "全局配置文件路径（默认 ~/.taskboard-app/config.yaml）")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger, *configPath); err != nil {
		logger.Error("daemon exited with error", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger, configPath string) error {
	// 1. 加载全局配置（缺失时返回默认值）。
	cfg, err := config.LoadGlobal(configPath)
	if err != nil {
		return fmt.Errorf("load global config: %w", err)
	}

	// 2. UI 会话凭据：首次启动生成并持久化（QA 默认项）。
	if cfg.UIToken == "" {
		tok, err := config.GenerateToken()
		if err != nil {
			return fmt.Errorf("generate ui_token: %w", err)
		}
		cfg.UIToken = tok
		if err := config.SaveGlobal(configPath, cfg); err != nil {
			return fmt.Errorf("persist ui_token: %w", err)
		}
		logger.Info("generated ui_token and saved to global config")
	}

	home, _ := os.UserHomeDir()

	// 3. 单实例锁：PID 文件记录身份；端口占用检测由 Server.Serve 兜底（二次启动拦截）。
	pidPath := filepath.Join(home, ".taskboard-app", "daemon.pid")
	if err := acquirePIDFile(pidPath); err != nil {
		return err
	}
	defer func() { _ = os.Remove(pidPath) }()

	// 4. 全局注册表库（自动迁移）。
	registry, err := db.EnsureGlobal(ctx, db.RegistryDBPath(home))
	if err != nil {
		return err
	}
	defer func() { _ = registry.Close() }()

	// 5. HTTP 服务。
	srv := api.NewServer(&cfg, registry, logger, configPath, home)

	// 5.1 知识库扫描器：注册全部已导入项目 + 启动扫描/监听（TF-048/050）。
	srv.StartKnowledgeScanner()

	// 6. 全局配置热重载：remote_access 立即生效 + 端口动态重绑（失败保留旧端口）。
	stopWatch, err := config.WatchGlobal(configPath, func(next config.GlobalConfig) {
		srv.SetConfig(&next)
		if err := srv.ReloadPort(next.Port); err != nil {
			logger.Error("port hot reload failed, keep old port", "port", next.Port, "err", err)
		}
	})
	if err != nil {
		return fmt.Errorf("watch global config: %w", err)
	}
	defer stopWatch()

	// 7. 服务启动与退出。
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()
	logger.Info("daemon started", "mode", "multi-project", "config", configPath, "registry", db.RegistryDBPath(home))

	// 端口热重载会关闭旧监听器，使主 Serve 返回 nil（预期行为，服务已由新监听器接管），
	// 此处仅处理真实错误与退出信号；收到 nil 且仍有活跃监听器时继续常驻。
	// 注意：ReloadPort 自行启动新监听器的 Serve，因此主 Serve 返回后不重复启动。
	mainServeDone := false
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return srv.Shutdown(shutdownCtx)
		case err := <-errCh:
			if err != nil {
				// 真实监听错误；热重载时旧监听器返回 net.ErrClosed，已被 Serve 归一化为 nil。
				return err
			}
			if mainServeDone && !srv.Alive() {
				return nil
			}
			// 主 Serve 首次正常返回（热重载触发）：守护进程由 ReloadPort 的新监听器继续服务，
			// 保持常驻等待退出信号。
			mainServeDone = true
		}
	}
}

// acquirePIDFile 写入自身 PID 文件（供运维与 CLI 排查；端口占用检测为单实例锁第二道防线）。
func acquirePIDFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir pid dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return fmt.Errorf("write pid file %s: %w", path, err)
	}
	return nil
}
