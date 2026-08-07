# Skills 功能重新设计 — QA 细化与整改方案

> 状态：**方案已定稿（2026-08-07 用户确认全部 QA）**　|　日期：2026-08-07　|　提出：用户　|　整理：ai
> 关联：TF-020（现状）、TF-028（现状 UI）；已立项 **TF-033（后端）/ TF-034（前端）**
> **确认记录**：Q1 内置包仅 `taskboard-basic`（导入导出由 App UI 负责）｜Q2 移除 `skills` 表（v3 迁移 DROP）｜Q3 安装向导=先选宿主再勾选包批量安装｜Q4 guide 仅 markdown｜Q5 卸载需二次确认

---

## 1. 现状与偏差分析

### 1.1 现状（TF-020 / TF-028 已实现）

| 层 | 现状 |
|---|---|
| 后端 | `internal/skill` 扫描 `{workdir}/.taskboard/skills/`（**一级目录，yaml/yml/md**）→ 缓存 `skills` 表 → `GET /api/skills`、`GET /api/skills/:name`（`skill.read` 权限） |
| MCP | `skill_info` 工具（`skill.read`） |
| CLI | `tangoforge skills list|info`（转 HTTP） |
| 前端 | SkillsPage/SkillsPanel：**只读浏览**——列表 + instructions 详情 |

### 1.2 目标（用户原设计意图）与偏差

| # | 目标 | 现状 | 偏差 |
|---|---|---|---|
| G1 | 从 App UI **下载 skill 技能包** | 无下载/获取能力 | ❌ 缺失 |
| G2 | 为**指定 Agent 工具**在当前项目**一键安装 Skill** | 无安装概念 | ❌ 缺失 |
| G3 | **检查技能安装状态** | 无状态概念 | ❌ 缺失 |
| G4 | 系统提供**默认 Skill 模板**（全局设置可修改） | 无模板 | ❌ 缺失 |
| G5 | 用户**自定义编辑**当前项目的 Skill | 只能手工放文件到 `.taskboard/skills/` | ❌ 偏差 |
| G6 | Skill =「给 Agent 看的说明书 + 可选工具包」，**安装到宿主能发现的位置** | 管理的是 `.taskboard/skills/` 文件本身 | ❌ 根本偏差 |
| G7 | 提供**放入 AGENTS.md 的推荐提示词**（中/英文，一键复制） | 无 | ❌ 缺失 |
| G8 | 为 AI 准备**说明书端点**（HTTP / MCP / CLI），**免鉴权**，无 Skill 也能掌握系统用法 | 无 | ❌ 缺失 |

**结论**：现有功能只是"文件浏览"，与目标形态（Skill 生命周期管理 + Agent 引导基础设施）差距大，需重新设计。

---

## 2. QA 决策记录（已确认 + 方案建议）

### QA-S1 宿主支持矩阵（✅ 用户已确认：多宿主 v1）

| 宿主 | 项目级安装位置 | 用户级安装位置 | 安装形态 |
|---|---|---|---|
| CodeBuddy / 通用 | `AGENTS.md`（项目根） | `~/.workbuddy/skills/` | 追加**标记段**（可识别/可撤销） |
| Claude Code | `CLAUDE.md`（项目根） | `~/.claude/skills/<name>/SKILL.md` | SKILL.md 目录复制 |
| Cursor | `.cursor/rules/*.mdc`（每包一文件） | `~/.cursor/rules/` | 单文件 `.mdc` |
| GitHub Copilot | `.github/copilot-instructions.md` | — | 追加标记段 |

> **单文件宿主（AGENTS.md / CLAUDE.md / copilot-instructions.md）多包共存**：用
> `<!-- tangoforge:skill:<name>:begin --> … :end -->` HTML 注释包裹，可追加多包、可按包卸载。

### QA-S2 技能源定位（✅ 用户已确认：彻底废弃 `.taskboard/skills/`）

