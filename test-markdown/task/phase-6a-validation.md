# Phase 6A 应用内快捷键验证记录

## 1. 当前结论

Phase 6A 的代码与自动化验证已完成，P6A-01 已完成。2026-08-01 的首次真实 Windows 人工
测试发现：按下 Command 时远端先收到 Windows 键，随后 Command-C/V 未执行预期 Ctrl
组合；随后还发现远程画布销毁时遗留的本地事件监控器会吞掉首页输入框的键盘事件。两项
事件生命周期缺陷均已修复并通过自动化与 Release 构建验证。组合键映射已由用户在真实
Windows 会话复测通过，P6A-02 标记为已完成；P6A-03 至 P6A-05 仍按各自范围等待验收。
本记录不把命令行测试等同于菜单抑制、远端按键效果或真实物理键盘验收。

## 2. 已实现范围

- 建立 `ShortcutPolicy`、Mac/Windows chord、窗口/全屏范围和默认策略。
- `Command-C/V/X/A/Z/S/F/P/N/T/W` 默认映射为对应 Windows `Ctrl` 组合。
- `Shift-Command-Z` 默认映射为 `Ctrl-Y`。
- 远程画布聚焦时默认拦截 `Command-Q`，只阻止本地退出，不发送危险的远端关闭动作。
- `Command-Tab`、`Command-Space` 和 `Control-方向键` 已在设置中标明需要 Phase 6B
  增强捕获，当前保持禁用，不申请辅助功能或输入监控权限。
- Command 修饰键采用延迟判定：按下时不立即向 Windows 发送 Win-down；若下一键匹配已启用
  的语义策略，只发送成对的 Ctrl 组合；若直接松开 Command，则在松开时发送完整 Windows
  键点击；若下一键未映射，则先补发 Win-down，再走物理按键路径。生命周期出口会清理延迟
  状态并吞掉对应本地 key-up，避免开始菜单抢焦点或 Windows 键残留。
- AppKit 画布同时使用 responder chain、`performKeyEquivalent` 和窗口限定的
  `NSEvent` local monitor；不使用 global monitor 或 `CGEventTap`。
- local monitor 只处理当前远程窗口且第一响应者为远程画布的事件，关闭窗口时显式、幂等移除。
- `Control-Option-Command-Escape` 是不可配置的紧急释放组合：释放所有远端输入并停止
  键盘捕获，需点击工具栏键盘按钮恢复。
- 工具栏键盘状态：灰色表示画布未聚焦，蓝色表示基础捕获，黄色表示捕获已释放。
- 设置窗口支持逐项开关、窗口/全屏范围和恢复默认值；增强项明确禁用。

## 3. 自动化与构建证据

在项目指定的 Mac 构建环境执行：

~~~sh
xcodebuild -list -project FarframeRDP.xcodeproj
/bin/sh scripts/test.sh
FARFRAME_CONFIGURATION=Release \
  FARFRAME_DERIVED_DATA_PATH=.derivedData-release \
  /bin/sh scripts/build.sh
~~~

结果：

- 工程探测确认 `FarframeRDP` scheme 和现有六个 targets。
- `scripts/test.sh`：41 项测试全部通过，0 失败、0 跳过。
- 其中 FarframeRDP App 测试 26 项，新增覆盖默认策略唯一性、语义优先级、窗口/全屏范围、
  禁用策略、增强策略不激活、Command-W 不发送 Windows 键、单独 Command 在松开时发送
  Windows 键点击、未映射 Command 组合保持物理路径、漏收 key-up 后恢复、画布移除后监控清理、紧急释放和状态变化。
- arm64 Release、macOS 14 deployment target、`CODE_SIGNING_ALLOWED=NO` 构建成功。
- 本阶段未改动 C/Objective-C Bridge 或 FreeRDP 原生内存代码，因此未重复运行 native
  ASan/UBSan harness。
- 未运行真实 RDP 快捷键集成自动化；现有集成脚本不会观察 Windows UI，也不应自动发送
  可能关闭窗口、打印或退出应用的组合键。

## 4. 人工验收步骤

### 4.1 准备

1. 完全退出此前运行的 Farframe RDP 进程，再在已解锁、已登录的 Mac 图形会话中通过 Xcode
   运行新构建。
2. 在连接前逐个点击首页的主机、端口、用户名、域和密码输入框，确认均能输入、删除和切换焦点。
3. 准备一个允许测试的 Windows RDP 会话；不要在含未保存工作或生产任务的会话中测试。
4. Windows 打开屏幕键盘，用于观察 Ctrl、Alt、Shift 和 Windows 键是否释放。
5. Windows 打开记事本和至少含两个标签页的浏览器。

### 4.2 设置界面

1. 打开 Farframe 的 Settings，进入 `Keyboard & Shortcuts`。
2. 确认常用 Command 项默认开启，映射文字与预期一致。
3. 确认每项可选择 `Window only`、`Full screen only` 或 `Window and full screen`。
4. 确认 Command-Tab、Command-Space、Control-方向键显示需要 Phase 6B，且无法在本阶段开启。
5. 随机更改两个开关和范围，点击 `Restore Shortcut Defaults`，确认恢复。

