package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"tangoforge/internal/audit"
	"tangoforge/internal/auth"
	"tangoforge/internal/config"
	"tangoforge/internal/db"
	"tangoforge/internal/exporter"
	"tangoforge/internal/mcp"
	"tangoforge/internal/parser"
	"tangoforge/internal/project"
	"tangoforge/internal/skill"
	"tangoforge/internal/task"
)

// runMCPCommand 启动 stdio MCP 服务（tangoforge mcp [--config <全局配置路径>]）。
//
// QA P4-1 决策：与 CLI/daemon 同一二进制子命令，stdio 直连业务层（不经 HTTP）。
// MCP 进程独立于 daemon 运行，直接读写项目库（本地优先、数据是主人）；
// 写操作审计直接写项目库 audit_log（与 daemon 一致）；WS 事件不推送
// （跨进程限制，登记 TASK-SEMANTICS §16；远程 MCP 挂载 daemon 内则事件正常广播）。
func runMCPCommand(args []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve home dir: %v\n", err)
		os.Exit(1)
	}
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	configPath := fs.String("config", config.GlobalConfigPath(home), "全局配置文件路径（默认 ~/.taskboard-app/config.yaml）")
	_ = fs.Parse(args)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := serveMCP(ctx, logger, *configPath); err != nil {
		logger.Error("mcp exited with error", "err", err)
		os.Exit(1)
	}
}

// serveMCP 组装业务依赖并运行 stdio 传输（阻塞至 ctx 取消）。
func serveMCP(ctx context.Context, logger *slog.Logger, configPath string) error {
	if _, err := config.LoadGlobal(configPath); err != nil {
		return fmt.Errorf("load global config: %w", err)
	}

	home, _ := os.UserHomeDir()
	registry, err := db.EnsureGlobal(ctx, db.RegistryDBPath(home))
	if err != nil {
		return err
	}
	defer func() { _ = registry.Close() }()

	// 业务依赖组装（与 api.NewServer 同构；无 hub——stdio 独立进程不推送 WS 事件）。
	auditStore := audit.NewStore(logger)
	defer func() { _ = auditStore.Close() }()
	permStore := auth.NewPermissionStore(logger)
	defer func() { _ = permStore.Close() }()
	permStore.OnDenied = func(ctx context.Context, workdir, action string) {
		actor := auth.ActorFrom(ctx)
		auditStore.Write(ctx, workdir, audit.Entry{
			Actor: actor.Name, ActorClass: actor.Class,
			Action: action, Target: workdir, Result: audit.ResultDenied,
			Detail: "permission denied",
		})
	}
	taskSvc := task.NewService(task.Options{
		Logger: logger,
		OnWrite: func(ctx context.Context, workdir, action, target string) {
			actor := auth.ActorFrom(ctx)
			auditStore.Write(ctx, workdir, audit.Entry{
				Actor: actor.Name, ActorClass: actor.Class,
				Action: action, Target: target, Result: audit.ResultOK,
			})
		},
	})
	defer func() { _ = taskSvc.Close() }()
	skillSvc := skill.NewService(logger, home)
	defer func() { _ = skillSvc.Close() }()

	// parser / exporter（stdio 独立进程：事件仅接审计，WS 由 daemon 侧广播）。
	parserSvc := parser.NewService(parser.Options{
		Logger: logger,
		LLM: func() config.LLMConfig {
			cfg, _ := config.LoadGlobal(configPath)
			return cfg.LLM
		},
		Tasks: taskSvc,
		OnEvent: func(ctx context.Context, workdir, action, target string) {
			actor := auth.ActorFrom(ctx)
			auditStore.Write(ctx, workdir, audit.Entry{
				Actor: actor.Name, ActorClass: actor.Class,
				Action: action, Target: target, Result: audit.ResultOK,
			})
		},
	})
	defer func() { _ = parserSvc.Close() }()
	exporterSvc := exporter.NewService(exporter.Options{
		Logger: logger,
		Tasks:  taskSvc,
		LLM: func() config.LLMConfig {
			cfg, _ := config.LoadGlobal(configPath)
			return cfg.LLM
		},
		OnExport: func(ctx context.Context, workdir, action, target string) {
			actor := auth.ActorFrom(ctx)
			auditStore.Write(ctx, workdir, audit.Entry{
				Actor: actor.Name, ActorClass: actor.Class,
				Action: action, Target: target, Result: audit.ResultOK,
			})
		},
	})

	deps := mcp.Deps{
		Logger:   logger,
		Tasks:    taskSvc,
		Projects: project.NewService(registry, logger),
		Perms:    permStore,
		Skills:   skillSvc,
		Parser:   parserSvc,
		Exporter: exporterSvc,
	}
	srv := mcp.NewServer(deps)

	logger.Info("mcp stdio server started", "config", configPath)
	if err := srv.StdioServer().Listen(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("mcp stdio: %w", err)
	}
	return nil
}