- **删除** `{workdir}/.taskboard/skills/` 目录及其扫描/缓存机制。
- **移除 `skills` 表**：新增迁移 v3 `drop_skills_table`（`DROP TABLE IF EXISTS skills`），不再读写。
- 技能包来源改为：
  - **内置包**：随 daemon 内嵌（`internal/skill/packages/` embed，编译进二进制，零外部依赖满足 N1）。v1 仅 `taskboard-basic` 一个（Q1：导入导出功能由 App UI 负责，不内置对应包）。
  - **全局技能库**：`~/.taskboard-app/skills/<name>/`——用户自定义 / 下载落点（跨项目共享）。
  - **全局默认模板**：`~/.taskboard-app/skills/_template/SKILL.md`（全局设置页可编辑，见 QA-S4）。

### QA-S3 说明书端点鉴权（✅ 用户已确认：完全免鉴权）

- 端点不经过 auth 中间件（或中间件显式放行），**任何来源（含局域网）可读**。
- 范围：`GET /api/guide`、MCP `guide` 工具、CLI `tangoforge guide`。
- 安全说明：说明书为**只读能力描述**（端点清单/工具用法/语义），不含项目数据与凭据，风险可控；正式版如介意可加开关（`guide_public: true/false`，默认 true）。

### QA-S4 默认 Skill 模板（全局设置可修改）

- 模板文件：`~/.taskboard-app/skills/_template/SKILL.md`（含 frontmatter + 正文骨架，占位符 `{{...}}`）。
- 修改入口：全局设置页新增「Skill 模板」tab（编辑器 + 保存到上述路径）。
- 内置包从模板生成时替换占位符（name/description/version）。

### QA-S5 Skill 包格式（v2，向 Anthropic Agent Skills 规范靠拢）

```
<name>/                    # 包目录（全局库 / 内置 embed）
├── SKILL.md               # 必须：frontmatter + 正文（给 Agent 看的说明书）
├── resources/             # 可选：只读参考（如 API 手册、示例）
└── scripts/               # 可选：可执行工具（Agent 可调脚本）
```

`SKILL.md` frontmatter（YAML）：

```yaml
---
name: taskboard-basic            # 唯一标识
description: 用 TangoForge 管理项目任务
version: "1.0.0"                 # 安装状态比对依据
hosts: [AGENTS.md, CLAUDE.md, .cursor/rules, copilot]  # 适用宿主
when_to_use: 需要创建/更新/查询/流转项目任务时激活
---
# 正文：场景 → 调用方式（HTTP/MCP/CLI 三端示例）→ 字段语义
```

### QA-S6 安装 / 状态 / 卸载语义

| 操作 | 行为 |
|---|---|
| **获取（download）** | 内置包 → 复制到全局技能库（`~/.taskboard-app/skills/<name>/`），幂等（已存在则提示已获取） |
| **安装（install）** | 从技能库复制到指定宿主位置（项目级或用户级）；单文件宿主追加标记段；目录宿主建 `<name>/` 或 `.mdc` |
| **状态（status）** | 扫描各宿主位置：`missing`（未安装）/ `current`（已装且版本一致）/ `stale`（库有新版） |
| **更新（update）** | `stale` 时重新复制/替换宿主文件（先卸后装或整段替换） |
| **卸载（uninstall）** | 移除宿主标记段 / 删除安装目录文件 |

### QA-S7 权限与审计

- 读（包列表 / 状态 / 说明书）：`skill.read`（默认 true，**说明书端点免鉴权不受此限**）。
- 写（获取 / 安装 / 更新 / 卸载 / 编辑自定义包）：新增 **`skill.install`** 权限（默认 false，UI 放行；Agent 需授权）。
- 审计：`skill.installed / skill.uninstalled / skill.updated / skill.package_written`（actor=ui，写宿主位置与技能库均记录）。

### QA-S8 `skills` 表（✅ 用户已确认：直接移除）

- 新增项目库迁移 **v3 `drop_skills_table`**：`DROP TABLE IF EXISTS skills`。
- 后端不再读写 skills 表；`internal/skill` 完全脱离项目库依赖。
- 安装状态实时扫描宿主位置得出（无状态、无 watcher、天然准确，符合 P4-1 精神）。

