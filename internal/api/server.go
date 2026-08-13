// Package api 是 TangoForge 传输层：HTTP 路由与中间件（薄封装，不做业务判断）。
//
// 分层铁律（AGENTS.md §3.2）：本包只做参数解析与响应格式化，禁止重复业务逻辑；
// 业务实现由 task / project 等业务层提供，中间件所需的最小数据查询直接复用 db 包原生 SQL。
//
// TF-003 范围：/ping 健康检查、来源过滤（remote_access）、X-Project 注册校验、
// 端口热重载（QA Q8 完整热重载）；
// TF-013 起：来源识别 + 权限中间件（internal/auth）+ 核心业务端点。
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"tangoforge/internal/audit"
	"tangoforge/internal/auth"
	"tangoforge/internal/config"
	"tangoforge/internal/exporter"
	"tangoforge/internal/knowledge"
	"tangoforge/internal/llm"
	"tangoforge/internal/mcp"
	"tangoforge/internal/parser"
	"tangoforge/internal/project"
	"tangoforge/internal/skill"
	"tangoforge/internal/task"

	"github.com/go-chi/chi/v5"
)

// Server 守护进程 HTTP 服务：持有可热切换的全局配置、全局注册表库连接与业务依赖。
type Server struct {
	cfgMu sync.RWMutex
	cfg   *config.GlobalConfig
	// configPath 全局配置文件路径（PUT /api/config 写盘；测试可留空仅热更新）。
	configPath string

	registry *sql.DB
	logger   *slog.Logger

	// TF-013 业务依赖（NewServer 内部自组装并接线）。
	tasks    task.Service
	projects *project.Service
	perms    *auth.PermissionStore
	audit    *audit.Store
	// TF-014 WS 事件广播中心。
	hub *hub
	// TF-020 Skill 业务服务。
	skills *skill.Service
	// TF-018 导入解析服务（LLM 草稿流）。
	parserSvc *parser.Service
	// TF-019 导出服务（Markdown 渲染 + LLM 模板生成）。
	exporterSvc *exporter.Service
	// TF-050 知识库业务服务 + 文件扫描器。
	knowledgeSvc     knowledge.Service
	knowledgeScanner *knowledge.Scanner
	// TF-016 远程 MCP HTTP 传输（/mcp，惰性初始化一次）。
	mcpOnce    sync.Once
	mcpHandler http.Handler

	httpSrv  *http.Server
	lnMu     sync.Mutex
	listener net.Listener

	// TF-053 守护进程生命周期控制：空闲重启（请求 daemon 在完成当前任务后重启）。
	restartMu sync.Mutex
	// restartIntent 空闲重启意图：非空 = 已请求，值为新 daemon 二进制路径（APP 提供）。
	// 由 main 的退出循环读取（RequestRestart 只是标记，Shutdown 等待活跃请求完成）。
	restartIntent string
	// onRestartRequested 空闲重启回调（main 注入：标记后由主循环 Shutdown → 重生）。
	onRestartRequested func(binPath string)
}

