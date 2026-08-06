# 国际化与简体中文基线

## 1. 当前结论

Farframe RDP 已建立可扩展的国际化基础，当前开发语言和默认语言为简体中文
（`zh-Hans`）。项目发布前暂不维护其他语言，不创建内容不完整的占位翻译。

本次改造不改变 Phase 6A 的快捷键行为、输入状态机、FreeRDP Bridge、证书信任决策或
凭据边界。Phase 6A 原有人工验收状态保持不变。

## 2. 实现约定

- Xcode 工程的 `developmentRegion` 为 `zh-Hans`，`knownRegions` 当前只包含
  `zh-Hans` 和 `Base`。
- 应用字符串目录为
  `Sources/FarframeRDP/Localizable.xcstrings`，`sourceLanguage` 为 `zh-Hans`。
- SwiftUI 的静态界面文本使用本地化字符串字面量。
- AppKit、状态、错误和动态显示名称使用 `String(localized:)`。
- 品牌名、协议名、按键名和组合键（例如 Farframe RDP、TLS/NLA、Command、Ctrl-W）
  按产品或技术名称保留，不强行翻译。
- OSLog 的内部诊断事件不作为 UI 文案，不随界面语言翻译，便于稳定检索。
- Bridge 继续返回稳定的错误分类；应用层将错误分类映射为本地化、用户可读的消息。

## 3. 本次覆盖范围

- 连接表单、连接阶段、校验错误和临时密码说明。
- 证书验证标题、字段、警告和决策按钮。
- 远程窗口标题、渲染等待/降级状态。
- 缩放与键盘捕获工具栏、提示和辅助功能标签。
- 快捷键设置、范围、默认策略名称和 Phase 6B 提示。
- DNS、网络、TLS、证书、认证、协议、取消和 Bridge 内部错误。

## 4. 开发期维护规则

在项目发布前：

1. 新增用户可见字符串时，继续以简体中文作为源字符串。
2. SwiftUI 文案保持可由编译器提取；返回 `String` 或 AppKit 文案使用
   `String(localized:)`。
3. 完成相关代码后运行构建，使用 Xcode 的 `xcstringstool sync` 将编译器生成的
   `.stringsdata` 同步到 `Localizable.xcstrings`。
4. 不添加英文或其他语言的空翻译，也不为了“国际化”复制一套业务字符串常量。
5. 用户可见的原生 C/FreeRDP 错误必须先映射为稳定错误类别，再在 Swift 应用层本地化；
   不直接把底层英文说明展示给用户。

## 5. 发布前语言适配流程

发布候选阶段统一执行：

1. 冻结简体中文源文案和 String Catalog key。
2. 从 `Localizable.xcstrings` 增加目标语言，不复制或拆分 catalog。
3. 翻译时保留格式参数、产品名、快捷键和安全术语。
4. 分语言运行连接表单、证书对话框、设置窗口、远程工具栏和所有错误状态。
5. 检查文本截断、窗口尺寸、VoiceOver 朗读、复数/格式参数和从右到左布局（如适用）。
6. 每种发布语言必须有实际人工验收记录；没有验收的语言不得标记为已支持。

## 6. 自动化与构建证据

2026-08-01 在项目指定的 Mac 构建环境执行：

~~~sh
xcodebuild -list -project FarframeRDP.xcodeproj
/bin/sh scripts/test.sh
FARFRAME_CONFIGURATION=Release \
  FARFRAME_DERIVED_DATA_PATH=.derivedData-release \
  /bin/sh scripts/build.sh
~~~

结果：

- Xcode 识别现有 6 个 targets 和 `FarframeRDP` scheme。
- Debug 完整测试 37 项全部通过：FarframeCore 10、FarframeRDP App 22、
  FarframeRDPBridge 5；0 失败、0 跳过。
- 新增测试确认 App Bundle 的开发语言为 `zh-Hans`、简体中文本地化可解析，并验证
  快捷键策略与范围的中文默认显示名称。
- arm64、macOS 14、`CODE_SIGNING_ALLOWED=NO` 的 Release 构建成功。
- Xcode 官方 `xcstringstool` 可编译并列出当前 String Catalog 条目。
- 构建产物的 `CFBundleDevelopmentRegion` 为 `zh-Hans`。

首次测试曾因补丁生成的 6 处字符串插值多余转义而在编译阶段失败；修正后重新运行完整
测试并通过，未将首次失败计为成功。

## 7. 尚未执行的人工验收

- 未在解锁的 Mac 图形会话中逐页确认中文文案、截断、工具提示和辅助功能朗读。
- 未重新执行 Phase 6A 的真实 Windows 快捷键人工验收；本次没有修改快捷键路由行为。
- 其他语言尚未添加、翻译或测试，这是当前明确范围，不是已支持能力。