---

## 3. 目标架构（Skill 生命周期）

```
                ┌────────────────────────────────────────────────┐
                │                 App UI（Skills 页）             │
                │  ① 技能库浏览/获取  ② 宿主×包 安装矩阵          │
                │  ③ 状态检查(缺失/最新/过期) ④ 提示词复制         │
                └───────┬────────────────────┬───────────────────┘
                        │ HTTP(skill.install) │ HTTP(skill.read / 免鉴权)
                ┌───────▼──────────┐  ┌──────▼──────────────────┐
                │   internal/skill  │  │   internal/guide        │
                │   包模型重构      │  │   说明书生成（单一来源） │
                │   · 内置包 embed  │  │   · HTTP GET /api/guide │
                │   · 全局技能库    │  │   · MCP guide 工具      │
                │   · 宿主矩阵      │  │   · CLI guide 子命令    │
                │   · 安装/卸载/状态│  └───────────┬──────────────┘
                └───────┬──────────┘              │
                        │ 复制文件                │ 免鉴权（回环+局域网）
        ┌───────────────▼──────────────────────────▼───────────────┐
        │  宿主位置（各类 Agent 约定）                              │
        │  AGENTS.md / CLAUDE.md / .cursor/rules/ /                 │
        │  .github/copilot-instructions.md / ~/.claude/skills/      │
        └───────────────────────────────────────────────────────────┘
                        │ 会话启动/运行中发现
        ┌───────────────▼───────────────────────────┐
        │  AI Agent：命中 Skill → 读 SKILL.md →       │
        │  按流程调 cli / mcp / http / scripts       │
        └─────────────────────────────────────────────┘
```

**Skill 工作步骤（用户需求原文落地）**：
1. 系统把 Skill 文件下载/复制到工作目录下各类 Agent 约定的位置 → **建立可发现性**；
2. Agent 在会话启动或运行过程中**发现**这些 Skill；
3. Agent 工作命中某 Skill → **读取 SKILL.md** → 按其中流程读资源 / 调脚本 / 调本系统工具。

---

## 4. 说明书端点设计（核心新增）

### 4.1 内容（单一来源 `internal/guide`）

```
# TangoForge 使用指南（AI 版）
1. 系统简介：本地优先人机协作任务中间件；项目=工作目录(.taskboard/)；多端等价
2. 核心概念：任务字段、状态机（默认 todo→doing→done→archived，可配置）、
   依赖(depends_on 无环)、权限模型（5 级来源识别）
3. HTTP API（推荐）：Base http://127.0.0.1:19810，X-Project 头强制
   （端点清单从注册表动态生成：方法/路径/权限/说明/示例）
4. MCP：`tangoforge mcp`（stdio），19 工具清单（从工具表动态生成）
5. CLI：`tangoforge <cmd> --project P`（子命令清单动态生成）
6. 业务语义速查：错误码→HTTP 映射、归档=删除、导入草稿流
```

### 4.2 三端形态

| 端 | 入口 | 鉴权 | 返回 |
|---|---|---|---|
| HTTP | `GET /api/guide` | 免鉴权 | `text/markdown`（Q4：仅 markdown，不做 JSON 格式） |
| MCP | `guide` 工具（无参数） | 免鉴权 | markdown 文本 |
| CLI | `tangoforge guide` | 免鉴权 | 打印到 stdout |

> **关键价值**：AI 没有安装任何 Skill 时，AGENTS.md 提示词（QA-S9）告知它
> "先 `GET http://127.0.0.1:19810/api/guide` 读说明书"，即可掌握全部调用方式。

---

## 5. AGENTS.md 推荐提示词（QA-S9，✅ 用户已确认：仅 UI 复制）

**交付形态**：Skills 页顶部「复制提示词」卡片，中/英切换 + 一键复制，**不自动写文件**。

中文版（草案）：