// NewServer 构造 HTTP 服务并自组装业务依赖。
//
// cfg 为初始配置指针；热重载（setConfig / ReloadPort）会原子替换内部状态。
// registry 为全局注册表库连接（已迁移），用于 X-Project 注册校验。
// configPath 为全局配置文件路径（PUT /api/config 写盘用；传 "" 则仅热更新不落盘）。
// homeDir 为 os.UserHomeDir() 结果（skill 全局技能库 / guide 使用）。
// 审计 / 权限 / 任务服务的接线（QA P3-1）：task 写钩子 → audit（result=ok）；
// 权限中间件 OnDenied → audit（result=denied）；actor 经 ctx（auth.ActorFrom）读取。
func NewServer(cfg *config.GlobalConfig, registry *sql.DB, logger *slog.Logger, configPath, homeDir string) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	auditStore := audit.NewStore(logger)
	permStore := auth.NewPermissionStore(logger)
	permStore.OnDenied = func(ctx context.Context, workdir, action string) {
		actor := auth.ActorFrom(ctx)
		auditStore.Write(ctx, workdir, audit.Entry{
			Actor: actor.Name, ActorClass: actor.Class,
			Action: action, Target: workdir, Result: audit.ResultDenied,
			Detail: "permission denied",
		})
	}
	hub := newHub()
	taskSvc := task.NewService(task.Options{
		Logger: logger,
		OnWrite: func(ctx context.Context, workdir, action, target string) {
			// 写钩子双通道：异步审计（TF-012）+ WS 事件广播（TF-014）。
			actor := auth.ActorFrom(ctx)
			auditStore.Write(ctx, workdir, audit.Entry{
				Actor: actor.Name, ActorClass: actor.Class,
				Action: action, Target: target, Result: audit.ResultOK,
			})
			hub.Publish(workdir, action, map[string]any{"id": target})
		},
	})

	s := &Server{
		cfg:        cfg,
		configPath: configPath,
		registry:   registry,
		logger:     logger,
		tasks:      taskSvc,
		projects:   project.NewService(registry, logger),
		perms:      permStore,
		audit:      auditStore,
		hub:        hub,
		skills:     skill.NewService(logger, homeDir),
	}

	// TF-050 知识库服务（写钩子 → 审计 + WS 事件；parser/exporter 依赖其只读接口）。
	knowSvc := knowledge.NewService(knowledge.Options{
		Logger: logger,
		// task.Service → TaskLister 适配（返回类型转换）。
		Tasks: knowledge.TaskListerAdapter(func(ctx context.Context, workdir, id string) (any, error) {
			return taskSvc.Get(ctx, workdir, id)
		}),
		LLM: s.newLLMClient(),
	})
	knowSvc.SetOnWrite(func(ctx context.Context, workdir, action, target string) {
		actor := auth.ActorFrom(ctx)
		auditStore.Write(ctx, workdir, audit.Entry{
			Actor: actor.Name, ActorClass: actor.Class,
			Action: action, Target: target, Result: audit.ResultOK,
		})
		hub.Publish(workdir, action, map[string]any{"id": target})
	})
	s.knowledgeSvc = knowSvc
	// 注入向量检索 embedding 配置（QA-K23：未配置模型 → 检索返回 NOT_CONFIGURED）。
	s.knowledgeSvc.SetEmbeddingConfig(s.embeddingConfig())
	// Scanner（daemon 启动时由 Start + RegisterWorkdir 接入）。
	s.knowledgeScanner = knowledge.NewScanner(knowSvc, s.currentConfig().Knowledge, s.embeddingConfig(), logger)

	s.parserSvc = parser.NewService(parser.Options{
		Logger: logger,
		LLM: func() config.LLMConfig {
			return s.currentConfig().LLM // 每次调用取最新（LLM 配置热重载即时生效）。
		},
		Tasks:     taskSvc,
		Knowledge: knowSvc,
		OnEvent: func(ctx context.Context, workdir, action, target string) {
			// 导入域事件双通道：异步审计 + WS 事件广播（与 task 写钩子同构）。
			actor := auth.ActorFrom(ctx)
			auditStore.Write(ctx, workdir, audit.Entry{
				Actor: actor.Name, ActorClass: actor.Class,
				Action: action, Target: target, Result: audit.ResultOK,
			})
			hub.Publish(workdir, action, map[string]any{"id": target})
		},
	})
	s.exporterSvc = exporter.NewService(exporter.Options{
		Logger:    logger,
		Tasks:     taskSvc,
		Knowledge: knowSvc,
		LLM: func() config.LLMConfig {
			return s.currentConfig().LLM
		},
		OnExport: func(ctx context.Context, workdir, action, target string) {
			actor := auth.ActorFrom(ctx)
			auditStore.Write(ctx, workdir, audit.Entry{
				Actor: actor.Name, ActorClass: actor.Class,
				Action: action, Target: target, Result: audit.ResultOK,
			})
			hub.Publish(workdir, action, map[string]any{"path": target})
		},
	})
	s.httpSrv = &http.Server{Handler: s.Handler()}
	return s
}

