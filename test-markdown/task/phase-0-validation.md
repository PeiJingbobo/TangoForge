# Phase 0 验证记录

- 日期：2026-07-31
- 范围：P0-01 至 P0-05
- 结论：Phase 0 已完成

## 仓库起点

- `Coding/FarframeRDP` 已确认为项目根目录。
- 开始时目录不是 Git 仓库，且没有 Xcode project/workspace 或工程代码。
- 已初始化 `main` 分支 Git 仓库；按仓库规则未创建提交。
- 已添加 README、忽略规则、ADR、兼容性矩阵、第三方许可证和本地集成配置骨架。

## 已确认决策

- 最低系统：macOS 14.0。
- Bundle ID：`com.farframe.rdp`。
- Keychain service：`com.farframe.rdp.credentials`。
- 非秘密存储：SwiftData。
- 首发架构：arm64。
- 工程组织：单一 Xcode project，分离 App、Core、Bridge 和测试 targets。

## Mac 构建环境快照

以下为只读探测结果，不包含连接地址、账号、签名身份或证书：

| 项目 | 结果 |
|---|---|
| CPU 架构 | arm64 |
| macOS | 26.5.2 |
| Xcode | 26.2（Build 17C52） |
| macOS SDK | 26.2 |
| Apple Clang | 17.0.0 |
| Git | 2.50.1（Apple Git-155） |
| CMake | 未安装或不在 PATH |
| Ninja | 未安装或不在 PATH |
| 可用代码签名身份 | 1 个，仅记录数量 |
| Xcode project/workspace | 尚不存在 |

CMake 和 Ninja 是 Phase 2 构建固定 FreeRDP 原生依赖的前置工具，不阻塞 Phase 1 创建应用骨架。

## 在 Mac 上执行的只读命令

```sh
uname -m
sw_vers -productVersion
xcode-select -p
xcodebuild -version
xcrun --sdk macosx --show-sdk-version
xcodebuild -showsdks
git --version
xcrun clang --version
command -v cmake
command -v ninja
security find-identity -v -p codesigning
find <project-root> -maxdepth 2 \( -name '*.xcodeproj' -o -name '*.xcworkspace' \)
```

实际远程连接命令、账号和机器路径不写入仓库。

## 结果边界

- SSH 连接和只读工具探测成功。
- 尚无工程，因此不能列出 scheme、destination 或执行构建测试。
- 尚未验证 GUI 启动、Keychain 提示、Accessibility/Input Monitoring、Metal、全屏或物理键盘。
- `codesign --version` 不是该工具支持的参数；此探测子命令失败，不影响签名身份数量检查。

## 后续动作

1. Phase 1 创建工程后重新探测 project、scheme 和 destination。
2. Phase 2 前以可复现方式补齐 CMake/Ninja，不在运行时依赖 Homebrew FreeRDP。
3. 在真实或虚拟 macOS 14 环境补做最低系统兼容验收。
