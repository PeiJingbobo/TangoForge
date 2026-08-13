# Changelog

所有显著变更均记录于本文件。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循 [SemVer](https://semver.org/lang/zh-CN/)。

> 版本号以 `app/package.json` 的 `version` 为唯一事实源；GitHub Release 标签须与之强一致（`release.yml` 强校验）。

## [0.7.0] - 2026-08-13

### 新增

- **知识库（M7 / TF-044~TF-052）**：命名多库 + 文档引用注册表 + 任务关联 + 语义索引。
  - 数据模型与迁移 v5/v6：`knowledge_bases / knowledge_documents / knowledge_base_documents / task_documents / knowledge_chunks` 5 表 + `archived` 列；项目初始化自动创建默认库。
  - 文档注册/复用/拷贝（`auto` / `copy` / `none`）、relink 历史、解除引用、二进制仅注册。
  - 语义索引：Markdown 标题分块 → LLM 摘要（≤200 字，hash 缓存）→ 向量嵌入（OpenAI / Ollama 双协议）→ 纯 Go 余弦检索（topK + 命中片段 + kb/阈值过滤）。
  - 文件扫描与防抖：fsnotify 监听 + 启动扫描 + 手动扫描；注册即自动索引（修复「注册后永不索引」）；模型漂移自动重嵌；嵌入任务队列（排队/进行中/失败重试/取消 + WS 实时推送）。
  - 归档/还原：归档后从默认列表/检索隐藏，任务引用与文件保留。
  - 三端等价：`/api/knowledge/*` HTTP 端点 + MCP 8 工具（list / search / read / link / unlink / relink / scan / edit）+ CLI `knowledge` 子命令。
  - 前端：任务详情「资料」区、知识库页（库/文档/检索/扫描/添加文件/归档视图）、文档抽屉（阅读/编辑/relink）、设置页「知识库」tab（含 QA-K23 向量开关置灰 + 连接测试按钮）。
  - 导入草稿流 `knowledge_files`（LLM 建议关联）+ 导出「资料」行（往返一致）。
- **守护进程空闲重启（TF-053）**：APP 启动检测 daemon 版本与自身不匹配 → `POST /api/daemon/restart` → `http.Server.Shutdown` 等待进行中请求完成（不打断 CLI）→ 用新二进制自我重生（跨平台 setsid / CREATE_NEW_PROCESS_GROUP）；`GET /api/daemon/version` 免鉴权版本探测；版本经 Makefile/release.yml 从 `app/package.json` 注入。
- **文件/目录选择器默认打开当前项目根目录**：导入、知识库添加文件、引导导入等 4 处调用点统一。

### 变更

- `app/package.json` version 0.6.5 → 0.7.0。
- `internal/task` 权限动作新增 `knowledge.read / knowledge.write / knowledge.index`（默认只读 read；存量项目经客户端授权后生效）。
- README 新增「知识库」章节与核心特性/常用场景条目。

### 修复

- CI golangci-lint 36 项问题（revive / gofumpt / errcheck / staticcheck / unused）。
- `FileFingerprint` 改用 inode+size 组合校验（修复 CI 上删除重建后 inode 快速复用导致指纹失效误判）。
- 知识库：嵌入前置 `indexing` 中间态（列表「正在嵌入」徽标 + 顶部进度条）、扫描补嵌入有 hash 未嵌入文档、选中库改后端 `filter[kb_id]` 过滤、注册顺序化修复状态条停滞、refetchInterval 空数据防御。

## [0.6.5] - 2026-08-12

### 修复

- **Windows 在线更新签名校验失败（TF-036）**：electron-updater 默认校验要求安装包证书链受信任（`Get-AuthenticodeSignature Status==Valid`），自签名证书（无受信任根）必然返回 `not signed by the application owner`，更新无法安装。修复：覆盖公开钩子 `autoUpdater.verifyUpdateCodeSignature` 跳过该校验（安装包完整性已由 latest.yml 的 sha512 + GitHub HTTPS 保证）；Phase 2 换上正式证书后恢复默认严格校验。

### 变更

- `app/package.json` version 0.6.4 → 0.6.5。

## [0.6.2] - 2026-08-12

### 修复

- **主进程启动崩溃（TF-036）**：`electron-updater` 为 CommonJS 包，而主进程产物为 ESM（`package.json "type": "module"`），`import { autoUpdater } from 'electron-updater'` 命名导入被 Node ESM-CJS 互操作拒绝（`Named export 'autoUpdater' not found`），启动即 Uncaught Exception。修复：改为默认导入再解构（`import electronUpdater from 'electron-updater'` + `const { autoUpdater } = electronUpdater`）。

### 变更

- `app/package.json` version 0.6.1 → 0.6.2。

## [0.6.1] - 2026-08-12（启动即崩，未可用）

### 修复

- **macOS 打包畸形（TF-036）**：CI 在仓库根目录运行 electron-builder，未加载 `app/electron-builder.yml`，导致产物名为 `tangoforge-app.app`、bundle id 为默认 `com.electron.tangoforge-app`、运行时依赖（electron-updater 等）打包不全、启动即崩溃。修复：mac/win 打包步骤加 `working-directory: app`，产物恢复为 `TangoForge.app`（productName）。

### 变更

- `app/package.json` version 0.6.0 → 0.6.1。

## [0.6.0] - 2026-08-12（打包畸形，未可用）

### 修复

- **Windows 自签名证书生成（CI）**：修复 Git Bash 将 `-subj "/CN=TangoForge"` 误转为路径导致 openssl 失败；改用 `MSYS2_ARG_CONV_EXCL="/CN*"` 仅对该参数禁用路径转换，并把证书文件写入工作区（避免 `/tmp` 转换问题）。
- 版本号 0.5.0 → 0.6.0（v0.5.0 发布链路未跑通，递增后重新发布）。

### 变更

- `app/package.json` version → 0.6.0。

## [0.5.0] - 2026-08-12（发布链路失败，未对外发布）

> v0.5.0 的 GitHub Actions 打包在 Windows 自签名步骤失败（Git Bash 路径转换），未生成可用 Release；功能变更已在 0.6.0 中继承。

### 新增

- **CI/CD 自动发布（TF-036）**：推送 `vX.Y.Z` 标签 → GitHub Actions 质量门禁 → macOS（arm64，未签名 dmg/zip）与 Windows（x64，自签名 nsis/portable）打包并发布到 GitHub Releases。
- **在线更新（TF-036）**：「设置 → 关于」检查更新；Windows 支持 electron-updater 全链路自动更新（检测 → 下载 → 重启安装）；macOS 未签名阶段检测新版本后自动打开 dmg 下载页由用户手动安装。
- **README 更新**：新增「下载与更新（GitHub Releases）」章节与 macOS 未签名允许运行指令（`xattr -dr com.apple.quarantine` / 右键打开）。

### 变更

- `app/package.json`：version `0.1.0` → `0.5.0`；新增生产依赖 `electron-updater`。
- `app/electron-builder.yml`：新增 GitHub publish 配置；macOS 保持 `identity: null` 未签名；Windows 移除 `signAndEditExecutable: false`，由 CI 注入自签名证书（`CSC_LINK`/`CSC_KEY_PASSWORD`）完成签名。

### 已知限制（未签名阶段）

- macOS 产物未代码签名，首次打开需手动允许（见 README）；在线更新为「自动打开 dmg 手动安装」。
- Windows 使用 CI 内生成的自签名证书，SmartScreen 会提示「未知发布者」（点「更多信息 → 仍要运行」即可）。
