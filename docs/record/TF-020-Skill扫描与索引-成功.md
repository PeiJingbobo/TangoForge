# TF-020 Skill 扫描与索引 — 任务总结
> 结果：成功　|　日期：2026-08-06　|　执行人：ai

## 1. 任务范围
实现 AI Skill 扫描与索引：`{workdir}/.taskboard/skills/` 文件系统为唯一数据源，skills 表仅缓存同步；提供 skill 列表与 `skill_info` 查询；替换 TF-014 的 HTTP 占位端点。

## 2. 交付内容
- **新增文件**：
  - `internal/skill/skill.go` — Service：`List`（重扫 + 缓存 + 名称升序）、`Info`（不存在 → `ErrSkillNotFound`）、`scanAndSync`（扫描 → upsert + 清理失效行）、`parseSkill`（YAML name/version/description/instructions / Markdown 首个 `# ` 标题 + 全文）、`firstMarkdownTitle`；目录缺失视为空
  - `internal/skill/skill_test.go` — 8 用例（双格式解析、缺 name/坏 YAML/无标题失败、忽略 txt/子目录、删除文件索引同步、修改内容同步、解析失败跳过、Info 404、项目未导入 404、目录缺失空）
  - `internal/api/handlers_skills.go` — `GET /api/skills`（skill.read）、`GET /api/skills/:name`（skill.read，`SKILL_NOT_FOUND` 404）
  - `internal/api/handlers_skills_test.go` — 2 用例（UI 列表/详情/404、agent 默认 skill.read 放行）
- **修改文件**：
  - `internal/db/migrate.go` — 项目库迁移 **v2 `extend_skills_meta`**（skills 表加 version/description/instructions 列；Down 重建表）
  - `internal/db/migrate_test.go` — skills 列断言、幂等版本 1→2
  - `internal/api/server.go` — Server 挂载 skillSvc（NewServer/Close）
  - `internal/api/handlers_placeholder.go` / `handlers_ws_test.go` — 移除 skill 占位与 501 断言
  - `docs/TASK-SEMANTICS.md` — 新增 §15（Skill 语义）
  - `docs/task/TASKS.md` / `OVERVIEW.md` — 状态 ✅、统计同步
- **关键实现点**：
  1. 文件系统唯一数据源：重扫时逐行比对清理缓存失效行，绝不反写文件
  2. 扫描时机 = 启动 + 每次查询重扫（QA P4-1：无 fsnotify 常驻 watcher）
  3. 解析失败仅告警跳过，不阻断扫描
  4. 仅一级目录 .yaml/.yml/.md，子目录与非支持扩展名忽略

## 3. 验证结果
- `gofmt -l internal/skill internal/api internal/db` → 干净
- `CGO_ENABLED=0 go test ./internal/skill/... ./internal/db/... ./internal/api/...` → **ok**
- `CGO_ENABLED=0 go test ./...` → **全仓全绿**（无回归）

## 4. 遗留问题与后续
- `skill.changed` 事件未实现（无 watcher，登记 TASK-SEMANTICS §15.1）：文件变化不实时推送，前端在 TF-028 如需实时性可加手动刷新或后续扩展 fsnotify。
- MCP `skill_info` 工具（TF-017）将复用本 Service。
