# TangoForge

> 人机协作任务管理中间件 —— 本地守护进程 + 桌面客户端，为 **AI Agent** 提供 MCP / HTTP / CLI 三通道接入。

TangoForge 将项目任务池、状态机、导入导出与 **Agent 协作** 打包成一个本地服务：人类在桌面客户端（看板 / 导航 / 全景图）操作任务，AI Agent 通过 MCP / HTTP / CLI 以**受控权限**读写同一份数据——谁改了什么，全部落在审计日志里。

## 核心特性

- **本地优先**：SQLite（WAL）+ 本地守护进程，数据全部落在你的工作目录，无云端依赖。
- **任务管理**：看板拖拽流转、树形导航、状态机动态列、Markdown 导入导出（草稿审阅后确认）。
- **Agent 三通道**：MCP（stdio / HTTP）、REST API、CLI——同一业务层，行为一致。
- **权限模型**：UI 全权；Agent 默认只读，写操作需显式授权（17 项 action 按项目配置）。
- **Skills 技能包**：为 Claude Code / Cursor / Copilot / WorkBuddy 等宿主安装可发现的技能包（SKILL.md），多选宿主一键安装。
- **审计日志**：全部写操作异步落库（actor / action / target / result），只追加不可篡改，可导出。
- **桌面客户端**：Electron + React 19 + shadcn-ui，温和协作风视觉体系。

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

## 技术栈

| 层 | 技术 |
|---|---|
| 守护进程 | Go（chi router，SQLite WAL，异步审计，WebSocket 事件广播） |
| 桌面客户端 | Electron 43 + React 19 + TypeScript + shadcn-ui（Radix + Tailwind CSS 4） |
| 构建 | electron-vite + electron-builder（mac dmg/zip、Windows nsis/portable） |
| Agent 接入 | mark3labs/mcp-go（MCP Server，24 个工具） |

## 快速开始

### 前置

- Go 1.26+、Node.js 22+、pnpm（corepack）
- 国内网络建议配置镜像：`GOPROXY=https://goproxy.cn,direct`、npm registry 使用 npmmirror

### 安装依赖与构建

```bash
# 后端
go build -o bin/tangoforge ./cmd/cli
go build -o bin/tangoforge-daemon ./cmd/daemon

# 前端
cd app && pnpm install
```

### 运行

```bash
./scripts/dev-run.sh           # 自动拉起守护进程 + 启动桌面客户端
./scripts/dev-run.sh debug     # 调试模式（打开 DevTools）
```

首次启动进入项目导入引导：选择工作目录 → LLM 接入 → 导入草稿 → Agent 权限 → 安装 Skill → 进入项目。

## 构建预发布版本

```bash
cd app
export ELECTRON_MIRROR=https://npmmirror.com/mirrors/electron/
export ELECTRON_BUILDER_BINARIES_MIRROR=https://npmmirror.com/mirrors/electron-builder-binaries/

pnpm dist:mac    # macOS：dmg + zip
pnpm dist:win    # Windows：nsis + portable
```

> 产物默认输出到本机磁盘（如 `~/tangoforge-release`）；未签名，安装时系统会提示"未知开发者"。

## AI Agent 接入

```bash
# MCP（stdio）
tangoforge mcp --project /path/to/project

# CLI（等价 HTTP）
tangoforge tasks list --project /path/to/project

# HTTP
curl -H "X-Project: /path/to/project" http://127.0.0.1:19810/api/guide
```

Agent 默认**只读**；写操作（创建/流转/导入导出等）需在客户端「Agent 权限」页显式授予。

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

## 开发约定

- 开发规范与架构约束见根目录 `AGENTS.md`（分层铁律、测试、提交规范）。
- 任务：`go test ./...`；前端：`cd app && pnpm test && pnpm run typecheck`。

## License

[GNU General Public License v3.0](LICENSE)（GPL-3.0，强 Copyleft）

依据源代码重新分发或修改后的衍生作品，必须同样以 GPL-3.0 开源发布。
