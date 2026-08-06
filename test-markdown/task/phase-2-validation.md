# Phase 2 验证记录

- 日期：2026-07-31
- 范围：P2-01 至 P2-06
- 结论：FreeRDP 原生依赖与窄 Bridge 已实现，自动化和 Release 审计通过

## 已实现

- 固定 FreeRDP/WinPR 3.30.0 与 OpenSSL 3.5.7 的不可变提交。
- 面向 macOS arm64、最低 macOS 14.0 的确定性静态依赖构建脚本。
- FreeRDP、WinPR、OpenSSL 四个实际归档合并为项目私有聚合静态库。
- Xcode App 与 Bridge 测试目标链接项目内静态产物，不依赖 Homebrew 运行时。
- Bridge 公共 Clang module、不透明会话句柄、ABI/版本 API。
- FreeRDP instance/context 创建与精确一次销毁。
- owner-thread 约束、同步事件回调、任意线程原子取消请求和脱敏结果文本。
- Swift 启动路径读取并记录 FreeRDP 版本。
- 第三方组件清单、能力开关和 Apache-2.0 许可原文。

依赖选择和能力边界见
[Phase 2 原生依赖与 Bridge 决策](phase-2-native-dependency-decision.md)。

## 通过的验证

### Debug Xcode 测试

~~~sh
FARFRAME_CMAKE=/path/to/cmake \
FARFRAME_NINJA=/path/to/ninja \
/bin/sh scripts/test.sh
~~~

结果：11 项测试，0 失败。

- FarframeCoreTests：4 项。
- FarframeRDPTests：3 项，其中 Bridge 返回 FreeRDP 3.30.0。
- FarframeRDPBridgeTests：4 项，其中 250 次会话创建/销毁循环、双重销毁 no-op、
  回调生命周期、取消和无效参数均通过。
- 测试宿主启动日志确认 `FreeRDP 3.30.0 loaded`。

### 原生 sanitizer harness

~~~sh
FARFRAME_CMAKE=/path/to/cmake \
FARFRAME_NINJA=/path/to/ninja \
/bin/sh scripts/test-native-bridge.sh
~~~

结果：ASan 与 UBSan 通过。harness 验证创建/销毁、取消、live-session 计数，
并验证非 owner 线程销毁被拒绝、随后由 owner 线程正常释放。

本机 Xcode clang 17 的 ASan runtime 在空程序也会初始化挂起，因此脚本优先使用可用的
Command Line Tools clang。该 clang 与 Xcode 构建的静态对象链接时会输出不影响结果的
`+zcm/+zcz` target-feature 提示；没有 sanitizer 报告。

### Release 构建与链接审计

~~~sh
FARFRAME_CONFIGURATION=Release \
FARFRAME_CMAKE=/path/to/cmake \
FARFRAME_NINJA=/path/to/ninja \
/bin/sh scripts/build.sh

otool -L .derivedData/Build/Products/Release/FarframeRDP.app/Contents/MacOS/FarframeRDP
nm -gU .derivedData/Build/Products/Release/FarframeRDP.app/Contents/MacOS/FarframeRDP
~~~

结果：

- Release 构建成功，主程序为 arm64 Mach-O。
- 主程序包含 `_freerdp_get_version_string`，证明 FreeRDP 实际进入最终链接。
- 动态依赖只有 macOS 系统库和 `@rpath/FarframeCore.framework`。
- 应用包扫描未发现 `/opt/homebrew`、`/usr/local/Cellar` 或 `/usr/local/opt`。
- 聚合原生依赖库为 arm64。
- 产物清单与锁定值一致：FreeRDP 3.30.0、OpenSSL 3.5.7、deployment target 14.0。
- CMakeCache 证实 client/server/proxy/shadow/sample/channels/FFmpeg/OpenH264 等均关闭，
  OpenSSL 开启。

### 许可校验

固定提交中的上游许可文件已复制到 `third-party/licenses/` 并用 `cmp` 验证相同。
SHA-256：

- FreeRDP：`cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30`
- OpenSSL：`7d5450cb2d142651b8afa315b5f238efc805dad827d91ba367d8516bc9d49e7a`

## 未执行与边界

- 未连接真实 Windows 主机；连接、TLS/NLA、证书和认证属于 Phase 3。
- 未在 macOS 14 真机或虚拟机运行；当前只验证 deployment target 和当前 arm64 Mac。
- Phase 1 的设置/远程窗口人工目视验收已于 2026-08-01 由操作者手动确认完成。
- 未验证 x86_64/Universal；当前不声明支持。
- Channels、音频、设备重定向、OpenH264/FFmpeg 尚未启用或测试。

## 残余风险

- 原生源码下载需要网络，并要求可用的 Git、CMake、Ninja、Xcode CLI 和 Perl/Make。
  脚本支持通过 `FARFRAME_CMAKE`、`FARFRAME_NINJA` 显式指定工具，但不锁定这些构建工具的二进制。
- 聚合静态库会随 Phase 3 连接能力扩大；每次开启 channel/codec 都必须重新做许可证、体积、
  安全和实际链接审计。
- 取消目前只是线程安全标志；真正中止 FreeRDP 连接循环是 Phase 3 的验收项。
- Debug 测试出现的 AppIntents/linkd 日志来自无头测试环境，测试仍全部通过，不涉及 Bridge。

## 当前任务状态

- P2-01：已完成。
- P2-02：已完成。
- P2-03：已完成。
- P2-04：已完成。
- P2-05：已完成。
- P2-06：已完成。
