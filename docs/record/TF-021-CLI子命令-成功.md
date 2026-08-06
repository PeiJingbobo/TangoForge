# TF-021 CLI 子命令 — 任务总结
> 结果：成功　|　日期：2026-08-06　|　执行人：ai

## 1. 任务范围
CLI 全部子命令转为 HTTP 调用（多端等价）：projects/tasks/import/export/graph/state-machine/skills/permission/audit；`--project` 强制、`--actor`（默认 human）、`--server`；守护进程自动拉起（找不到提示手动启动）。

## 2. 交付内容
- **新增/修改文件**：
  - `cmd/cli/client.go` — HTTP 客户端（apiResp 兼容 code 数字 0/字符串错误码、错误映射）+ `ensureDaemon`（/ping → spawn 同目录 daemon → 轮询 ≤5s，找不到提示）+ `findDaemonBinary`（同目录 → PATH）+ `printOutput`（--json / 人类可读）
  - `cmd/cli/main.go` — 子命令分发 + `extractGlobal`（--server/--actor/--json 任意位置）
  - `cmd/cli/cmd_tasks.go` — tasks 全操作（list/get/create/update/status/archive/restore/delete）+ projects（list/import/remove）
  - `cmd/cli/cmd_import.go` — import（preview/drafts/confirm/discard）+ export（run/template <示例文件>）
  - `cmd/cli/cmd_other.go` — graph / state-machine（get/update <file.json>）/ skills / permission / audit
  - `scripts/cli_smoke.sh` — 真实冒烟脚本
  - `docs/TASK-SEMANTICS.md` — 新增 §19（CLI 语义）
  - `docs/task/TASKS.md` / `OVERVIEW.md` — 状态 ✅（P4 7/7 完成）
- **关键实现点**：
  1. 全部子命令薄封装转 HTTP（与 HTTP 同守护进程/同权限表/同审计）
  2. 自动拉起（N6）：spawn 同目录 daemon 二进制 + 轮询 /ping；找不到提示手动启动
  3. `--json` 原始输出 / 缺省人类可读；`--actor` 覆盖来源识别
  4. tasks status 走 task.update_status；parent_id 空串=置顶

## 3. 验证结果
- `go vet ./...` → 干净；`CGO_ENABLED=0 go test ./...` → **12 包全绿**
- `bash ./scripts/check_coverage.sh` → **91.4% ≥ 90%** 通过
- **真实 daemon CLI 冒烟（scripts/cli_smoke.sh，SMOKE PASS）**：
  1. projects import/list ✓
  2. UI 授权（PUT /api/permissions，等价 App 勾选）✓
  3. tasks create（priority high→4、tags）→ update（title）→ status（todo→doing）→ archive → restore 全链路 ✓
  4. export（default 模板，写盘 + content）✓
  5. graph（节点/边摘要）✓、state-machine get ✓、permission（✓/✗ 列表）✓
  6. 缺 --project 报错 ✓；daemon 未运行 → 找不到二进制时提示手动启动 ✓
  7. audit 未授权（audit.read=false）→ denied（预期安全默认）✓

## 4. 遗留问题与后续
- 自动拉起的「成功路径」（同目录存在 tangoforge-daemon 二进制）未在冒烟中覆盖（测试用 /tmp/tf-daemon 命名不同）；M4 人工验证时用正式构建（bin/tangoforge + bin/tangoforge-daemon 同目录）复现。
- `projects remove` 仅 UI 可执行（agent 身份 403），CLI 提供但标注。
- 真实 LLM 导入草稿流（import preview → confirm）未冒烟（LLM 未配置）；M4 验证时配置 DeepSeek 后执行。
- **P4 全部 7 个任务完成**（TF-015~021），M4 里程碑待用户按人工验证清单验收后关闭。
