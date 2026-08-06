# 集成测试（/test/integration）

按照 docs/TECHNICAL.md §3.8 的要求：

- 本目录存放**集成测试**：启动临时 Daemon（`cmd/daemon`，随机端口 + 临时工作目录），
  通过真实 HTTP 客户端调用接口验证端到端行为。
- 单元测试（`sqlite:memory:` 隔离、不依赖本地文件系统）放在各业务包内的 `_test.go` 中，
  不属于本目录。
- 约定：
  - 文件命名 `*_integration_test.go`，并在文件头声明 `//go:build integration`；
  - 本地/CI 通过 `go test -tags=integration ./test/integration/...` 运行（见 Makefile `test-integration`）；
  - 每个测试自行创建临时目录（`t.TempDir()`），结束后清理，不得污染用户环境；
  - 覆盖率要求见 docs/TECHNICAL.md §3.8（核心 internal/task ≥ 90%）。
