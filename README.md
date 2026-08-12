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
# 后端（显式指定平台；Windows 产物带 .exe）
# macOS（arm64）
GOOS=darwin GOARCH=arm64 go build -o bin/tangoforge ./cmd/cli
GOOS=darwin GOARCH=arm64 go build -o bin/tangoforge-daemon ./cmd/daemon

# Windows（amd64，PowerShell 语法）
$env:GOOS="windows"; $env:GOARCH="amd64"
go build -o bin\tangoforge.exe ./cmd/cli
go build -o bin\tangoforge-daemon.exe ./cmd/daemon

# 前端
cd app && pnpm install
```

> **Windows 首次运行注意**：若 `pnpm dev` 报 `Error: Electron uninstall`（electron 的 postinstall 被 pnpm 跳过导致二进制未下载），手动补齐：
> ```powershell
> $env:ELECTRON_MIRROR="https://npmmirror.com/mirrors/electron/"
> cd C:\path\to\TangoForge
> node node_modules\electron\install.js
> ```

### 运行

**使用脚本（推荐，自动拉起守护进程 + 启动客户端）：**

```bash
# macOS / Linux
./scripts/dev-run.sh            # 正常启动
./scripts/dev-run.sh debug      # 调试模式（打开 DevTools）
```

```bat
:: Windows
scripts\dev-run.bat             :: 正常启动
scripts\dev-run.bat debug       :: 调试模式（打开 DevTools）
```

**不使用脚本，手动运行（任意平台通用）：**

```bash
# ① 启动守护进程（后台常驻，端口 19810；退出 App 后仍在）
bin/tangoforge-daemon          # macOS/Linux；Windows 为 bin\tangoforge-daemon.exe

# ② 启动桌面客户端
cd app && pnpm dev             # 调试模式（打开 DevTools）：ELECTRON_DEBUG=1 pnpm dev
```

> 首次启动进入项目导入引导：选择工作目录 → LLM 接入 → 导入草稿 → Agent 权限 → 安装 Skill → 进入项目。

## 构建预发布版本

```bash
cd app
export ELECTRON_MIRROR=https://npmmirror.com/mirrors/electron/
export ELECTRON_BUILDER_BINARIES_MIRROR=https://npmmirror.com/mirrors/electron-builder-binaries/

pnpm dist:mac    # macOS：dmg + zip
pnpm dist:win    # Windows：nsis + portable
```

> 产物默认输出到本机磁盘（如 `~/tangoforge-release`）；未签名，安装时系统会提示"未知开发者"。

## 下载与更新（GitHub Releases）

安装包由 **GitHub Actions 自动打包并发布**到 [GitHub Releases](https://github.com/PeiJingbobo/TangoForge/releases)（推送 `vX.Y.Z` 标签触发，版本号与 `app/package.json` 强一致）：

- **Windows（x64）**：`TangoForge-<版本>-setup.exe`（NSIS 安装包，自签名）+ `TangoForge-<版本>-win-x64.exe`（便携版）；
- **macOS（arm64）**：`TangoForge-<版本>.dmg`（安装/手动更新用）+ `TangoForge-<版本>-mac-arm64.zip`。

App 内置在线更新：启动后延迟自动检查，也可在「设置 → 关于」手动检查。

| 平台 | 更新方式 |
|---|---|
| Windows | 检测到新版本 → 提示下载 → 「重启并安装」自动完成（electron-updater 全链路） |
| macOS | 未签名阶段**无法自动安装**：检测到新版本后**自动打开 dmg 下载页**，下载后双击 dmg 将 TangoForge 拖入 Applications 完成手动安装 |

> 无签名阶段说明（TF-036 评审决策）：macOS 暂无 Apple Developer ID 证书，产物未签名；Windows 使用 CI 内生成的自签名证书签名（SmartScreen 仍会提示"未知发布者"，点「更多信息 → 仍要运行」即可，属预期）。

### macOS：首次打开未签名 App 的允许运行指令

macOS 未签名产物首次打开会提示「无法验证开发者」或「已损坏，无法打开」，按下列任一方式允许运行：

```bash
# 方式一（推荐）：移除 quarantine 隔离标记，之后可正常双击打开
xattr -dr com.apple.quarantine "/Applications/TangoForge.app"

# 方式二：终端直接启动（绕过 Gatekeeper 校验）
open "/Applications/TangoForge.app"
```

或 GUI 操作：在「访达」中**右键** `TangoForge.app` →「打开」→ 在弹窗中再次点「打开」（仅首次提示一次，此后可正常双击启动）。

## CLI 使用与全局注册

打包产物 `resources/bin/` 内包含 **CLI（tangoforge）** 与 **守护进程（tangoforge-daemon）** 双二进制（App 启动自动拉起 daemon；CLI 通过 HTTP 与 daemon 等价操作，见 `tangoforge --help`）。

**方式一：直接使用（免注册）**

```bash
# Windows（安装目录内）
"C:\Program Files\TangoForge\resources\bin\tangoforge.exe" --help
# macOS（App 内）
/Applications/TangoForge.app/Contents/Resources/bin/tangoforge --help
```

**方式二：注册到全局 PATH（推荐，注册后任意终端直接 `tangoforge`）**

```powershell
# Windows：追加到当前用户 PATH（幂等）
powershell -ExecutionPolicy Bypass -File "<安装目录>\resources\bin\register-cli.ps1"
```

```bash
# macOS / Linux：创建 ~/bin/tangoforge 链接并注入 shell PATH（幂等）
"<App 路径>/Contents/Resources/bin/register-cli.sh"
```

> 注册后**新开终端**生效；CLI 依赖守护进程运行（App 启动即自动拉起，或 `scripts/dev-run.sh` / `dev-run.bat`）。
> 常用：`tangoforge projects list`、`tangoforge tasks list --project <工作目录>`、`tangoforge import preview`、`tangoforge guide`。

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
