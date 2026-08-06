# TF-017 MCP 工具全集 — 任务总结
> 结果：成功　|　日期：2026-08-06　|　执行人：ai

## 1. 任务范围
补齐 v1 固定 MCP 工具集（19 个）：task/project/import/export/graph/state_machine/skill/permission 全域；QA P4-1 Q6 新增 `project_create` 并拆分 import/init 语义；graph 数据组装下沉 task 包供双端复用。

## 2. 交付内容
- **新增文件**：
  - `internal/mcp/tools_project.go` — project_list（全局豁免）/ project_import（仅导入）/ project_init（仅初始化）/ project_create（init+import）
  - `internal/mcp/tools_import.go` — import_preview / import_confirm / import_discard（parser 复用）
  - `internal/mcp/tools_export.go` — export_markdown（exporter 复用）
  - `internal/mcp/tools_graph.go` — graph_get（task.Graph 复用）
  - `internal/mcp/tools_state_machine.go` — state_machine_get/update（state_machine 对象参数）
  - `internal/mcp/tools_skill.go` — skill_info
  - `internal/mcp/tools_permission.go` — permission_list
  - `internal/mcp/mcp_tools_test.go` — 6 用例（全工具清单 19、生命周期全链路、未初始化 import 报错、skill_info、denied 路径）
  - `internal/task/graph.go` + `graph_test.go` — Service.Graph（排除 archived + parent/dependency 边，空数组友好）
  - `internal/project/project_init_test.go` — Init/ImportExisting/Create 3 用例
- **修改文件**：
  - `internal/mcp/tools_task.go` — task_list/update/archive/restore 补充（parent_id 三重态）
  - `internal/mcp/mcp.go` — Deps 注入 Parser/Exporter；registerTools 19 工具
  - `internal/project/project.go` — Init / ImportExisting / Create（QA P4-1 Q6）
  - `internal/api/handlers_graph.go` — 薄封装（委托 task.Graph）
  - `cmd/cli/cmd_mcp.go` / `api/server.go` — Deps 注入 Parser/Exporter
  - `docs/TASK-SEMANTICS.md` — §16.4 工具集登记
  - `docs/task/TASKS.md` / `OVERVIEW.md` — 状态 ✅
- **关键实现点**：
  1. project 三工具语义（import 仅导入 / init 仅初始化 / create 一条龙），HTTP import 兼容不变
  2. project 域豁免权限（与 /api/projects 组一致）
  3. graph 组装下沉 task 包（传输层薄封装）
  4. 权限与 HTTP 查同一表（Require），denied 审计；业务错误放 result

## 3. 验证结果
- `go vet ./...` → 干净
- `CGO_ENABLED=0 go test ./...` → **12 包全绿**
- `bash ./scripts/check_coverage.sh` → **91.4% ≥ 90%** 通过
- 覆盖：tools/list 19 工具齐全；project_create → 任务创建/读取/更新/归档/还原 → graph/状态机/权限 → import_preview/confirm → export_markdown 全链路 stdio 冒烟；denied 路径（task.update / state_machine.write 默认拒绝）

## 4. 遗留问题与后续
- TF-021 CLI 子命令（最后一个 P4 任务）将复用全部业务层（HTTP 调用等价）。
- stdio MCP 独立进程的 parser/exporter 事件仅接审计（无 WS，跨进程限制，§16.1）。
- `project_list` 无 project 参数（全局操作，豁免登记 §16.4）；`task_read` 无 id 兼容返回树（与 task_list 等价）。
