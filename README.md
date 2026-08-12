# TangoForge

> 人机协作的任务管理中间件 —— 本地守护进程 + 桌面客户端，为 **AI Agent** 提供 MCP / HTTP / CLI 三通道接入。

TangoForge 把「任务池 · 状态机 · 导入导出 · Agent 协作」打包成一个**纯本地**服务：人类在桌面客户端（看板 / 导航 / 全景图）操作任务，AI Agent 通过 MCP / HTTP / CLI 以**受控权限**读写同一份数据——谁在什么时候改了什么，全部落在不可篡改的审计日志里。

[![Backend CI](https://github.com/PeiJingbobo/TangoForge/actions/workflows/backend-ci.yml/badge.svg)](https://github.com/PeiJingbobo/TangoForge/actions/workflows/backend-ci.yml)
[![Frontend CI](https://github.com/PeiJingbobo/TangoForge/actions/workflows/frontend-ci.yml/badge.svg)](https://github.com/PeiJingbobo/TangoForge/actions/workflows/frontend-ci.yml)
[![Release](https://github.com/PeiJingbobo/TangoForge/actions/workflows/release.yml/badge.svg)](https://github.com/PeiJingbobo/TangoForge/actions/workflows/release.yml)
[![GitHub Release](https://img.shields.io/github/v/release/PeiJingbobo/TangoForge?color=blue)](https://github.com/PeiJingbobo/TangoForge/releases)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL%203.0-blue.svg)](LICENSE)

![TangoForge 看板界面](https://github.com/PeiJingbobo/TangoForge/raw/main/app/src/assets/screenshots/kanban.png)
<!-- TODO: 补充看板页面截图，保存到 app/src/assets/screenshots/kanban.png 后替换上方 URL 即生效 -->

---

## 目录

- [下载与更新](#下载与更新)
- [核心特性](#核心特性)
- [架构总览](#架构总览)
- [快速开始](#快速开始)
- [AI Agent 接入（详细教程）](#ai-agent-接入详细教程)
- [CLI 全局注册](#cli-全局注册)
- [开发者指南](#开发者指南)
- [目录结构](#目录结构)
- [项目文档](#项目文档)
- [许可证](#许可证)

---

## 下载与更新

安装包由 **GitHub Actions 自动构建**并发布到 [GitHub Releases](https://github.com/PeiJingbobo/TangoForge/releases)（推送 `vX.Y.Z` 标签触发，版本与 `app/package.json` 强一致）。

| 平台 | 产物 | 安装方式 |
|------|------|----------|
| **macOS**（Apple Silicon, arm64） | `TangoForge-<版本>.dmg` | 双击 dmg → 将 TangoForge 拖入 Applications |
| **Windows**（x64） | `TangoForge-<版本>-setup.exe` | 双击运行 NSIS 安装向导；另有 `TangoForge-<版本>-win-x64.exe` 便携版 |

App 内置**在线更新**：启动后自动检查（或「设置 → 关于」手动检查）。

- **Windows**：检测到新版本 → 提示下载 → 「重启并安装」自动完成。
- **macOS**：未签名阶段无法自动安装，检测到新版本后**自动打开 dmg 下载页**，下载后手动安装。

> ⚠️ **未签名阶段说明**（当前版本）：
> - macOS 暂无 Apple Developer ID 证书，产物**未代码签名**，首次打开需手动允许（见下方指令）。
> - Windows 使用 CI 内生成的自签名证书，SmartScreen 会提示「未知发布者」，点 **「更多信息 → 仍要运行」** 即可。

### macOS：允许未签名 App 运行

首次打开会提示「无法验证开发者」或「已损坏，无法打开」，任选其一：

```bash
# 方式一（推荐）：移除隔离标记后正常双击打开
xattr -dr com.apple.quarantine "/Applications/TangoForge.app"

# 方式二：终端直接启动（绕过 Gatekeeper 校验）
open "/Applications/TangoForge.app"
```

或在「访达」中**右键** `TangoForge.app` → **「打开」** → 弹窗中再次点「打开」（仅首次提示一次）。

---

## 核心特性

- **本地优先 · 数据自持**：SQLite（WAL）+ 本地守护进程，数据全部落在你指定的工作目录（`.taskboard/`），无云端、无账号、无订阅。
- **任务管理**：看板拖拽流转、树形导航、状态机动态列、依赖关系（无环校验）、归档/还原回收站、Markdown 导入导出（草稿审阅后确认）。
- **Agent 三通道**：MCP（stdio / HTTP）、REST API、CLI —— 同一业务层实现，行为完全一致，多端等价。
- **受控权限**：UI 全权；Agent 默认**只读**，写操作需在客户端按项目显式授权（细粒度 action 开关）。
- **Skills 技能包**：为 Claude Code / Cursor / Copilot / WorkBuddy 等宿主一键安装可发现的技能包（SKILL.md）。
- **审计日志**：全部写操作异步落库（actor / action / target / result），只追加不可篡改，可导出。
- **健康桌面客户端**：Electron + React 19 + shadcn-ui，温和协作风视觉，深色/浅色/跟随系统。

## 架构总览

```
┌─────────────────────────────┐        ┌──────────────────────────────┐
│  桌面客户端（Electron+React） │        │  AI Agent（Claude/Cursor/...） │
│  看板/导航/全景图/设置/权限    │        │  MCP · HTTP · CLI             │
└──────────────┬──────────────┘        └──────────────┬───────────────┘
               │  HTTP (回环 + X-UI-Token)             │  识别层：Bearer / X-Actor
               ▼                                      ▼
        ┌───────────────────────────────────────────────────┐
        │             TangoForge 守护进程（Go）               │
        │  /api 中间件链：识别 → 项目校验 → 权限 → 审计 → WS    │
        ├──────────────────────┬────────────────────────────┤
        │ 全局注册表 registry.db │ 项目库 {workdir}/.taskboard/  │
        │ (projects 表)         │ (tasks/permissions/audit…)   │
        └──────────────────────┴────────────────────────────┘
```

- **单守护进程 · 多项目**：一个常驻进程管理多个项目，项目以**工作目录路径**唯一标识。
- **多端等价铁律**：所有跨端调用必须显式携带项目目录（HTTP `X-Project` 头 / CLI `--project` / MCP `project` 参数），GUI 能做的，Agent 一样能做。

---

## 快速开始

### 1. 安装

从 [GitHub Releases](https://github.com/PeiJingbobo/TangoForge/releases) 下载对应平台安装包安装（macOS 首次打开见上方允许运行指令）。

### 2. 首次启动

启动 TangoForge，进入**项目导入引导**：

1. 选择工作目录（任务库落在这里）；
2. 配置 LLM 接入（Markdown 导入解析用，可跳过）；
3. 导入草稿审阅 → 确认入库；
4. 配置 Agent 权限（默认只读）；
5. 安装 Skill 技能包（可选）→ 进入项目。

### 3. 基本使用

- **看板**：拖拽卡片跨列流转（需满足状态机流转规则），父子任务层级缩进。
- **导航**：树形任务列表，按状态/标签/关键词过滤。
- **全景图**：依赖关系可视化（`parent` / `depends_on` 边）。
- **导入/导出**：Markdown ↔ 任务库，导入走草稿审阅流。

---

## AI Agent 接入（详细教程）

> **权限模型**：Agent 默认**只读**。所有写操作（创建/流转/导入导出/权限/状态机修改）需在客户端「Agent 权限」页按项目显式授权；未授权返回 `PERMISSION_DENIED`。

### 方式一：MCP（推荐给 Claude Code / Cursor / 支持 MCP 的宿主）

```bash
# stdio 模式：MCP Server 由 CLI 启动，项目通过参数显式指定
tangoforge mcp --project /path/to/project
```

在宿主中配置 MCP Server（以 Claude Code 为例，`.mcp.json`）：

```json
{
  "mcpServers": {
    "tangoforge": {
      "command": "tangoforge",
      "args": ["mcp", "--project", "/path/to/project"]
    }
  }
}
```

MCP 工具集（v1 固定，均带 `project` 必填参数）：任务（read / list / create / update / archive / restore）、项目（list / import / init / create）、导入导出（preview / confirm / discard / export）、状态机（get / update）、Skill（info / install / status / uninstall）、权限（list）、全景图（graph）、指南（guide，免鉴权）。

### 方式二：CLI（等价 HTTP）

```bash
# 查看任务树
tangoforge tasks list --project /path/to/project

# 创建任务
tangoforge tasks create --project /path/to/project --title "接入 Agent" --status todo

# 导入 Markdown 草稿
tangoforge import preview --project /path/to/project --file roadmap.md
tangoforge import confirm --project /path/to/project --draft <id>

# 查看系统使用说明书
tangoforge guide
```

完整命令面：`tangoforge --help`（version / mcp / projects / tasks / import / export / graph / state-machine / skills / permission / audit / guide）。

### 方式三：HTTP API（守护进程 127.0.0.1:19810）

```bash
# 项目标识通过 X-Project 头显式传递
curl -H "X-Project: /path/to/project" http://127.0.0.1:19810/api/guide

# 读取任务
curl -H "X-Project: /path/to/project" http://127.0.0.1:19810/api/tasks
```

> 远程访问需在全局配置开启并配置 API Token（`Authorization: Bearer <token>`）；默认仅回环可访问。

### 常用场景示例

| 场景 | 命令 / 调用 |
|------|-------------|
| 让 Agent 汇报本周进展 | `tangoforge tasks list --project <dir>` → 按状态统计 |
| 让 Agent 创建新需求 | 创建任务（需 `task.create` 授权） |
| 让 Agent 批量导入需求文档 | `import preview` → 人工确认 → `import confirm` |
| 让 Agent 维护任务板 | 状态流转 / 归档 / 依赖调整（需对应 action 授权） |

---

## CLI 全局注册

打包产物 `resources/bin/` 内含 **CLI（tangoforge）** 与 **守护进程（tangoforge-daemon）**（App 启动自动拉起 daemon；CLI 经 HTTP 与 daemon 等价操作）。

```bash
# macOS / Linux：创建 ~/bin/tangoforge 链接并注入 shell PATH（幂等）
"/Applications/TangoForge.app/Contents/Resources/bin/register-cli.sh"
```

```powershell
# Windows：追加到当前用户 PATH（幂等）
powershell -ExecutionPolicy Bypass -File "C:\Program Files\TangoForge\resources\bin\register-cli.ps1"
```

注册后新开终端即可直接 `tangoforge`；未注册时使用完整路径调用（见 `tangoforge --help`）。

---

## 开发者指南

### 技术栈

| 层 | 技术 |
|---|---|
| 守护进程 | Go（chi router、SQLite WAL、异步审计、WebSocket 事件广播） |
| 桌面客户端 | Electron 43 + React 19 + TypeScript 严格模式 + shadcn-ui（Radix + Tailwind CSS 4） |
| 构建 / 发布 | electron-vite + electron-builder；GitHub Actions 自动打包发布 + electron-updater 在线更新 |
| Agent 接入 | MCP（stdio / HTTP）+ REST API + CLI（mark3labs/mcp-go） |

### 本地开发

前置：Go 1.25+、Node.js 22+、pnpm（corepack）。

```bash
# 后端：编译 daemon 与 CLI（目标平台二进制）
make build                      # 或：GOOS=darwin GOARCH=arm64 go build -o bin/... ./cmd/...

# 前端：安装依赖 + 开发启动（自动拉起守护进程）
cd app && pnpm install
cd .. && ./scripts/dev-run.sh   # Windows: scripts\dev-run.bat
```

### 质量门禁

```bash
make check        # gofmt + vet + lint + go test + 覆盖率 + 集成测试 + 前端 typecheck
pnpm test         # 前端 Vitest（含覆盖率门槛）
```

> 核心 `internal/task` 覆盖率门槛 ≥ 90%；未通过门槛的代码不得提交（见 `AGENTS.md` §10）。

### 发布

```bash
# 1. 更新 app/package.json 版本 + CHANGELOG.md
# 2. 打标签并推送（触发 release.yml 自动打包发布）
git tag vX.Y.Z && git push origin vX.Y.Z
```

详见 [`docs/CI-CD-UPDATER.md`](docs/CI-CD-UPDATER.md) 与 [`docs/BUILD-RELEASE.md`](docs/BUILD-RELEASE.md)。

---

## 目录结构

```
├── cmd/                 # CLI 与守护进程入口
├── internal/            # Go 业务层（api / task / project / skill / audit / mcp / guide …）
├── app/                 # Electron + React 桌面客户端
│   ├── electron/        #   主进程 / preload
│   └── src/             #   渲染进程（features / hooks / components / stores）
├── scripts/             # 开发运行脚本（dev-run.sh 等）
├── test/                # 端到端 / 验收脚本
└── AGENTS.md            # 开发约束（架构分层铁律、工具链约定）
```

## 项目文档

| 文档 | 说明 |
|------|------|
| [`docs/REQUIREMENTS.md`](docs/REQUIREMENTS.md) | 需求基线（最高优先级） |
| [`docs/TECHNICAL.md`](docs/TECHNICAL.md) | 技术落地说明 |
| [`docs/CI-CD-UPDATER.md`](docs/CI-CD-UPDATER.md) | CI/CD 自动发布 + 在线更新方案 |
| [`docs/BUILD-RELEASE.md`](docs/BUILD-RELEASE.md) | 本地打包指南 |
| [`docs/README.md`](docs/README.md) | docs 目录索引 |

## 许可证

[GNU General Public License v3.0](LICENSE)（GPL-3.0，强 Copyleft）——依据源代码重新分发或修改后的衍生作品，必须同样以 GPL-3.0 开源发布。
