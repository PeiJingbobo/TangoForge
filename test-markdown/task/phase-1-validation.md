# Phase 1 验证记录

- 日期：2026-07-31
- 范围：P1-01 至 P1-05
- 结论：实现、自动化验证与 Mac 图形会话人工目视验收均已完成
- 人工验收日期：2026-08-01

## 已实现

- 单一 FarframeRDP.xcodeproj，包含 App、Core、Bridge 与三个对应测试 target。
- Debug、Release、Sanitizer 三种构建配置，最低系统 macOS 14.0，首发架构 arm64。
- SwiftUI 应用外壳和 Settings scene。
- AppKit RemoteSessionWindowManager 与可成为第一响应者的 RemoteCanvasView。
- App、Session、Input、Render、Security、Channel 六类 OSLog 分类。
- 稳定 Bundle ID、Keychain service 常量、基础错误类型和诊断脱敏。
- C ABI 版本边界；没有伪装 FreeRDP 已集成，FreeRDP 会话所有权留在 Phase 2。
- 规范化 build.sh 与 test.sh，Derived Data 使用项目内忽略目录或显式覆盖路径。
- App target 嵌入 FarframeCore.framework，并使用 @executable_path/../Frameworks 运行时搜索路径。

## Mac 环境与工程探测

- Xcode project：FarframeRDP.xcodeproj
- scheme：FarframeRDP
- targets：FarframeRDP、FarframeCore、FarframeRDPBridge 及三个测试 target
- configurations：Debug、Release、Sanitizer
- destination：platform=macOS,arch=arm64
- 工具链快照沿用 Phase 0：Xcode 26.2、macOS SDK 26.2、Apple Silicon arm64

以上记录不包含连接地址、账号、签名身份或机器特定路径。

## 最终通过的命令

~~~sh
xcodebuild -list -project FarframeRDP.xcodeproj
plutil -lint FarframeRDP.xcodeproj/project.pbxproj
/bin/sh scripts/test.sh
FARFRAME_CONFIGURATION=Release /bin/sh scripts/build.sh
FARFRAME_CONFIGURATION=Sanitizer /bin/sh scripts/build.sh
~~~

结果：

- project.pbxproj 语法检查通过。
- Debug 完整测试：7 项，0 失败。
  - FarframeCoreTests：4 项。
  - FarframeRDPTests：2 项。
  - FarframeRDPBridgeTests：1 项。
- Release 构建通过，FarframeCore.framework 已嵌入应用。
- Sanitizer（ASan + UBSan）构建通过，FarframeCore.framework 已嵌入应用。
- 在排除 Git、Derived Data、本地配置和系统元数据的临时纯源副本中，Debug 7 项测试与 Release 构建再次通过。
- Debug 应用通过 open 启动，进程保持运行；修复前发现并修正了动态 framework 未嵌入问题。
- 受 warnings-as-errors 保护的 Swift/C 编译没有代码警告。

## 未通过或未执行

### Sanitizer 下的 XCTest

Sanitizer 测试未通过，不能记为测试成功。测试代码执行前，xctest 在引导阶段中止：

~~~text
Early unexpected exit, operation never finished bootstrapping
__sanitizer::VerifyInterceptorsWorking()
libsystem_c.dylib: abort() called
~~~

崩溃报告显示 Xcode 测试运行器同时载入 sanitizer runtime 与 libMainThreadChecker。尝试使用本地
临时签名、在 Test action 和 Launch action 关闭 Main Thread Checker，均未阻止 Xcode 26.2
注入该运行时。为避免弱化普通测试诊断，scheme 已恢复默认行为。

这属于当前工具链/XCTest 引导限制，不是 sanitizer 发现了项目内存错误。Phase 2 已增加独立原生
Bridge ASan/UBSan harness 并验证通过；XCTest 引导限制仍按本节记录保留。

### GUI 人工验收

2026-08-01，操作者在解锁、登录的 Mac 图形会话中手动确认以下五项全部通过：

1. 主窗口内容和布局正常。
2. macOS 应用菜单中的 Settings 能打开设置窗口。
3. Open Remote Window 能打开空远程窗口。
4. 关闭远程窗口后可再次打开，主窗口不退出。
5. 设置窗口和远程窗口没有明显布局或焦点异常。

该结论来自操作者人工确认；本次文档更新未重新执行 GUI 自动化。

### 尚不适用于 Phase 1

- Windows/RDP 连接、证书、Keychain、Metal、输入、全屏和权限流程尚未实现，因此未测试。
- 最低 macOS 14 的真实或虚拟环境验收未执行；当前只在当前 macOS 构建验证。

## 修复过的验证问题

- PowerShell 补丁管道产生 CRLF，已统一为 LF，并让脚本使用物理路径避免共享盘别名产生陈旧输出。
- Bridge 测试头文件导入与静态库 Header Search Path 不一致，已修正。
- Swift 6 不接受无状态 enum 中多余的空初始化器，已删除。
- App 测试最初错误地把 NSApp.windows 立即移除作为关闭条件，已改为验证窗口管理器可见状态。
- 首次 GUI 启动发现 FarframeCore.framework 未嵌入，已增加 Embed Frameworks 和正确 rpath。

## 当前任务状态

- P1-01：已完成。
- P1-02：已完成。
- P1-03：已完成。
- P1-04：已完成。
- P1-05：已完成。

P1-02 的五项 GUI 检查已由操作者确认，Phase 1 整体完成。
