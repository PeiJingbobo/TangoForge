# Phase 2 原生依赖与 Bridge 决策

- 日期：2026-07-31
- 范围：P2-01 至 P2-06
- 状态：已采用

## 决策摘要

Farframe RDP 的首个原生协议层固定为 FreeRDP 3.30.0，并使用该标签指向的不可变提交
`6b107f0aadbabc47941c5a5b893b88c01792af6d`。WinPR 与 FreeRDP 来自同一源码树和提交。
TLS 实现固定为 OpenSSL 3.5.7 LTS，提交
`8cf17aaeb4599f8af87fefd810b5b5fee90fe69e`。

这些值集中保存在 `third-party/versions.sh`。构建脚本只接受精确提交，不跟随分支或可移动标签。
首发产物仅支持 macOS arm64，最低部署版本为 macOS 14.0。

## 实际链接组件

| 组件 | 版本或提交 | 进入产物 | 许可证 | 用途 |
|---|---|---|---|---|
| FreeRDP | 3.30.0 / `6b107f0a...` | `libfreerdp3.a` | Apache-2.0 | RDP 协议实现 |
| WinPR | 与 FreeRDP 同提交 | `libwinpr3.a` | Apache-2.0 | FreeRDP 平台抽象 |
| OpenSSL | 3.5.7 / `8cf17aae...` | `libssl.a`、`libcrypto.a` | Apache-2.0 | TLS 与密码算法 |

许可原文位于 `third-party/licenses/`，内容已与固定提交中的上游文件逐字比较。

## 构建与能力范围

`scripts/build-native-dependencies.sh` 使用 CMake、Ninja 和 Xcode 工具链生成 Release 静态库，
再由 Apple `libtool` 合并为私有的
`third-party/artifacts/macos-arm64/lib/libFarframeRDPDependencies.a`。
源码、构建目录和产物均为 Git 忽略项；版本、提交和构建配方写入产物清单。

当前明确启用：

- FreeRDP/WinPR 核心静态库；
- OpenSSL 静态 TLS 实现；
- arm64 与 macOS 14.0 deployment target。

当前明确关闭：

- FreeRDP 自带客户端、服务端、代理、shadow、示例和工具；
- 动态库与测试/benchmark 安装产物；
- channels；
- OpenH264、FFmpeg、swscale、JPEG；
- CUPS、PCSC、FUSE 和 Mac 音频。

Channels 和可选编解码器会在对应功能进入真实集成测试时单独评审、启用并更新兼容矩阵。
上游具备能力不代表 Farframe 已支持。

## Bridge 边界

`FarframeRDPBridge` 的公共头只暴露 Farframe 类型和不透明 `FFRSession`，不暴露或允许
Swift 持有 FreeRDP struct。Bridge：

- 创建并唯一拥有 `freerdp*` 及其 context；
- 按 context、instance、Bridge allocation 的顺序精确释放一次；
- 把创建、回调变更和销毁限制在创建线程；
- 允许任意线程原子请求取消；
- 同步回调只借用 session、event 和 user context，禁止重入和回调内销毁；
- 提供 ABI、FreeRDP 版本、构建 revision 和脱敏结果描述。

当前取消接口只记录原子标志。Phase 3 的连接循环负责消费标志并触发 FreeRDP abort；
Phase 2 不伪造“已连接”路径。

## 安全与更新策略

- 不从系统或 Homebrew 动态加载 FreeRDP/OpenSSL。
- 不关闭 TLS/NLA 或证书验证来通过构建或测试。
- 新版本进入项目前，必须核对上游安全公告、许可证、体积和能力变化，更新精确提交，
  从空原生产物重新构建并重跑 Bridge sanitizer、Xcode 测试和 Release 链接审计。
- OpenSSL 或 FreeRDP 的安全修复发布时优先触发上述更新，不等待常规功能迭代。

## 重新评估条件

出现以下任一情况时重新评估本决策：

- 需要 Intel Mac 或 Universal 产物；
- Phase 3 需要当前已关闭的 RDP channel；
- Phase 5/8 需要音频、设备重定向或可选视频编解码器；
- 静态聚合库导致符号冲突、体积不可接受或许可证分发要求变化；
- OpenSSL 3.5 LTS 或 FreeRDP 3.30 系列出现影响当前能力集的安全问题。
