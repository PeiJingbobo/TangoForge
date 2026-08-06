# TF-018 Markdown 导入与草稿流 — 任务日志

> 日期：2026-08-06　|　执行人：ai　|　分支：`feat/TF-018-parser`

## 进展记录

### 2026-08-06（完成）
1. `internal/task/import.go`：`ImportTasks` 事务接口（文件级全量覆盖：归档旧 source_file + 批量 INSERT；事务外校验 title/status/依赖存在性/本批环；WAL 铁律：事务内首语句即写；不触发 task.* 钩子）。
2. `internal/parser/schema.go`：ParseResult/ParsedTask 结构、prompt 构造（系统角色 + 状态机注入 + JSON Schema）、status 映射（key/label）、标题依赖解析、flattenTasks（section 路径 + parent_id）。
3. `internal/parser/parser.go`：Service（Parse→parseCore 拆分以支持失败事件、List、Confirm、Discard、normalizeOutput、countTasks）；事件 draft_ready/confirmed/discarded/failed。
4. `internal/api/handlers_imports.go`：替换占位；api/errors.go 映射 IMPORT_FAILED/DRAFT_NOT_FOUND/LLM_*。
5. 测试：task/import_test.go 8 用例 + parser/parser_test.go 15 用例 + api/handlers_imports_test.go 6 用例。

## 决策记录
- **Parse 事件拆分**（QA P4-1）：Parse 外层统一 emit——成功 draft_ready、失败 failed（含 LLM 未配置）；parseCore 不含事件。
- **TaskCount 语义**：展平后全部任务数（含嵌套子任务），countTasks 递归统计。
- **depends_on 标题引用**（§17.3）：LLM 输出被依赖任务标题，确认时映射 UUID；不唯一/不存在 → 整次失败。
- **ImportTasks 不触发 task.\* 钩子**：批量导入避免事件风暴（QA Q9）；import.draft_confirmed 事件 + 审计一条由 parser 层发。
- **环检测范围**：仅本批内部依赖图（库内旧任务不可能反向依赖新任务，指向旧任务的引用不参与环检测）。
- **LLM provider 每次调用取最新配置**（`func() config.LLMConfig`）：支持 LLM 配置热重载即时生效。

## 踩坑记录
1. TestParse_DraftCreated TaskCount=2（仅统计顶层）→ countTasks 递归。
2. TestParse_Failures 事件为空 → Parse 失败路径未 emit failed → 拆分 parseCore。
3. api 测试断言 data 为数组 → 实际 ListResult 对象（tree/total）→ 修正断言。
4. TestPlaceholders_NOT_IMPLEMENTED 仍测 import 501 → 改为只测 export 占位。
5. `go: command not found`（check_coverage.sh）→ PATH 加 `$HOME/go-sdk/go/bin`。

## 建议提交命令（经 SSH）
```bash
cd ~/HD-DATA/Coding/TangoForge
git add -A
git commit -m "feat(parser): TF-018 Markdown 导入草稿流（LLM 解析 + 文件级覆盖确认 + task.ImportTasks）"
```
