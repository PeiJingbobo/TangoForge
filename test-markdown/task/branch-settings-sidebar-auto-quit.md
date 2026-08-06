# B7-01 设置侧边栏与窗口生命周期支线任务

## 1. 任务状态

- 任务 ID：B7-01
- 状态：待人工验收
- 启动背景：Phase 7 完成后的独立支线
- 关联开发计划：步骤一的应用外壳与窗口生命周期、第五节设置界面规格、最终完成定义
- 最近更新：2026-08-01

## 2. 目标与范围

本任务包含两个可观察行为：

1. 将软件 Settings 重构为接近 macOS 系统设置的信息架构，左侧固定显示设置分类，右侧显示当前分类内容。
2. 当主窗口、设置窗口、远程窗口等全部关闭后，Farframe RDP 自动退出，不在 Dock 中留下无窗口进程。

本任务不改变快捷键策略、Profile、Keychain、可达性探测或 RDP 会话实现；只重新组织既有设置项并调整应用级窗口生命周期。

## 3. 实现记录

### 3.1 设置页

- 新增 `SettingsDestination`，稳定定义“通用”“网络与状态”“键盘与快捷键”三个分类及 SF Symbols。
- 使用固定宽度的 sidebar `List` 与右侧内容区组成 `HStack`，没有 `NavigationSplitView` 的折叠状态、折叠按钮或单栏降级路径。
- 侧边栏宽度固定为 220 pt；内容区最小 600 pt，窗口最小高度 620 pt。
- 右侧使用原生 `Form`、`Section`、`Toggle`、`Picker` 和语义颜色，保留原有自动可达性设置和全部快捷键策略行为。
- 为侧边栏及每个分类增加稳定的辅助功能标识，便于后续 UI 测试。

### 3.2 应用退出

- 新增 `FarframeApplicationDelegate`，通过 AppKit 的 `applicationShouldTerminateAfterLastWindowClosed` 返回 `true`。
- 使用 `NSApplicationDelegateAdaptor` 接入 SwiftUI App 生命周期。
- 退出判断由应用统一处理，不针对某一个 SwiftUI 或 AppKit 窗口做计数，因此只要仍有任意窗口打开，应用不会提前退出。

## 4. 自动化验证

在项目指定 Mac 环境执行：

~~~sh
xcodebuild \
  -project FarframeRDP.xcodeproj \
  -scheme FarframeRDP \
  -configuration Debug \
  -destination 'platform=macOS,arch=arm64' \
  -derivedDataPath .derivedData \
  CODE_SIGNING_ALLOWED=NO \
  test \
  -only-testing:FarframeRDPTests/FarframeRDPTests/testSettingsSidebarHasStableNonCollapsibleWidthAndDestinations \
  -only-testing:FarframeRDPTests/FarframeRDPTests/testApplicationTerminatesAfterLastWindowCloses
~~~

结果：2 项测试通过，0 失败、0 跳过。构建目标为 arm64、macOS 14 deployment target，编译启用 warnings-as-errors。

随后执行完整回归与 Release 构建：

- `scripts/test.sh`：53 项测试通过，0 失败、0 跳过。
- `FARFRAME_CONFIGURATION=Release FARFRAME_DERIVED_DATA_PATH=.derivedData-b7-01-release /bin/sh scripts/build.sh`：构建成功。

首次执行时，测试补丁经 PowerShell 管道传输导致两个中文期望值变为问号，因此侧边栏测试失败；应用源码未受影响。改用 Unicode 转义修复测试传输编码后重新执行，两项测试均通过。该次失败不计为功能通过证据。

## 5. 尚需人工验收

命令行测试不能证明设置窗口的实际布局、窗口缩放、浅色/深色可读性，也不能证明真实进程在最后窗口关闭后退出。必须按
[`manual-verification/B7-01.md`](manual-verification/B7-01.md) 在已解锁的 Mac 图形会话中执行。

在人工步骤全部通过前，本任务保持“待人工验收”。

## 6. 残余风险与限制

- 固定侧边栏以最小窗口宽度换取不可折叠行为；小屏幕上需要人工确认窗口仍能完整显示。
- 自动退出符合本次需求，但与某些 macOS 应用“关闭窗口后继续驻留”的惯例不同；验证时应确认设置窗口或远程窗口仍打开时不会提前退出。
- 本任务没有增加恢复已关闭主窗口的 Dock 重开行为，因为最后窗口关闭后进程会直接结束。
- 未修改任何凭据、证书、会话输入或原生 Bridge 代码。