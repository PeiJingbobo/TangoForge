# TF-018 Markdown 导入与草稿流 — 任务总结
> 结果：成功　|　日期：2026-08-06　|　执行人：ai

## 1. 任务范围
实现 Markdown → 结构化任务的 LLM 解析草稿流：解析（LLM 严格 Schema）→ 草稿（import_drafts）→ 确认入库（source_file 文件级全量覆盖，事务原子）→ 丢弃；替换 TF-014 的 import 占位端点。

## 2. 交付内容
- **新增文件**：
  - `internal/task/import.go` — `ImportTasks`（Service 接口追加）：事务内归档旧 source_file 任务 + 批量 INSERT；事务外校验（title/status/依赖存在性/本批内部环检测）；不触发 task.* 钩子；`ImportResult{Created, Archived}`
  - `internal/task/import_test.go` — 8 用例（归档+插入、6 项校验矩阵、无钩子、依赖库内旧任务、同文件重导入、空列表）
  - `internal/parser/schema.go` — ParseResult/ParsedTask、prompt 构造、status 映射、标题依赖解析、flattenTasks
  - `internal/parser/parser.go` — Service：`Parse`（含失败事件）/`List`（pending）/`Confirm`/`Discard`；事件 draft_ready/confirmed/discarded/failed
  - `internal/parser/parser_test.go` — 15 用例（草稿创建、label 映射、5 项失败矩阵、LLM 未配置、确认全流程含嵌套/依赖、重导入归档、草稿 404、丢弃、pending 列表、file_path 模式、flatten/resolve/JSON round-trip）
  - `internal/api/handlers_imports.go` — 4 端点替换占位
  - `internal/api/handlers_imports_test.go` — 6 用例（全流程、丢弃、LLM 未配置 422、解析失败 422、agent 默认 403、草稿 404）
- **修改文件**：
  - `internal/api/server.go` — parserSvc 组装（LLM provider 热重载 + OnEvent 双通道 audit/hub）；路由替换
  - `internal/api/errors.go` — 映射 `IMPORT_FAILED`(422) / `DRAFT_NOT_FOUND`(404) / `LLM_*`(422)
  - `internal/llm/client.go` — `ErrorCode` 导出（错误 → 业务码映射）
  - `internal/api/handlers_ws_test.go` — 占位测试改为 export-only
  - `docs/TASK-SEMANTICS.md` — 新增 §17（导入草稿流语义）
  - `docs/task/TASKS.md` / `OVERVIEW.md` — 状态 ✅、统计同步
- **关键实现点**：
  1. WAL 写锁铁律：ImportTasks 事务外完成全部校验，事务内首语句即写（归档 UPDATE）
  2. status 严格映射（key/label，失败整次失败）+ priority 复用 task.NormalizePriority
  3. depends_on 标题引用 → UUID（确认时解析）
  4. Parse 事件统一出口（成功 draft_ready / 失败 failed）

## 3. 验证结果
- `go vet ./...` → 干净
- `CGO_ENABLED=0 go test ./internal/task/... ./internal/parser/... ./internal/api/...` → **ok**
- `CGO_ENABLED=0 go test ./...` → **全仓全绿**
- `bash ./scripts/check_coverage.sh` → **91.7% ≥ 90%** 通过

## 4. 遗留问题与后续
- 真实 DeepSeek LLM 冒烟未执行（环境变量未配置）；M4 人工验证时配置后跑真实导入草稿流。
- TF-017 `import_preview/confirm/discard` MCP 工具与 TF-021 CLI import 子命令将复用本 Service。
- `import.failed` 事件 target 为空串（无草稿 ID 可指）；前端可在 UI 提示层展示错误信息（错误响应含 LLM 原始输出）。
