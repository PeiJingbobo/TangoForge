# TF-033 Skills 后端重设计 — 任务总结

> 结果：成功　|　日期：2026-08-07　|　执行人：ai

## 1. 任务范围
Skill 功能从「扫描 `.taskboard/skills/` 只读浏览」重设计为「技能包生命周期管理」：
技能包来源（内置 embed + 全局技能库 + 默认模板）、宿主安装/卸载/状态检测、
AI 说明书端点（HTTP/MCP/CLI 免鉴权）。方案见 `docs/task/SKILLS-REDESIGN.md`（用户确认 QA-S1~S9）。

## 2. QA 决策（用户确认）
| 决策点 | 结论 |
|---|---|
| 宿主范围 | 多宿主 v1：AGENTS.md / CLAUDE.md / .cursor/rules / copilot / ~/.claude/skills / ~/.workbuddy/skills |
| 技能源定位 | **彻底废弃 `.taskboard/skills/`**；技能包 = 内置 embed + 全局技能库 `~/.taskboard-app/skills/` |
| 说明书鉴权 | **完全免鉴权**（GET /api/guide 任何来源可读） |
| 提示词交付 | 仅 UI 复制（不自动写文件） |
| 内置包 | v1 仅 `taskboard-basic`（导入导出由 App UI 负责） |
| skills 表 | 直接移除（迁移 v3 DROP） |
| 卸载确认 | 前端二次确认 |

## 3. 交付内容
**后端（Go）**
- `internal/skill` 重构：包模型（SKILL.md frontmatter 解析）+ 内置包 embed（`packages/taskboard-basic`）+
  全局技能库读写（同名覆盖内置）+ 全局默认模板（`_template/SKILL.md`，DefaultTemplate/WriteTemplate）
- `internal/skill/hosts.go`：宿主矩阵 6 个（marker 标记段 / 单文件 .mdc / 目录复制三类 Kind）；
  Install（幂等覆盖 update 语义 + hosts 适配校验）/ Uninstall / Status（missing/current/stale 实时扫描，无 watcher）
- `internal/guide`（新包）：AI 使用说明书单一来源（端点表/工具表/CLI 表/语义速查动态渲染）
- `internal/api`：`GET /api/guide`（免鉴权，注册在中间件链外）；`GET /api/skills/packages[/{name}]`
  （兼容旧端点）；`PUT /api/skills/packages/{name}`（仅 UI）；`GET /api/skills/status`；
  `POST /api/skills/install|uninstall`（skill.install）；`GET/PUT /api/skill-template`（豁免 X-Project 全局组，PUT 仅 UI）
- `internal/mcp`：`guide`（免鉴权无参数）+ `skill_install`/`skill_status`/`skill_uninstall` 工具；`skill_info` 语义迁移
- `cmd/cli`：`tangoforge guide` + `skills list|info|install|status|uninstall` 子命令
- 权限：`project.AllActions` 新增 `skill.install`（默认 false，Agent 需授权）
- 审计：`skill.installed/updated/uninstalled/package_written/template_written` 5 事件
- `internal/db`：迁移 v3 `drop_skills_table`（DROP TABLE IF EXISTS skills）；`project.Import` 不再创建 skills/ 目录
- 错误映射：`SKILL_NOT_FOUND`（404）/ `SKILL_INVALID`（422，含未知宿主/包不合法）

## 4. 验证结果
- Go 全仓全绿：skill 14 例（解析/内置/自定义覆盖/宿主矩阵/状态 stale/卸载幂等/sanitize）+ guide 3 例 +
  api 5 例（列表详情/自定义包/安装卸载状态/用户级宿主/模板）+ mcp 3 例（info 迁移/guide 免鉴权/install 权限拒绝）
- daemon 实测：guide 免鉴权 200；packages/status/install/uninstall 全链路（标记段写入、current 状态、卸载删文件）
- gofmt 干净；`CGO_ENABLED=0 go build` 通过

## 5. 遗留问题与后续
- 内置包仅 taskboard-basic；后续可扩展（如 import-export 包，需先有 App UI 对应流程）
- Skill 安装到用户级宿主（~/.claude/skills 等）依赖 daemon 进程用户主目录（mac 实测项）
- P5.6 完成后回到 P6（TF-029~031 待 M5 mac 验证后启动）

## 6. 踩坑记录
- **marker 标记段 `%s` 未格式化**：模板字符串直接拼接导致 `tangoforge:skill:%s:begin` 原样落盘，
  状态匹配失败——需 `fmt.Sprintf(markerBeginTmpl, pkg.Name)`。
- **upsert 正则误匹配多包**：begin/end 正则未按包名限定，第二个包安装会替换掉第一个包的整段——
  必须按 `QuoteMeta(name)` 精确匹配。
- **原始字符串反引号截断**：guide.go 用反引号字符串含 `X-Project` 等反引号导致语法错误——改用 WriteString 拼接。