// Close 释放业务依赖（审计排空、权限/任务/Skill/parser 连接关闭）。
func (s *Server) Close() error {
	var firstErr error
	if s.audit != nil {
		if err := s.audit.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.perms != nil {
		if err := s.perms.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.tasks != nil {
		if err := s.tasks.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.skills != nil {
		if err := s.skills.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.parserSvc != nil {
		if err := s.parserSvc.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.knowledgeScanner != nil {
		s.knowledgeScanner.Stop()
	}
	if s.knowledgeSvc != nil {
		if err := s.knowledgeSvc.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// StartKnowledgeScanner 注册全部已导入项目并启动知识库扫描器（daemon 启动时调用）。
// scanner 未创建（测试环境）→ 静默跳过。
func (s *Server) StartKnowledgeScanner() {
	if s.knowledgeScanner == nil {
		return
	}
	// 注册全部已导入项目（project.Service.List 只返回可见项目；hidden 项目也需扫描，
	// 用 projects.List 覆盖全部——此处依赖可见即可，隐藏项目下次导入引导后补注册）。
	if list, err := s.projects.List(context.Background()); err == nil {
		for _, p := range list {
			s.knowledgeScanner.RegisterWorkdir(p.Workdir)
		}
	}
	_ = s.knowledgeScanner.Start(context.Background())
}

// SetConfig 原子替换当前配置（remote_access 等内存标志立即生效；供热重载回调调用）。
func (s *Server) SetConfig(cfg *config.GlobalConfig) {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	s.cfg = cfg
}

// currentConfig 返回当前配置副本（供中间件读取，避免并发读写竞态）。
func (s *Server) currentConfig() config.GlobalConfig {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	if s.cfg == nil {
		return config.DefaultGlobalConfig()
	}
	return *s.cfg
}

// currentConfigPtr 返回当前配置指针（识别中间件提供者：每次请求读取，热重载即时生效）。
func (s *Server) currentConfigPtr() *config.GlobalConfig {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg
}

// newLLMClient 懒构造 LLM 客户端（知识库摘要用；配置不可用 → nil 降级）。
func (s *Server) newLLMClient() *llm.Client {
	cl, err := llm.New(llm.FromConfig(s.currentConfig().LLM), s.logger)
	if err != nil {
		return nil
	}
	return cl
}

// embeddingConfig 返回当前全局配置下的向量嵌入配置（QA-K23；model 空 → nil = 检索禁用）。
func (s *Server) embeddingConfig() *llm.EmbeddingConfig {
	cfg := s.currentConfig().LLM
	if !llm.EmbeddingConfigured(cfg) {
		return nil
	}
	ec := llm.EmbeddingFromConfig(cfg)
	if ec.APIKey == "" {
		ec.APIKey = os.Getenv("DEEPSEEK_API_KEY")
	}
	return &ec
}

// perm 包装动作权限中间件为 http.HandlerFunc（chi 路由要求 HandlerFunc）。
func (s *Server) perm(action string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.perms.RequirePermission(action)(h).ServeHTTP(w, r)
	}
}

// Handler 组装路由与中间件链。
//
// /ping 不经过任何中间件（健康检查）；/api 下：
//  1. 来源过滤（非回环且未开启 remote_access → 403）；
//  2. /api/projects 组：**豁免 X-Project**（项目列表/导入无"当前项目"概念，QA P3-2），
//     仅经来源识别；DELETE 项目记录仅 UI（回环 + X-UI-Token）；
//  3. 其余 /api/*：X-Project 注册校验（未携带或未注册 → 404 PROJECT_NOT_FOUND）
//     → 来源识别（远程无 Token → 401）→ 动作权限（ui 放行，其余查 permissions 表 → 403）。
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	// CORS：Electron 渲染进程跨源访问（dev localhost:5173 / 生产 file://）；OPTIONS 预检 204。
	r.Use(s.corsMiddleware)
	r.Get("/ping", s.handlePing)
	// WS 事件订阅（独立于 /api 中间件链，handleWS 内自行完成来源过滤/项目校验/权限）。
	r.Get("/ws/events", s.handleWS)
	// AI 说明书端点（QA-S3 完全免鉴权）：GET /api/guide 任何来源可读，
	// 不经过 remoteAccessMiddleware / projectMiddleware / 来源识别（内容为只读能力描述）。
	r.Get("/api/guide", s.handleGuide)

	// 远程 MCP（QA P4-1 扩展）：Streamable HTTP 传输，挂载 /mcp。
	// 鉴权链：remote_access 过滤 → MCP 通道鉴权（远程必须 Bearer → 401；回环放行）。
	// 不经过来源识别中间件：MCP 恒为 agent 身份（actor 取会话 clientInfo.name），
	// 权限在工具执行时查 permissions 表（与 HTTP 等价）。
	r.Route("/mcp", func(r chi.Router) {
		r.Use(s.remoteAccessMiddleware)
		r.Use(s.mcpAuthMiddleware)
		r.Handle("/", s.MCPHandler())
	})

	r.Route("/api", func(r chi.Router) {
		r.Use(s.remoteAccessMiddleware)

		// 项目注册表组：豁免 projectMiddleware（QA P3-2）。
		r.Route("/projects", func(r chi.Router) {
			r.Use(auth.IdentifyMiddleware(s.currentConfigPtr))
			r.Get("/", s.handleProjectList)
			r.Post("/import", s.handleProjectImport)
			// TF-041 引导流程：目录前置检查 / 清空历史元数据（均豁免 X-Project）。
			r.Post("/check", s.handleProjectCheck)
			r.Post("/import/reset", s.handleProjectResetMetadata)
			// TF-043 引导完成（hidden→可见，仅 UI）。
			r.Post("/complete", s.handleProjectComplete)
			r.Patch("/{id}", s.handleProjectRename) // TF-035 重命名（仅 UI）
			r.Delete("/{id}", s.handleProjectRemove)
		})

		// 全局配置组：豁免 X-Project（全局首选项，与项目无关）；仅 UI（handler 内二次校验）。
		r.Route("/config", func(r chi.Router) {
			r.Use(auth.IdentifyMiddleware(s.currentConfigPtr))
			r.Get("/", s.handleConfigGet)
			r.Put("/", s.handleConfigPut)
			// TF-041 引导：LLM 连接测试（暂存配置，仅 UI）。
			r.Post("/test", s.handleConfigTestLLM)
		})

		// 全局 Skill 模板组（QA-S4）：豁免 X-Project（模板存于全局技能库，与项目无关）；
		// GET 任意识别来源（模板无敏感信息），PUT 仅 UI（handler 内二次校验）。
		r.Route("/skill-template", func(r chi.Router) {
			r.Use(auth.IdentifyMiddleware(s.currentConfigPtr))
			r.Get("/", s.handleSkillTemplateGet)
			r.Put("/", s.handleSkillTemplatePut)
		})

		// TF-053 守护进程生命周期控制：豁免 X-Project（daemon 级，与项目无关）。
		// GET version 免鉴权（版本探测）；POST restart 仅 UI（handler 内二次校验）。
		r.Route("/daemon", func(r chi.Router) {
			r.Use(auth.IdentifyMiddleware(s.currentConfigPtr))
			r.Get("/version", s.handleDaemonVersion)
			r.Post("/restart", s.handleDaemonRestart)
		})

		// 主业务组：X-Project → 来源识别 → 动作权限。
		r.Group(func(r chi.Router) {
			r.Use(s.projectMiddleware)
			r.Use(auth.IdentifyMiddleware(s.currentConfigPtr))

			r.Route("/tasks", func(r chi.Router) {
				r.Get("/", s.perm("task.read", s.handleTaskList))
				r.Post("/", s.perm("task.create", s.handleTaskCreate))
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", s.perm("task.read", s.handleTaskGet))
					r.Patch("/", s.perm("task.update", s.handleTaskUpdate))
					r.Post("/archive", s.perm("task.delete", s.handleTaskArchive))
					r.Post("/restore", s.perm("task.restore", s.handleTaskRestore))
					r.Delete("/", s.perm("task.delete", s.handleTaskDelete))
				})
			})

			r.Get("/state-machine", s.perm("state_machine.read", s.handleStateMachineGet))
			r.Put("/state-machine", s.perm("state_machine.write", s.handleStateMachinePut))

			// TF-032 项目配置（设置页数据源）：GET 读整份（state_machine.read）；
			// PUT 全量覆盖 state_machine+export，仅 UI（handler 内二次校验 actor==ui）。
			r.Get("/project-config", s.perm("state_machine.read", s.handleProjectConfigGet))
			r.Put("/project-config", s.handleProjectConfigPut)

			r.Get("/permissions", s.perm("permission.read", s.handlePermissionGet))
			// PUT /api/permissions 仅 UI（回环 + X-UI-Token 由识别层保证），handler 内二次校验 actor==ui。
			r.Put("/permissions", s.handlePermissionPut)

			// TF-014：graph / audit。
			r.Get("/graph", s.perm("graph.read", s.handleGraph))
			r.Get("/audit", s.perm("audit.read", s.handleAuditQuery))
			r.Get("/audit/export", s.perm("audit.read", s.handleAuditExport))

			// TF-018 已落地：Markdown 导入草稿流（parser）。
			r.Post("/import", s.perm("import.run", s.handleImport))
			r.Get("/import/drafts", s.perm("import.run", s.handleImportDrafts))
			r.Get("/import/drafts/{id}", s.perm("import.run", s.handleImportDraftGet))
			r.Put("/import/drafts/{id}/tasks", s.perm("import.run", s.handleImportDraftUpdateTasks))
			r.Post("/import/drafts/{id}/confirm", s.perm("import.confirm", s.handleImportDraftConfirm))
			r.Delete("/import/drafts/{id}", s.perm("import.run", s.handleImportDraftDiscard))
			// TF-019 已落地：Markdown 导出与模板（exporter）。
			r.Post("/export", s.perm("export.run", s.handleExport))
			r.Post("/export/template/generate", s.perm("export.run", s.handleExportTemplateGenerate))
			// TF-038：模板内容预览（导出对话框展示当前模板；llm 未生成 → TEMPLATE_INVALID）。
			r.Get("/export/template", s.perm("export.run", s.handleExportTemplateContent))

			// TF-033 已重构：Skill 技能包生命周期（获取/编辑/宿主安装/状态）。
			// 兼容旧端点 /api/skills、/api/skills/{name}（skill.read）语义迁移到包列表/详情。
			r.Get("/skills", s.perm("skill.read", s.handleSkills))
			r.Get("/skills/{name}", s.perm("skill.read", s.handleSkillInfo))
			r.Get("/skills/packages", s.perm("skill.read", s.handleSkillPackages))
			r.Get("/skills/packages/{name}", s.perm("skill.read", s.handleSkillPackageInfo))
			r.Put("/skills/packages/{name}", s.handleSkillPackageWrite) // 仅 UI（handler 二次校验）
			r.Get("/skills/status", s.perm("skill.read", s.handleSkillStatus))
			r.Post("/skills/install", s.perm("skill.install", s.handleSkillInstall))
			r.Post("/skills/uninstall", s.perm("skill.install", s.handleSkillUninstall))

			// TF-050：知识库端点（docs/KNOWLEDGE-BASE.md §6）。
			r.Route("/knowledge", func(r chi.Router) {
				r.Get("/bases", s.perm("knowledge.read", s.handleKnowledgeBasesGet))
				r.Post("/bases", s.perm("knowledge.write", s.handleKnowledgeBasesCreate))
				r.Patch("/bases/{id}", s.perm("knowledge.write", s.handleKnowledgeBasePatch))
				r.Delete("/bases/{id}", s.perm("knowledge.write", s.handleKnowledgeBaseDelete))
				r.Get("/documents", s.perm("knowledge.read", s.handleKnowledgeDocumentsGet))
				r.Post("/documents", s.perm("knowledge.write", s.handleKnowledgeDocumentRegister))
				r.Get("/documents/{id}", s.perm("knowledge.read", s.handleKnowledgeDocumentGet))
				r.Get("/documents/{id}/content", s.perm("knowledge.read", s.handleKnowledgeDocumentContentGet))
				r.Put("/documents/{id}/content", s.perm("knowledge.write", s.handleKnowledgeDocumentContentPut))
				r.Post("/documents/{id}/relink", s.perm("knowledge.write", s.handleKnowledgeDocumentRelink))
				r.Delete("/documents/{id}", s.perm("knowledge.write", s.handleKnowledgeDocumentDelete))
				r.Post("/link", s.perm("knowledge.write", s.handleKnowledgeLink))
				r.Post("/unlink", s.perm("knowledge.write", s.handleKnowledgeUnlink))
				r.Get("/search", s.perm("knowledge.read", s.handleKnowledgeSearch))
				r.Post("/scan", s.perm("knowledge.index", s.handleKnowledgeScan))
				r.Get("/tasks/{taskId}", s.perm("knowledge.read", s.handleKnowledgeTaskDocuments))
			})
		})

		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "api root: 无端点定义", "")
		})
	})
	return r
}

