# Phase 4：远程画面验证记录

更新日期：2026-08-01

## 结论

Phase 4 的协议画面接收、边界校验、有界帧邮箱、Metal 呈现、本地缩放、原生全屏、
远程光标和 CoreGraphics 诊断回退已经实现。自动化测试、原生 sanitizer、真实 RDP
画面事件消费和 arm64 Release 构建均通过。

P4-03 已于 2026-08-01 由操作员确认人工验收完成。Retina/非 Retina、全屏观感仍归入
P4-04；光标热点和高变化画面的长期性能分别归入 P4-06、P4-07，并按当前安排稍后
统一测试。在取得对应记录前，仍不能宣称这些待测项最终通过。

## 已实现范围

- Bridge 在 FreeRDP `BeginPaint`、`EndPaint`、`DesktopResize` 和 pointer callbacks
  接收桌面尺寸、BGRA 帧、脏矩形及光标事件。
- 所有服务器提供的尺寸、stride、矩形、buffer 长度、光标尺寸和热点均先验证；桌面
  单边上限为 16384 像素，帧缓冲上限为 256 MiB，尺寸变化在分配前拒绝超限输入。
- Bridge 回调中的像素只在回调期间借用，Swift worker 在回调返回前复制所需行，避免
  Swift 持有或修改 FreeRDP 内部指针。
- `RemoteFrameMailbox` 维护单个完整 CPU 后备帧，只保留一个待通知信号并合并脏矩形；
  主线程落后时不会无限堆积帧对象。
- `CAMetalLayer` 使用 BGRA 纹理执行脏区域上传；同一时刻最多一个 GPU command buffer
  在途，后续更新合并，输入和主线程不会等待每一帧完成。
- 远程窗口提供“适应窗口”和“1:1”两种模式；Retina drawable 按 backing scale 更新。
  1:1 模式在远程桌面大于窗口时居中裁切，目前不含滚动/平移。
- 窗口启用 macOS 原生全屏；全屏进出沿用同一画布和缩放策略。
- 远程光标形状转换为 BGRA `NSCursor`，保留热点，并处理默认/隐藏状态；位置事件已进入
  状态链路，但不会主动扭曲本机系统指针。
- 设置 `FARFRAME_RENDERER=coregraphics` 可启用 CoreGraphics 诊断回退；无 Metal 设备时
  也会自动回退。该路径用于排查颜色、方向或 Metal 可用性，不是第二套主渲染架构。

## 自动化验证

### Xcode 测试

```sh
/bin/sh scripts/test.sh
```

结果：Debug 测试通过，共 23 项、0 失败：

- FarframeCoreTests：10 项；
- FarframeRDPTests：8 项；
- FarframeRDPBridgeTests：5 项。

覆盖帧几何与字节数拒绝、脏矩形合并与背压、画布状态、光标校验、CoreGraphics
回退、全屏/缩放入口、Bridge 所有权、取消和回调生命周期。测试进程出现的
`com.apple.linkd.autoShortcut` 日志来自无签名 headless 测试环境，不影响测试结果。

### 原生 sanitizer

```sh
/bin/sh scripts/test-native-bridge.sh
```

结果：AddressSanitizer/UndefinedBehaviorSanitizer 通过。输出中的 `+zcm/+zcz not
recognized` 是当前 clang 处理第三方 arm64 对象特性时的已知噪声，本次未发现
越界、UAF、重复释放或未定义行为。

### 真实 RDP 自动集成

```sh
/bin/sh scripts/test-rdp-integration.sh
```

结果：使用本地未跟踪配置完成 NLA 协商，并安全消费 15 次远程画面更新；未发现无效
尺寸、stride、脏矩形或缓冲区长度。输出不包含真实端点、用户名或凭据。

该检查证明协议回调和事件消费可工作，不等同于人工确认颜色、方向、光标和视觉流畅度。

### Release 构建

```sh
xcodebuild -project FarframeRDP.xcodeproj -scheme FarframeRDP \
  -configuration Release -destination 'platform=macOS,arch=arm64' \
  -derivedDataPath .derivedData-release CODE_SIGNING_ALLOWED=NO build
```

结果：arm64、macOS deployment target 14.0、无签名 Release 构建成功。

说明：当前 `scripts/build.sh` 使用 Debug 配置，虽然本次也执行成功，但不能作为 Release
证据；上面的显式命令才是本阶段 Release 验证。

## 动态分辨率决策

当前固定的 FreeRDP 3.30.0 依赖以 `WITH_CHANNELS=OFF` 构建，因此没有 `drdynvc/disp`
动态分辨率通道。为了只完成 Phase 4 而临时打开全部通道，会扩大二进制、协议与安全审计
范围，并与 P8 的通道治理重复。

因此本阶段完成本地缩放和原生全屏，动态分辨率协商转入 P8-01：在列出最小必需静态通道、
重新执行许可证/链接/攻击面审计后，再实现 Display Control Capability 和尺寸请求。
全屏在当前版本不会通知远端改变桌面分辨率。

## 人工验收记录

- P4-03：2026-08-01 由操作员确认验收完成。

## 后续统一人工测试清单

- Retina 内屏与外接非 Retina 显示器分别检查适应窗口、1:1、窗口缩放和跨屏移动。
- 反复进入/退出全屏，确认画面比例、焦点、窗口恢复和断开行为。
- 检查箭头、文本、调整大小、链接、隐藏等光标，重点确认热点和可见性。
- 快速滚动、拖动窗口和播放视频，记录 CPU/GPU、帧延迟与内存曲线；确认长时间无持续增长。
- 通过 CoreGraphics 诊断回退对照颜色与方向；回退只用于诊断，不作为性能目标。

## 当前状态与残余风险

- P4-01、P4-02：已完成，自动化和真实画面事件验证通过。
- P4-03：已完成，操作员已确认人工验收通过。
- P4-04：实现完成，等待统一人工验收。
- P4-05：原生全屏和本地缩放完成；动态分辨率转入 P8-01。
- P4-06、P4-07：实现已就绪，按安排稍后完成统一人工测试。
- 真实集成只覆盖一个未记录版本的 Windows 主机和 15 次帧更新，不能据此扩展 Windows
  版本兼容声明。
- 1:1 大画面尚无平移；远程 cursor position 不移动本机系统光标；高变化场景没有形成
  可量化基线。这些均需在人工批次记录后再关闭对应任务。
