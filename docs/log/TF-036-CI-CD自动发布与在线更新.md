# TF-036 CI/CD 自动发布 + 在线更新 — 任务日志

## 2026-08-12

- **进展**：
  - 评审通过 `docs/CI-CD-UPDATER.md`（v1.0），按评审决策落地实现（TF-036）。
  - `app/package.json`：version 0.1.0 → 0.5.0（首个 CI 发布版本，对齐里程碑 v0.4.0 之后）；新增生产依赖 `electron-updater@6.8.9`。
  - `app/electron-builder.yml`：新增 GitHub publish（`PeiJingbobo/TangoForge`）；macOS 保持 `identity: null`（未签名，评审决策）；Windows 移除 `signAndEditExecutable: false`，改由 CI 注入自签名证书签名。
  - 新增 `.github/workflows/release.yml`：`v*` 标签触发 → 版本强一致校验 → 质量门禁 → Release 骨架（`gh release create`）→ macos-14(arm64) 未签名打包（dmg+zip+latest-mac.yml）+ windows-latest(x64) 自签名打包（nsis+portable+latest.yml）→ `--publish always`。
  - 在线更新：新增 `app/electron/updater.ts`（Windows electron-updater 全链路；macOS GitHub Releases API 检测 + 自动打开 dmg，按版本持久化去重）；`main.ts` 注册 IPC + 启动延迟 10s 自动检查；`preload.ts` 暴露 `window.tangoforge.update`；新增共享类型 `app/src/types/update.ts`。
  - UI：「设置 → 关于」tab（`UpdateSection.tsx` + 8 条测试）。
  - README 新增「下载与更新」章节 + macOS 未签名允许运行指令（`xattr -dr com.apple.quarantine` / 右键打开）。
- **决策**：
  - macOS 本阶段不做代码签名 → dmg 手动安装为官方更新路径；Windows 用 CI 内 openssl 生成的自签名 .pfx 签名（SmartScreen「未知发布者」属预期）。
  - mac 更新检查使用 GitHub Releases API（`releases/latest` + dmg 资产 URL），不启用 electron-updater（未签名下 zip 无法应用）。
  - 版本对齐：tag==`app/package.json` version 强校验；首个发布 `v0.5.0`。
- **踩坑**：
  - electron-updater 误放入 devDependencies（electron-builder 打包时 main 进程运行时 require 会缺依赖）→ 移入 dependencies。
  - `pnpm install` 后 node_modules 物理布局错乱（SMB + `node-linker=hoisted`），`brace-expansion@2.1.4` 嵌套到 `balanced-match@4.0.4`（lockfile 语义应为 1.0.2），导致 vitest coverage 报 `balanced is not a function` → 删除 node_modules 全新安装解决（lockfile 本身正确）。
  - `format:check` 会把 `app/out`、`app/release` 构建产物纳入扫描 → 新增 `app/.prettierignore` 排除。
- **验证**：
  - `pnpm typecheck` ✅；`pnpm lint` ✅；`pnpm format:check` ✅；`pnpm test` 241 用例全绿 ✅；`pnpm test:coverage`（lines 87.79% > 70 门槛）✅。
  - 本机 electron-builder mac 出包冒烟：`TangoForge-0.5.0.dmg` / `TangoForge-0.5.0-mac-arm64.zip` + `latest-mac.yml` + `app-update.yml`（publish 配置正确）+ `app.asar` 内含 electron-updater（38 文件）✅。
  - 待办：GitHub Actions release.yml 实跑；win/mac 更新链路真机验证（见 `docs/record/TF-036-*` 遗留）。

## 2026-08-12（第二次 CI 迭代）

- **进展**：v0.5.0 release.yml 实跑，mac 打包正常；Windows 卡在自签名证书生成。
- **踩坑与解决**：
  - Git Bash 把 `-subj "/CN=TangoForge"` 误转成路径 `C:/Program Files/Git/CN=TangoForge` → 先加 `MSYS_NO_PATHCONV=1`，结果连 `/tmp/...` 路径也不转换（openssl 报 Can't open /tmp/... for writing）→ 改用 `MSYS2_ARG_CONV_EXCL="/CN*"` 仅对该参数禁用转换，证书文件改写到工作区 `$(pwd)`（MSYS 仍正常转换为 Windows 路径）。
