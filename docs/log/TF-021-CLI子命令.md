# TF-021 CLI 子命令 — 任务日志

> 日期：2026-08-06　|　执行人：ai　|　分支：`feat/TF-021-cli`

## 进展记录

### 2026-08-06（完成）
1. `cmd/cli/client.go`：HTTP 客户端（apiResp 兼容 code 数字 0/字符串错误码、X-Actor/X-Project 头、错误映射）+ ensureDaemon（/ping → spawn 同目录 tangoforge-daemon → 轮询 5s，找不到提示）+ findDaemonBinary + printOutput（--json/人类可读）。
2. `cmd/cli/main.go`：子命令分发（projects/tasks/import/export/graph/state-machine/skills/permission/audit + mcp）+ extractGlobal（--server/--actor/--json 任意位置）。
3. `cmd/cli/cmd_tasks.go`（tasks 全操作 + projects）+ `cmd_import.go`（preview/drafts/confirm/discard + export/template）+ `cmd_other.go`（graph/state-machine/skills/permission/audit）。
4. `scripts/cli_smoke.sh`：真实冒烟脚本（起 daemon → import → UI 授权 → tasks 生命周期 → export → graph → 状态机 → permission → 缺 project 报错 → 自动拉起提示），SMOKE PASS。

## 决策记录
- **apiResp.code 兼容**：成功 `code:0`（number）与错误码（string）共存 → Code 用 json.RawMessage + ok() 判断。
- **自动拉起**（QA P4-1 Q15-A）：CLI 与 daemon 为两个二进制（Makefile build-cli/build-daemon 分开）→ 查找同目录 `tangoforge-daemon[.exe]` → spawn；找不到提示手动启动（验收允许"可先提示"）。
- **tasks create title 为位置参数**（create <title>），也支持 --title。
- **task_update parent_id 空串 → 置顶**（body parent_id: null）。
- **tasks status 走 PATCH {status}**（task.update_status 动态权限）。

## 踩坑记录
1. `code:0` 无法 unmarshal 到 string 字段 → apiResp.Code 改 json.RawMessage。
2. 冒烟首跑：注册表有旧记录（目录被删后 import 幂等返回旧记录、meta.db 缺失 → 权限 PROJECT_NOT_FOUND）→ 冒烟用 mktemp 唯一目录。
3. 旧 daemon（TF-016 编译）残留运行导致行为不一致 → pkill 后重启新编译 daemon。
4. CLI 默认 agent 身份：task.create 等默认 denied（正确行为）→ 冒烟脚本先经 UI 凭据 PUT /api/permissions 授权（等价 App 勾选）。
5. 冒烟脚本中 tasks create 的 title 位置参数未处理 → 修正实现。

## 建议提交命令（经 SSH）
```bash
cd ~/HD-DATA/Coding/TangoForge
git add -A
git commit -m "feat(cli): TF-021 CLI 子命令（HTTP 等价操作 + 自动拉起 + 冒烟脚本）"
```
