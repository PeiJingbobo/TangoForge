# TangoForge 预发布版本编译指南

> 面向 mac / Windows 双平台的预发布（pre-release，未签名）构建流程。
> 实测基准：2026-08-08，Electron 43.3.0 + electron-builder 26.15.3（macOS 25.5 arm64，mac 打包约 30s）。

## 0. 总览

| 项 | 说明 |
|---|---|
| 编译管线 | `electron-vite build`（main/preload/renderer → `out/`）→ `electron-builder`（→ 安装包） |
| 打包工具 | electron-builder（`app/electron-builder.yml`），产物目录默认 `app/release/` |
| 双平台原则 | **各自本机打包**（mac 出 .dmg/.zip，Windows 出 .exe）；不建议跨平台交叉打包 |
| 预发布定位 | 未代码签名（mac `identity: null`、win `signAndEditExecutable: false`），安装时系统会提示"未知开发者/来自不明发布者"，属预期行为 |

## 1. 前置条件

- Node.js 22（`~/node-sdk/bin`）+ pnpm（corepack）。
- **Electron 版本必须精确固定**（`app/package.json` 中为 `"electron": "43.3.0"`，无 `^`）——electron-builder 要求精确版本，range 会直接报错退出。
- 依赖已安装：`cd app && corepack pnpm install`（node_modules hoisted 于仓库根目录）。

### ⚠️ 必设镜像环境变量（国内网络关键）

electron-builder 首次运行会从 **GitHub** 下载 Electron 二进制与辅助工具（dmg 打包等）。
国内直连 GitHub 会**长时间卡死**（连接 ESTABLISHED 但无进度），必须走镜像：

```bash
export ELECTRON_MIRROR=https://npmmirror.com/mirrors/electron/
export ELECTRON_BUILDER_BINARIES_MIRROR=https://npmmirror.com/mirrors/electron-builder-binaries/
```

Windows（cmd/PowerShell）对应：

```powershell
$env:ELECTRON_MIRROR="https://npmmirror.com/mirrors/electron/"
$env:ELECTRON_BUILDER_BINARIES_MIRROR="https://npmmirror.com/mirrors/electron-builder-binaries/"
```

### ⚠️ 输出目录建议用本地磁盘

本项目仓库位于 SMB 共享盘（mac 上 `/Volumes/HD-DATA` / Windows 上 `D:\Coding\`）。
electron-builder 需解包 Electron（约 300MB）+ 生成 .app/.exe，**在共享盘上写入极慢（曾 10 分钟无产出）**。
**打包输出目录指向本机磁盘**（见下方命令的 `-c.directories.output=...`）。

## 2. macOS 构建（本机，arm64）

```bash
# ① 设置镜像（见 §1）
export ELECTRON_MIRROR=https://npmmirror.com/mirrors/electron/
export ELECTRON_BUILDER_BINARIES_MIRROR=https://npmmirror.com/mirrors/electron-builder-binaries/

# ② 编译 + 打包（产物输出到本机 ~/tangoforge-release，避开共享盘）
cd ~/HD-DATA/Coding/TangoForge/app
corepack pnpm build && \
  corepack pnpm exec electron-builder --mac -c.directories.output=$HOME/tangoforge-release
```

> 必须用 `corepack pnpm exec electron-builder`（或 `pnpm dist:mac`）调用——本项目依赖 hoisted 于
> 共享盘根目录 `node_modules/.bin/`，**zsh 直接敲 `electron-builder` 会报 command not found**；
> `pnpm exec` 会自动把依赖的 `.bin` 加入 PATH。

产物（`~/tangoforge-release/`）：

```
TangoForge-0.1.0-mac-arm64.zip    # 通用分发（zip，119M）
TangoForge-0.1.0.dmg              # 安装镜像（119M）
```

- 版本号：`app/package.json` 的 `version` 字段（发布前按需递增）。
- 首次打包会从镜像下载 Electron 二进制 + dmg 工具（一次性，缓存于 `~/Library/Caches`）。
- 未签名 + 默认 Electron 图标 → 构建日志出现两处警告，**不阻断**（正式发布时再补图标 `build/icon.icns` 与签名）。

## 3. Windows 构建（本机）

> ⚠️ 尚未在本机实测（Windows 侧为共享盘映射 + SMB 写入限制），以下为配置语义等价命令，首次执行请留意 §4 常见问题。

**路径 A：Windows 本机直接打包**（node_modules 可用时）

```powershell
cd D:\Coding\TangoForge\app
$env:ELECTRON_MIRROR="https://npmmirror.com/mirrors/electron/"
$env:ELECTRON_BUILDER_BINARIES_MIRROR="https://npmmirror.com/mirrors/electron-builder-binaries/"
corepack pnpm build
corepack pnpm exec electron-builder --win -c.directories.output=$env:TEMP\tangoforge-release
```

产物：`TangoForge-0.1.0-win-<arch>.exe`（nsis 安装包）+ `TangoForge-0.1.0-setup.exe` + portable exe。

**路径 B：共享盘上 pnpm/文件操作失败时**——将仓库复制到 Windows 本地磁盘（如 `C:\dev\TangoForge`，可用 `robocopy /E`，排除 `.git` 与 `node_modules`），在该副本执行 `corepack pnpm install` + 路径 A 命令。

## 4. 常见问题

| 症状 | 原因 | 处理 |
|---|---|---|
| `zsh: command not found: electron-builder` | 依赖 hoisted 在共享盘根 `node_modules/.bin`，shell PATH 不含 | 用 `corepack pnpm exec electron-builder ...` 或 `pnpm dist:mac` |
| 打包长时间无输出，`lsof` 显示连接 `github.com:https ESTABLISHED` | 辅助工具从 GitHub 直连下载被墙 | 设置 `ELECTRON_BUILDER_BINARIES_MIRROR`（§1）后重跑 |
| `Electron version "^43.3.0" is a range` 直接退出 | electron-builder 要求精确版本 | `package.json` 固定 `"electron": "43.3.0"` |
| 共享盘上打包极慢/卡住 | SMB 大文件 + 小文件写入慢 | 输出目录指向本机（`-c.directories.output=`） |
| `default Electron icon is used` | 未提供应用图标 | 预发布可忽略；正式发布补 `build/icon.icns`（mac）/ `icon.ico`（win） |
| `skipped macOS code signing` | 预发布未签名（配置 `identity: null`） | 正式发布移除该行并配置开发者证书 |
| 安装时提示"无法验证开发者/不明发布者" | 未签名产物 | 预发布预期行为，右键打开 / 接受提示 |

## 5. 正式发布（发布前切换）

- 补图标：`app/build/icon.icns`、`app/build/icon.ico`（electron-builder 自动读取）。
- mac 签名：删除 `electron-builder.yml` 中 `mac.identity: null`，配置 Developer ID 证书。
- win 签名：删除 `win.signAndEditExecutable: false`，设置 `CSC_LINK`/`CSC_KEY_PASSWORD`。
- 版本号：递增 `app/package.json` 的 `version`。
