# TF-034 Skills 前端重构 + 全局模板 — 任务总结

> 结果：成功　|　日期：2026-08-07　|　执行人：ai

## 1. 任务范围
Skills 页从「只读浏览」重构为「技能包生命周期管理界面」：安装向导（先选宿主 → 勾选包 → 批量安装）、
安装状态矩阵（missing/current/stale + 更新/卸载）、技能库（内置 + 自定义编辑）、
AGENTS.md 推荐提示词复制（中/英）；全局设置页新增「Skill 模板」tab。

## 2. QA 决策（用户确认，见 SKILLS-REDESIGN.md）
- 安装向导交互：先选宿主，再勾选要装的包，批量安装（Q3）
- 卸载需二次确认（Q5）
- 提示词仅 UI 复制，不自动写文件（Q4 相关）

## 3. 交付内容
**前端（React）**
- `app/src/types/models.ts`：Skill → `SkillPackage`（hosts/when_to_use/source/instructions/content）+
  `SkillInstallResult` / `InstalledSkill` / `HostStatus`；`ACTION_KEYS` 新增 `skill.install`（17 项）
- `app/src/hooks/useSkills.ts` 重写：useSkillPackages / useSkillStatus / useSkillInstall / useSkillUninstall /
  useSkillPackageWrite / useSkillTemplate(+Write)
- `app/src/features/skills/SkillsPanel.tsx` 重构：
  - 安装向导：宿主 Badge 单选（项目级/用户级标注）→ 技能包 Badge 多选（显示各宿主当前状态）→ 批量安装按钮
  - 安装状态矩阵表：宿主 × 技能包（状态徽章 未安装/已安装/可更新）+ 更新/卸载操作；卸载二次确认 Dialog
  - 技能库：内置/自定义包列表（展开详情：触发场景 + 适用宿主）+ 新建/编辑 SKILL.md（Dialog 编辑器）
  - AGENTS.md 推荐提示词卡片（中/英切换 + 一键复制）
- `app/src/features/settings/SettingsPage.tsx`：新增「Skill 模板」tab（模板编辑器 + 保存/放弃，实时回显）
- `app/src/features/permissions/permissions-skills.test.tsx` 重写：17 项权限断言 + SkillsPanel 4 例
**后端配套（TF-034 内补齐）**
- `internal/api/handlers_skill_template.go`：`GET/PUT /api/skill-template`（豁免 X-Project 全局组，
  PUT 仅 UI 二次校验 + 审计 `skill.template_written`）
- `internal/skill.WriteTemplate`：写入 `~/.taskboard-app/skills/_template/SKILL.md`（不做 frontmatter 强校验）

## 4. 验证结果
- 前端：147 用例全绿（+2：SkillsPanel 4 例、权限 17 项）；typecheck / lint / electron build 全绿
- Go：全仓全绿（+2：模板端点 Get/Put/Agent 403）
- daemon 实测：GET /api/skill-template 返回内置模板 200

## 5. 遗留问题与后续
- AGENTS.md 提示词为静态文案（UI 内置），未读后端；如需动态（含实际端口/项目列表）可后续改为调 guide 端点渲染
- Skills 页安装向导的宿主选项为前端硬编码（与后端 skill.Hosts 对齐）；两端一致需靠文档/测试维护
- P5.6 完成；P6（TF-029~031）待 M5 mac 人工验证后启动

## 6. 踩坑记录
- **getByText 多匹配**：技能包名同时出现在向导 Badge / 矩阵表头 / 技能库列表 → 测试用 getAllByText 断言数量
- **模板端点 X-Project 依赖**：初版模板端点放主业务组（强制 X-Project），但全局设置页无项目上下文 → 移出为豁免组