```markdown
> TangoForge 是本项目使用的任务管理中间件（本地守护进程，端口 19810）。
> 当你需要管理本项目任务（创建/查询/更新/流转状态/导入导出 Markdown）时，
> 应通过 TangoForge 完成，而不是自己臆造数据。
> 用法：先读说明书 `curl http://127.0.0.1:19810/api/guide`（或 MCP guide 工具），
> 按其中 HTTP/MCP/CLI 方式调用；所有请求必须带 X-Project 项目目录头。
> 项目下已安装的 Skill 位于 AGENTS.md / CLAUDE.md / .cursor/rules 等宿主位置，
> 命中任务场景时优先读取对应 SKILL.md 按其流程执行。
```

英文版对应翻译。UI 提供复制按钮（navigator.clipboard + 桌面模式 IPC 兜底）。

---

## 6. 整改任务拆分（确认后立项）

### TF-033 后端（skill 包模型 + guide 端点）

| 项 | 内容 |
|---|---|
| 1 | `internal/skill` 重构：删除 `.taskboard/skills/` 扫描与 skills 表依赖；新增包模型（SKILL.md frontmatter 解析）、内置包 embed（**仅 taskboard-basic**）、全局技能库读写 |
| 2 | 宿主矩阵：`hosts` 定义（4 项目级 + 2 用户级）、安装/卸载/更新/状态检测（标记段 or 目录） |
| 3 | `internal/guide`：说明书生成（端点表 + 工具表 + 语义速查动态渲染，单一来源） |
| 4 | 端点：`GET /api/guide`（免鉴权）、`GET /api/skills/packages`（skill.read）、`GET /api/skills/status`（skill.read）、`POST /api/skills/install`（skill.install）、`POST /api/skills/uninstall`、`PUT /api/skills/packages/{name}`（自定义包读写，UI） |
| 5 | MCP：`guide` 工具 + `skill_install` / `skill_status`（skill.install / skill.read） |
| 6 | CLI：`tangoforge guide` + `skills install|status|uninstall` 子命令 |
| 7 | 权限：新增 `skill.install`（默认 false）+ 审计 4 事件 |
| 8 | 全局配置：`guide_public`（默认 true）+ 模板路径；SettingsPage 对应后端 |
| 9 | 测试：包解析 / 宿主矩阵 / 安装卸载状态 / guide 渲染 / 权限审计（Go 全绿） |

### TF-034 前端（Skills 页重构 + 全局模板 tab）

| 项 | 内容 |
|---|---|
| 1 | SkillsPage 重构：**安装向导**（**先选宿主 → 再勾选要装的包 → 批量安装**，Q3）；**安装矩阵表**（宿主×包，状态徽章 missing/current/stale + 安装/更新/卸载，**卸载需二次确认** Q5）；**技能库**（内置+自定义，自定义包编辑入口） |
| 2 | 「复制 AGENTS.md 提示词」卡片（中/英 + 一键复制） |
| 3 | 全局设置页新增「Skill 模板」tab（模板编辑器 + 保存） |
| 4 | hooks：useSkillPackages / useSkillStatus / useSkillInstall / useGuide；WS 无新事件（状态实时扫描） |
| 5 | 测试：矩阵渲染 / 安装卸载交互 / 提示词复制 / 模板编辑（Vitest 全绿） |

### 文档同步

- `docs/REQUIREMENTS.md` §8 重写（Skill 生命周期 + guide 说明书）
- `docs/TASK-SEMANTICS.md` §15 重写（新语义 + 权限 + 审计）
- `docs/AGENTS.md`（根 + docs 副本）同步 §5/§8
- `docs/REQUIREMENTS-REVIEW.md` 追加 QA-S1~S9 记录

---

## 7. 待确认点（✅ 2026-08-07 已全部确认）

1. **内置包数量**：v1 仅 `taskboard-basic`；导入导出功能由 App UI 负责，不内置对应包 ✅
2. **`skills` 表**：直接移除（v3 迁移 DROP TABLE IF EXISTS skills）✅
3. **安装向导交互**：先选宿主，再勾选要装的包，批量安装 ✅
4. **guide 结构化输出**：仅 markdown，不做 JSON ✅
5. **卸载确认**：需要二次确认 ✅