- **决策**：版本号 0.5.0 → 0.6.0（发布链路未跑通不对外发版，递增重发，tag `v0.6.0`）；CHANGELOG 记录 0.5.0 未发布。
- **验证**：openssl req/pkcs12 本地复现通过（subject=/CN=TangoForge + codeSigning EKU）；待 v0.6.0 实跑。

## 2026-08-12（第三次 CI 迭代：mac 打包畸形 + ESM 导入崩溃）

- **进展**：v0.6.0 mac 产物畸形（`tangoforge-app.app`）、启动即崩；v0.6.1 修好打包但 Windows 打开报 `Named export 'autoUpdater' not found`，mac 同源崩溃。
- **根因与解决**：
  1. **打包畸形**：CI 在仓库根目录运行 `electron-builder`，未加载 `app/electron-builder.yml`（默认 productName/appId/files）→ mac/win 打包步骤加 `working-directory: app`，产物恢复 `TangoForge.app`。
  2. **启动崩溃**：`electron-updater` 是 CJS，主进程产物是 ESM（`type: module`），`import { autoUpdater }` 命名导入被 Node ESM-CJS 互操作拒绝 → 改为默认导入解构（`import electronUpdater from 'electron-updater'`）；已核对 out/main 其余外部导入均安全。
- **决策**：版本 0.6.0 → 0.6.1 → 0.6.2（每轮修复递增，均未对外可用）。
- **验证**：typecheck / lint / UpdateSection 8 用例通过；`import('electron-updater').default.autoUpdater` 在纯 Node 下可解析（仅缺 electron.app）；out/main/index.js 已为默认导入形式。

## 2026-08-12（第四次 CI 迭代：Windows 更新签名校验）

- **进展**：0.6.2 双平台可启动；用户推 0.6.3/0.6.4 验证更新，Windows 报 `not signed by the application owner`。
- **根因**：electron-updater 的 `NsisUpdater.verifySignature` 从 `app-update.yml` 读 `publisherName`（由签名证书 CN=TangoForge 派生），用 PowerShell `Get-AuthenticodeSignature` 校验新安装包并要求 `Status==Valid`（证书链受信任）。自签名证书无受信任根 → `Status=1 NotTrusted` → 校验必然失败。
- **解决**：覆盖 `NsisUpdater` 公开钩子 `verifyUpdateCodeSignature`（setter 指向内部 `_verifyUpdateCodeSignature`，`verifySignature` 实际调用它）返回 `null` 跳过校验；完整性由 latest.yml sha512 + GitHub HTTPS 保证。Phase 2 正式证书后移除。
- **决策**：版本 0.6.4 → 0.6.5。
- **验证**：typecheck / build 通过；待 v0.6.5 实跑：已装 0.6.2/0.6.3 → 检查更新 → 下载 → 重启安装。

## 2026-08-18（v0.7.2 正式发布）

- **发布准备**：将全部工作树改动提交并合入 `main`；`app/package.json` 升级为 `0.7.2`，补充 `CHANGELOG.md`。
- **Git 结果**：功能提交 `dd4ee17`；合并提交 `81f5e28`；发布提交 `6e43bd2`；`main` 与注解标签 `v0.7.2` 已推送远端。
- **本地验证**：前端 49 个测试文件、322 个测试通过；`pnpm typecheck` 与 `pnpm build` 通过；`internal/task` 覆盖率 91.4%。Mac 本机完整 `make check` 的覆盖率步骤受缺少 Go `covdata` 工具及一次临时目录清理竞争影响，已由 GitHub Actions 质量门禁完成最终验证。
- **CI 实跑**：GitHub Actions Release Run #14 成功，总耗时 5m50s；版本校验与质量门禁、Release 骨架、macOS arm64、Windows x64 四个 job 全部成功。
- **发布资产**：Release 共 11 项资产；主要产物为 `TangoForge-0.7.2.dmg`、`TangoForge-0.7.2-mac-arm64.zip`、`TangoForge-0.7.2-setup.exe`、`TangoForge-0.7.2-win-x64.exe`，并包含 `latest-mac.yml`、`latest.yml` 与 blockmap。
- **人工验收待办**：尚未在真实旧版本安装环境中执行 Windows 自动升级与 macOS 打开 dmg 的端到端验证；该项继续作为人工验证手册保留。
