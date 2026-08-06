# TF-020 Skill 扫描与索引 — 任务日志

> 日期：2026-08-06　|　执行人：ai　|　分支：`feat/TF-020-skill`

## 进展记录

### 2026-08-06（完成）
1. `internal/skill/skill.go`：Service（List / Info / Close）+ scanAndSync（扫描一级目录 → 缓存 upsert + 清理失效行）+ parseSkill（YAML / Markdown 双格式）+ firstMarkdownTitle。
2. `internal/db/migrate.go`：项目库迁移 v2 `extend_skills_meta`（ALTER TABLE skills 加 version/description/instructions；Down 重建表回退）；`migrate_test.go` 同步（列断言 + 幂等版本 2）。
3. `internal/api`：Server 挂载 skillSvc；新建 handlers_skills.go（GET /api/skills、GET /api/skills/:name → SKILL_NOT_FOUND 404）；从 handlers_placeholder.go 移除 skill 占位；handlers_ws_test.go 的 501 断言移除。
4. `internal/skill/skill_test.go` 8 用例 + `internal/api/handlers_skills_test.go` 2 用例。

## 决策记录
- **扫描时机**（QA P4-1）：启动 + 每次 List/Info 查询时重扫（轻量），无 fsnotify；`skill.changed` 事件不推送（登记 TASK-SEMANTICS §15.1）。
- **缓存列扩展走迁移 v2**：skills 原表仅 name/content/updated_at 三列，YAML 结构化字段需落缓存 → 新增迁移（v1 已发布不可改）。
- **子目录不递归**：仅一级目录 .yaml/.yml/.md（QA P4-1 推荐）。
- **Markdown 标题规则**：首个 `# ` 标题为 name，全文为 instructions/content；无标题告警跳过。

## 踩坑记录
1. skills 表无 version 列 → 首跑 SQL error；补迁移 v2 + migrate_test 同步断言。
2. 排序断言写反：UTF-8 中文「导」字节序 > ASCII「t」，`taskboard-basic` 在前（按 name ASC 正确）；修正测试期望。
3. `handlers_ws_test.go` 既有「/api/skills 501」断言，skill 落地后需移除，否则回归红。

## 建议提交命令（经 SSH）
```bash
cd ~/HD-DATA/Coding/TangoForge
git add -A
git commit -m "feat(skill): TF-020 Skill 扫描与索引（双格式 + 缓存同步 + HTTP 端点）"
```
