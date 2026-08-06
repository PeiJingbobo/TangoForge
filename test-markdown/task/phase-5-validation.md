# Phase 5 基础输入验证记录

## 1. 结论

P5-01 至 P5-06 已完成。P5-04 已按实际验收反馈改为“只传物理 scan code，由受控端处理
输入法”，自动化回归通过，并由操作者确认修复完成。2026-08-01，操作者在启动 Phase 6A
时明确确认 Phase 5 已完成，因此 P5-05、P5-06 的任务状态已关闭；本记录保留原有自动化与
人工证据边界，不把未逐项复述的观察写成新的测试结果。

当前代码已打通 AppKit 画布、Swift 输入状态、Bridge 有界队列和 FreeRDP 所有者线程之间的
完整链路。本记录不把命令行测试等同于 GUI、远端输入法或真实物理键盘验收。

## 2. 已实现范围

- RemoteCanvasView 成为 AppKit 第一响应者，处理物理键盘、鼠标、滚轮和跟踪区域事件。
- 物理键和控制键使用明确的 macOS virtual key 到 Windows scan code 映射；左右
  Command、Option、Control、Shift 分别保留，Command 在本阶段映射为 Windows 键。
- RemoteCanvas 不调用 interpretKeyEvents，也不实现 NSTextInputClient；macOS 本地输入法的
  marked/committed text 被忽略，字符键、Backspace 和控制键统一按 keyCode 映射为 scan code。
- Bridge 仍保留底层 Unicode API 作为协议能力，但 Farframe 应用层不再生成或分发 Unicode
  输入命令。
- 支持按键重复、Caps Lock/Num Lock 状态、鼠标左右中键和 X1/X2 扩展键、垂直与水平滚轮。
- 坐标转换覆盖等比适配、原始像素和 Retina backing scale；原始像素模式的渲染裁剪与输入
  坐标采用同一几何定义。
- 失去第一响应者、应用退到后台、窗口关闭、视图移除、取消和断开时，统一释放所有已按下
  的按键与按钮。
- Swift 不持有 FreeRDP 内部结构。Bridge 使用固定容量队列把输入交给 FreeRDP 所有者线程；
  相邻移动可合并，队列满时返回明确错误并触发释放屏障。
- Bridge ABI 升级为 5，公共接口记录了线程、所有权、长度和生命周期约束。

## 3. 自动化与构建证据

在项目指定的 Mac 构建环境执行：

~~~sh
/bin/sh scripts/test.sh
/bin/sh scripts/test-native-bridge.sh
/bin/sh scripts/test-rdp-integration.sh
xcodebuild -project FarframeRDP.xcodeproj \
  -scheme FarframeRDP \
  -configuration Release \
  -destination 'platform=macOS,arch=arm64' \
  -derivedDataPath .derivedData-release \
  CODE_SIGNING_ALLOWED=NO build
~~~

结果：

- scripts/test.sh：30 项测试全部通过，其中 FarframeCore 10 项、FarframeRDP 15 项、
  Bridge 5 项。
- Swift 测试覆盖代表性字母、数字、导航、F1–F12、左右修饰键、重复、锁定键、按钮配对、
  纯 scan code 输入、中文字符事件下的物理 B 键、Backspace、释放屏障、缩放与 Retina 坐标。
- Native Bridge 的 ASan/UBSan 测试通过，覆盖 512 项固定队列、相邻移动合并、队列满错误和
  释放屏障；未发现 sanitizer 错误。
- 真实 RDP 集成测试成功协商 NLA，发送锁定键同步、指针移动和释放命令，并安全消费 22 次
  画面更新。测试没有发送文本或危险快捷键。
- arm64 Release、禁用签名的命令行构建成功。

测试日志中的第三方目标参数提示以及 headless AppIntents 日志不影响测试结果；本次没有通过
修改签名、TLS、证书或权限配置绕过问题。

## 4. 人工验收状态

- P5-01、P5-02、P5-03 已由操作者确认完成。
- P5-04 已由操作者确认修复完成：Farframe 采用纯 scan code 策略，输入法由受控端处理。
- 2026-08-01，操作者明确确认 Phase 5 已完成，因此 P5-05、P5-06 已关闭。
- 本轮没有重新逐项复述 P5-05、P5-06 的观察过程；自动化证据与真实设备限制仍按下文记录。

## 5. 已知限制与后续边界

- Command-W 等语义快捷键策略属于 Phase 6A；Phase 5 只建立物理键和文本基础，不声明可以
  抑制所有 macOS 保留快捷键。
- 当前未启用全局 monitor 或 CGEventTap。紧急释放组合已在 Phase 6A 实现；增强捕获和
  权限拒绝降级属于 Phase 6B。
- 真实集成测试故意不输入文字，也不能从命令行证明远端指针实际落点、受控端输入法候选窗
  或屏幕键盘状态；这些项目仍需人工观察。
- 本地中文输入源到远端 scan code 的 B/Backspace 自动回归已通过，但受控端输入法组合过程、
  非美式物理键盘布局、扩展鼠标键和滚轮方向仍无真实设备验收结论。
- 原始像素模式在远端桌面大于窗口时采用居中裁剪，目前没有平移视口交互。
- 应用层不支持把 macOS 本地输入法已经提交的 Unicode 文本直接注入远端；需要在受控端
  配置并切换目标输入法。
