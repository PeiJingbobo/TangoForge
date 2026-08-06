# TangoForge 统一构建 / 测试 / 质量入口（vibe coding 最佳实践）
#
# Linux / macOS / CI 使用 make；Windows 本地可执行等价的 go / pnpm 命令
# （命令对照见 docs/AGENTS.md「开发与测试流程」§12）。

GO      ?= go
GOLANGCI ?= golangci-lint

.PHONY: all fmt vet lint test test-cover test-integration build build-daemon build-cli build-all dev check

all: check

## 代码格式：gofmt（Go）+ prettier（前端）
fmt:
	$(GO) fmt ./...
	pnpm format

## 静态检查：go vet（Go）+ ESLint（前端）
vet:
	$(GO) vet ./...

lint: vet
	pnpm lint

## 测试：Go 单元测试 + 前端 Vitest
test:
	$(GO) test ./...
	pnpm test

## 覆盖率：Go 全量 + internal/task ≥ 90% 门槛（scripts/check_coverage.sh）+ 前端覆盖率
test-cover:
	$(GO) test -cover ./...
	./scripts/check_coverage.sh
	pnpm test:coverage

## 集成测试（启动临时 Daemon + 真实 HTTP 客户端，见 test/integration）
test-integration:
	$(GO) test -tags=integration ./test/integration/...

## 本地构建
build: build-daemon build-cli

build-daemon:
	$(GO) build -o bin/tangoforge-daemon ./cmd/daemon

build-cli:
	$(GO) build -o bin/tangoforge ./cmd/cli

## 交叉编译 6 产物：Windows / macOS / Linux × x64 / arm64，全部 CGO_ENABLED=0（docs/TECHNICAL.md §1.3）
build-all:
	@mkdir -p bin/release
	@for os in windows darwin linux; do \
	  for arch in amd64 arm64; do \
	    ext=""; \
	    [ "$$os" = "windows" ] && ext=".exe"; \
	    echo ">> $$os/$$arch"; \
	    CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -o bin/release/tangoforge-$$os-$$arch$$ext ./cmd/daemon && \
	    CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -o bin/release/tangoforge-cli-$$os-$$arch$$ext ./cmd/cli || exit 1; \
	  done; \
	done
	@echo "✓ 6 产物构建完成（bin/release）"

## 前端开发服务器
dev:
	pnpm dev

## 提交前全量检查（本地 CI 等价物，与 .github/workflows 对齐）
check: fmt vet lint test test-cover test-integration build
	pnpm typecheck
	@echo "✓ 全部检查通过"