// addr 返回当前配置下的监听地址（始终 0.0.0.0，由中间件按来源 IP 动态过滤，QA Q8）。
func (s *Server) addr() string {
	return net.JoinHostPort("0.0.0.0", strconv.Itoa(s.currentConfig().Port))
}

// Serve 启动监听并服务请求（阻塞）。端口被占用时返回错误（单实例锁第二道防线）。
func (s *Server) Serve() error {
	ln, err := net.Listen("tcp", s.addr())
	if err != nil {
		return fmt.Errorf("listen %s: %w（可能已有守护进程在运行）", s.addr(), err)
	}
	s.lnMu.Lock()
	s.listener = ln
	s.lnMu.Unlock()
	s.logger.Info("daemon listening", "addr", s.addr())
	if err := s.httpSrv.Serve(ln); err != nil &&
		!errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}

// ReloadPort 热重载监听端口（QA Q8）：先监听新端口（失败则保留旧端口），
// 成功后将新 listener 交给 http.Server 服务并关闭旧 listener。
func (s *Server) ReloadPort(port int) error {
	if port <= 0 {
		return fmt.Errorf("invalid port %d", port)
	}
	addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("reload listen %s: %w", addr, err)
	}
	s.lnMu.Lock()
	old := s.listener
	s.listener = ln
	s.lnMu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	go func() { _ = s.httpSrv.Serve(ln) }()
	s.logger.Info("daemon port reloaded", "addr", addr)
	return nil
}

