# TF-017 MCP 工具全集 — 任务日志

> 日期：2026-08-06　|　执行人：ai　|　分支：`feat/TF-017-mcp-tools`

## 进展记录

### 2026-08-06（完成）
1. `internal/mcp/tools_*.go` 按域补齐 19 个工具：task（list/update/archive/restore）、project（list/import/init/create）、import（preview/confirm/discard）、export_markdown、graph_get、state_machine（get/update）、skill_info、permission_list。
2. `internal/task/graph.go`：`Service.Graph`（graph 数据组装从 api 层下沉，MCP/HTTP 复用）；`api/handlers_graph.go` 改为薄封装。
3. `internal/project/project.go`：`Init`（仅初始化不注册）/ `ImportExisting`（仅导入，要求已初始化）/ `Create`（先 init 后 import）——QA P4-1 Q6 三工具语义。
4. `cmd/cli/cmd_mcp.go` 与 `api.MCPHandler` 的 Deps 注入 Parser/Exporter。
5. 测试：mcp_tools_test.go 6 用例（全工具清单、生命周期全链路、未初始化 import 报错、skill_info、denied 路径）、task/graph_test.go 2 用例、project/project_init_test.go 3 用例。

## 决策记录
- **project 三工具**（QA P4-1 Q6）：project_import=ImportExisting（仅注册，要求 meta.db 存在）、project_init=Init（仅初始化）、project_create=Create（init+import 一条龙）；HTTP POST /api/projects/import 保持 Import（导入并初始化）不变，语义差异登记 §16.4。
- **project 域豁免权限**：与 HTTP /api/projects 组一致（无项目上下文、项目引导，QA P3-2）；其余工具一律 Require。
- **graph 下沉 task 包**：避免 api/mcp 重复组装（分层铁律：传输层薄封装）。
- **task_update parent_id 三重态**：空串=置顶（&nil 指针）、非空=改父（&pstr）。

## 踩坑记录
1. `in.ParentID = &sv` 类型错误（*string 赋 **string）→ 用 `var pstr *string; in.ParentID = &pstr`。
2. Perms.Set 全量覆盖语义：测试授权 task.create 后 task.read 被重置 → 先 Get 合并再 Set。
3. state_machine.read 默认 false（非只读 5 项）→ 测试需显式授权。
4. task 覆盖率 88.8%（新增 Graph 无包内测试）→ graph_test.go 补 2 用例 → 91.4%。
5. exporter.Service 无 Close 方法（无连接缓存）→ 测试 cleanup 移除。

## 建议提交命令（经 SSH）
```bash
cd ~/HD-DATA/Coding/TangoForge
git add -A
git commit -m "feat(mcp): TF-017 MCP 工具全集（19 工具 + project 三语义 + graph 下沉 + project Init/Create）"
```