### 4.3 聚焦与状态

1. 连接 Windows，确认远程窗口打开且画布聚焦时工具栏键盘图标为蓝色。
2. 点击窗口工具栏或把焦点切到 Farframe 设置/连接表单，确认图标变灰，Command 快捷键按
   macOS 本地行为执行。
3. 再点击远程画布，确认图标恢复蓝色。

### 4.4 常用语义快捷键

在 Windows 记事本中输入一段可丢弃文本，逐项测试：

1. 先按住 Command 至少 1 秒但不要按其他键：Windows 开始菜单不得展开；松开 Command 后，
   开始菜单应展开。按 Escape 关闭开始菜单，再继续测试。
2. 分别以正常速度和故意停顿 1 秒的速度按 `Command-C`、`Command-V`：按住 Command
   期间和组合键完成后开始菜单都不得展开，Windows 记事本必须完成复制、粘贴。
3. `Command-A/C/X/V/Z`：Windows 分别执行全选、复制、剪切、粘贴、撤销；Farframe/macOS
   菜单不执行对应本地动作。
4. `Shift-Command-Z`：Windows 执行 `Ctrl-Y` 重做。
5. `Command-S/F/P/N`：Windows 分别出现保存、查找、打印、新建行为；出现对话框后取消，
   避免写文件或打印。
6. 在 Windows 浏览器测试 `Command-T` 和 `Command-W`：新增/关闭的是远端标签页，
   Farframe 远程窗口不关闭。
7. 远程画布聚焦时按 `Command-Q`：Farframe 不退出，Windows 也不应收到关闭动作。
8. 每次操作后观察 Windows 屏幕键盘，确认 Ctrl、Shift、Alt、Windows 键均未保持按下。

### 4.5 开关与范围

1. 在设置中关闭 `Close Remote Tab or Window`，返回远程画布按 `Command-W`；确认恢复本地
   关闭 Farframe 远程窗口的行为。然后重新连接。
2. 将该项设为 `Full screen only`。窗口模式按 `Command-W` 应执行本地关闭；重新连接并进入
   全屏后按 `Command-W` 应只向 Windows 发送 `Ctrl-W`。
3. 将该项设为 `Window only`，验证结论相反。
4. 恢复默认值并重新连接，避免后续测试继承临时范围。

### 4.6 紧急释放与恢复

1. 画布聚焦时按 `Control-Option-Command-Escape`。
2. 确认工具栏键盘图标变黄，Windows 屏幕键盘没有残留修饰键。
3. 继续输入普通字符，确认不会送到 Windows。
4. 点击黄色键盘图标，确认图标变蓝且输入恢复。
5. 该组合必须始终有效，设置中不应存在关闭或改写它的入口。

### 4.7 生命周期与卡键

分别按住 Command、Control、Option、Shift、普通字符键和一个鼠标按钮，在按住期间逐项执行：

1. 点击到其他 Farframe 控件使画布失焦。
2. 切换到其他 macOS 应用。
3. 关闭远程窗口。
4. 主动断开连接。
5. 在允许的测试环境中制造突然断线。

每次重新连接或观察 Windows 屏幕键盘，确认没有卡住的键或按钮；离开画布后 macOS 快捷键
立即恢复。突然断线若本轮无法安全制造，应明确记录为未测试。

## 5. 人工结果记录

| 场景 | 结果 | 备注 |
|---|---|---|
| 首页五个连接输入框 | 未通过，待复测 | 初测时键盘事件被已销毁画布的遗留监控器吞掉；监控生命周期已修复 |
| 设置默认值、恢复默认值和增强项说明 | 未执行 | |
| 灰/蓝/黄状态与焦点一致 | 未执行 | |
| Command-C/V/X/A/Z/S/F/P/N/T | 已通过 | 用户确认组合键映射工作正常 |
| Shift-Command-Z | 未执行 | |
| Command-W 开启/关闭 | 未执行 | |
| 窗口/全屏范围 | 未执行 | |
| Command-Q 本地退出保护 | 未执行 | |
| 紧急释放与工具栏恢复 | 未执行 | |
| 失焦、切后台、关窗、主动断开无卡键 | 未执行 | |
| 突然断线无卡键 | 未执行 | 可单独延期并说明原因 |

## 6. 已知限制

- Phase 6A 只实现应用内捕获，不请求辅助功能/输入监控权限，也不使用 `CGEventTap`。
- Command-Tab、Command-Space、Mission Control/Spaces 等系统快捷键不能在本阶段承诺拦截。
- 快捷键设置当前为应用运行期全局设置；与 Profile 关联和持久化属于 Phase 7。
- 自动化测试验证发送序列和本地拦截决策，但真实 Windows 应用对组合键的响应仍需人工观察。