// Alive 报告是否仍有正在服务的监听器。
// 端口热重载替换主监听器后返回 true（守护进程继续常驻）。
func (s *Server) Alive() bool {
	s.lnMu.Lock()
	defer s.lnMu.Unlock()
	return s.listener != nil
}

// Shutdown 优雅关闭全部监听与连接。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

// handlePing 健康检查（GET /ping，不经过中间件）。
func (s *Server) handlePing(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]string{"status": "ok"},
	})
}

// MCPHandler 返回远程 MCP 的 Streamable HTTP handler（惰性初始化一次）。
// 复用 daemon 既有业务依赖（taskSvc 已接 audit + WS 双通道，MCP 写操作事件正常广播）。
func (s *Server) MCPHandler() http.Handler {
	s.mcpOnce.Do(func() {
		srv := mcp.NewServer(mcp.Deps{
			Logger:           s.logger,
			Tasks:            s.tasks,
			Projects:         s.projects,
			Perms:            s.perms,
			Skills:           s.skills,
			Parser:           s.parserSvc,
			Exporter:         s.exporterSvc,
			Knowledge:        s.knowledgeSvc,
			KnowledgeScanner: s.knowledgeScanner,
		})
		s.mcpHandler = srv.HTTPHandler()
	})
	return s.mcpHandler
}

