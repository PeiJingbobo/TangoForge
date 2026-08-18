# TF-036 CI/CD 自动发布 + 在线更新 — 任务总结

> 结果：部分完成　|　日期：2026-08-18　|　执行人：ai

## 1. 任务范围

按 `docs/CI-CD-UPDATER.md`（v1.0 评审通过）落地 CI/CD 自动发布与在线更新：macOS 未签名打包（dmg 手动安装），Windows 自签名打包（electron-updater 自动更新），经 GitHub Actions + GitHub Releases 分发；README 增 macOS 未签名允许运行指令。

## 2. 交付内容

- **`app/package.json`**：version 0.1.0 → 0.5.0；新增生产依赖 `electron-updater`。
- **`app/electron-builder.yml`**：新增 GitHub publish（`PeiJingbobo/TangoForge`）；mac `identity: null`（未签名）；win 移除 `signAndEditExecutable: false`（CI 自签名）。
- **`.github/workflows/release.yml`**：`v*` 标签触发 → 版本强一致校验 → 质量门禁 → Release 骨架 → mac(arm64) 未签名 + win(x64) 自签名打包发布。
- **`app/electron/updater.ts`**（新）：Windows electron-updater 全链路；macOS 检测新版本后自动打开 dmg 手动安装（每个版本仅一次，持久化去重）。
- **`app/electron/main.ts` / `preload.ts`**：注册 `update:*` IPC；preload 暴露白名单 `window.tangoforge.update`；启动延迟 10s 自动检查。
- **`app/src/types/update.ts`**（新）：更新状态/载荷共享类型。
- **`app/src/features/settings/UpdateSection.tsx` + 测试**（新）：设置「关于」tab（版本 + 检查更新 + 下载/安装 + 进度），8 条测试。
- **`README.md`**：新增「下载与更新（GitHub Releases）」与 macOS 未签名允许运行指令。
- **`CHANGELOG.md`**（新）；文档联动（CI-CD-UPDATER 标记已评审、BUILD-RELEASE、docs/README、AGENTS.md + 副本同步、TASKS/OVERVIEW 登记 TF-036、log/record）。
- **`app/.prettierignore`**（新）：排除构建产物，稳定本地 `format:check`。

## 3. 验证结果

| 项 | 结果 |
|---|---|
| `pnpm typecheck` | ✅ |
| `pnpm lint` | ✅ |
| `pnpm format:check` | ✅ |
| `pnpm test` | ✅ 241 用例全绿（+8 UpdateSection） |
| `pnpm test:coverage` | ✅ lines 87.79%（门槛 70%） |
| 本机 electron-builder mac 出包冒烟 | ✅ `TangoForge-0.5.0.dmg` / `-mac-arm64.zip` / `latest-mac.yml` / `app-update.yml` / asar 内含 electron-updater |
| GitHub Actions release.yml 实跑 | ✅ v0.7.2 Run #14，质量门禁、Release、macOS arm64、Windows x64 全部成功（5m50s） |
| GitHub Release 资产 | ✅ 11 项；含 dmg、mac arm64 zip、Windows setup/portable exe、latest*.yml 与 blockmap |
| 更新链路真机验证（win 自动 / mac 打开 dmg） | ⬜ 人工验收项，尚未执行，不影响自动发布链路完成判定 |

## 4. 遗留问题与后续

- **人工验证手册**：Windows 安装旧版 → 检查 v0.7.2 → 下载 → 重启安装；macOS 安装旧版 → 检查 v0.7.2 → 自动打开 dmg 下载页。该项尚未人工执行，不能视为已验证。
- **CI 维护提醒**：Run #14 有 4 条 Node.js 20 action 运行时弃用警告；后续应升级 `actions/checkout`、`actions/setup-go`、`actions/setup-node` 与 `pnpm/action-setup` 的主版本。
- **Phase 2（后续阶段）**：Apple Developer ID + 公证后 mac 切换 zip 自动更新；win 换正式证书（EV/OV）消除 SmartScreen 提示。
