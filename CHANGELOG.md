# Changelog

所有显著变更均记录于本文件。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循 [SemVer](https://semver.org/lang/zh-CN/)。

> 版本号以 `app/package.json` 的 `version` 为唯一事实源；GitHub Release 标签须与之强一致（`release.yml` 强校验）。

## [0.5.0] - 2026-08-12

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