// mcpAuthMiddleware 远程 MCP 通道鉴权（QA P4-1）：
//   - 远程请求必须携带有效 Bearer api_token（与 /api 一致），否则 401；
//   - 回环请求放行（actor 由 MCP 会话 clientInfo 提供，权限在工具执行时查表）；
//   - MCP 通道不识别 UI 凭据（UI 走 /api，MCP 恒为 Agent 身份）。
func (s *Server) mcpAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r.RemoteAddr) {
			cfg := s.currentConfig()
			bearer := auth.BearerToken(r)
			if bearer == "" || cfg.APIToken == "" || !auth.SecureEqual(bearer, cfg.APIToken) {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED",
					"远程请求必须携带有效的 Authorization: Bearer <api_token>", "")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// projectFromRequest 从 X-Project 头或 ?project= 查询参数提取项目工作目录。
func projectFromRequest(r *http.Request) string {
	if p := r.Header.Get("X-Project"); p != "" {
		return p
	}
	return r.URL.Query().Get("project")
}

// isLoopback 判断来源地址是否为回环（127.0.0.1 / ::1）。
func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// writeJSON 写统一 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeError 写统一错误响应（QA Q9：{code, message, detail}，code 为业务错误码）。
func writeError(w http.ResponseWriter, status int, code, message, detail string) {
	body := map[string]string{
		"code":    code,
		"message": message,
	}
	if detail != "" {
		body["detail"] = detail
	}
	writeJSON(w, status, body)
}
