# Phase 7 Profile 与一键连接验证记录

## 1. 当前结论

Phase 7 已完成。Profile 数据与 Keychain 凭据分离；一键连接、证书指纹决策、可达性提示
以及删除时的部分失败处理已经接入现有连接状态机。自动化共执行 51 项测试，0 失败、0 跳过，
原生 Bridge sanitizer 验证和 Release 构建均已通过。

2026-08-01，用户已在所需的 Mac 图形会话和真实 Windows 环境中完成 Phase 7 全部人工
验收，包括新版界面与辅助功能、错误认证、首次成功保存、重启后一键连接、Keychain 异常、
证书决策、凭据/Profile 删除及失败恢复、可达性设置与状态显示；暂未发现问题。因此
P7-01 至 P7-06 全部标记为已完成。RD Gateway 的真实配置、认证和 Gateway 感知探测仍按
原计划属于 Phase 9，不构成 Phase 7 未完成项。

## 2. 已实现范围

### 2.1 Profile 与持久化

- 使用 SwiftData 保存稳定 UUID、显示名称、主机、端口、用户名、域、桌面/重定向/快捷键策略、
  最近成功时间、警告和 Profile 级证书指纹引用。
- 数据模型不含密码、token、hash、private key 或其他可复用秘密。
- 新 Profile 只有在真实会话进入 `connected` 后才写入数据库；失败或取消不会留下无效记录。
- 编辑显示名称或主机不会改变 UUID，因此不会因可变名称破坏 Keychain 关联；更改端点会清除
  旧的 Profile 级证书信任记录。
- 使用版本化 SwiftData schema 和 migration plan，避免后续模型演进依赖隐式推断。

### 2.2 Keychain 与凭据生命周期

- 使用 Security.framework generic-password item；service 固定为
  `com.farframe.rdp.credentials`，account 使用稳定 Profile UUID。
- 查询、读取、保存、更新和删除均在非主线程执行；UI 只接收结果状态。
- 处理缺失、重复、锁定/不允许交互、用户拒绝、损坏数据和其他 OSStatus 错误。
- 密码只存在于安全输入控件、短生命周期连接请求和 Keychain；不写 SwiftData、UserDefaults
  或日志。
- “保存密码”和“信任证书”是两个独立决策。认证成功后才保存用户选择保留的密码。
- 删除 Profile 先删除 Keychain 项；失败时保留 Profile 并报告可重试错误，避免数据库已经删除
  而秘密仍残留。成功后同步删除 Profile 内的信任记录。

### 2.3 一键连接与安全证书决策

- 侧边栏单击已保存 Profile 即启动连接；存在 Keychain 凭据时无需再次输入。
- 凭据缺失、Keychain 锁定或访问被拒绝时，显示安全输入表单，不禁用真实连接尝试。
- 每次会话使用独立的临时 FreeRDP certificate store 路径，避免全局 FreeRDP `known_hosts`
  绕过 Swift/Profile 的显式证书决策。
- Profile 指纹匹配时只接受当前会话；指纹变化时明确警告，用户必须再次决定。UI 不向
  FreeRDP 请求静默持久化证书。
- 会话 identity、取消和旧回调隔离继续由现有 `SessionCoordinator` 保证。

### 2.4 可达性提示

- 使用 Network.framework 做有超时和取消的 TCP 可达性提示，可由用户关闭自动检查。
- 状态显示为“在线”“暂时不可达”“最近连接成功”等提示，但不宣称已认证。
- 在线圆点使用 macOS 系统绿色，不可达指示图标使用窗口交通灯风格的系统黄色。
- 探测失败不会禁用连接按钮，也不会阻止真实 FreeRDP 连接。
- Phase 9 引入 RD Gateway 前只能检查直接目标；配置了 Gateway 后的双层探测策略仍属于
- “自动检查可达性”已从首页侧边栏移入软件 Settings 的“网络与状态”区域。
- 开关关闭时会取消现有刷新、清空可达性状态，不调用主机探测服务；列表状态图标、详情页
  状态标签和在线状态说明文字同时隐藏。凭据状态刷新仍可正常执行。
- 开关开启时，当前主机的状态标签直接显示在详情页主机地址下方。
  Phase 9 集成范围。

### 2.5 新版 macOS 界面

- 主窗口使用原生 `NavigationSplitView`：侧边栏负责 Profile 列表和状态，详情区负责连接、
  编辑和安全提示。
- 工具栏提供新增与刷新；删除、更新密码和清除密码放在 Profile 的上下文菜单/详情操作中。
- macOS 26 使用原生 SwiftUI glass effect；macOS 14–15 使用 `regularMaterial` 回退，保持
  可读性和 deployment target。
- 使用系统字体、SF Symbols、语义颜色和原生控件；未用自绘按钮模拟 AppKit 行为。
- 窗口默认尺寸调整为 980×680，在分栏、表单和连接状态之间保留清晰层级。

界面实现依据 Apple 当前的 macOS、Windows、Sidebars、Material 和 Glass Effect 设计/API
说明。最终图形验收仍以真实 App 在 Mac 上的显示、键盘导航、增加对比度和减少透明度设置为准。

## 3. 自动化与构建证据

在项目指定的 Mac 构建环境执行：

~~~sh
/bin/sh scripts/test.sh
/bin/sh scripts/test-native-bridge.sh
FARFRAME_CONFIGURATION=Release \
  FARFRAME_DERIVED_DATA_PATH=.derivedData-phase7-release \
  /bin/sh scripts/build.sh
~~~

结果：

- `scripts/test.sh`：51 项测试全部通过，0 失败、0 跳过。
- FarframeCore 10 项、FarframeRDP App 36 项、Bridge 5 项。
- Phase 7 共 10 项测试，覆盖 Profile 校验/规范化、SwiftData 内存库往返、稳定 UUID、
  Keychain 抽象 CRUD/错误注入、test-scoped 真实 Keychain CRUD、非法目标不发起网络探测。
- 新增两项设置行为测试：关闭自动检查会清空状态且不会新增探测调用；开启自动检查会发布
  探测结果。
- 新增状态名称测试，固定“在线”“暂时不可达”和“最近连接成功”的用户可见语义。
- test-scoped Keychain service 使用随机 UUID，测试结束删除项目；测试值运行期随机生成，
  不在源码或命令参数中保存固定凭据。
- 原生 Bridge ASan/UBSan harness 通过，包含新增 certificate store path 的长度、所有权和
  ABI 6 边界。工具链仍输出已知的 `+zcz`/`+zcm` linker 选项提示，不影响 harness 结果。
- arm64、macOS 14 deployment target、`CODE_SIGNING_ALLOWED=NO` 的 Release 构建成功。
- headless XCTest 出现 `linkd.autoShortcut` 和 DetachedSignatures 日志提示，但测试未失败。

## 4. 人工验收步骤

### 4.1 界面与辅助功能

1. 在已解锁、已登录的 Mac 图形会话中运行 Debug 或签名后的应用。
2. 检查空状态、侧边栏、详情卡片、新建/编辑表单、密码表单和警告窗口的层级与间距。
3. 分别在浅色/深色模式下检查文字、分隔线、焦点环和玻璃背景可读性。
4. 开启“增加对比度”和“减少透明度”，确认系统材质回退后仍可辨认和操作。
5. 只用键盘完成新增、编辑、取消、保存、连接和菜单操作；用 VoiceOver 检查关键控件名称。
6. 缩小窗口至合理最小尺寸，再放大/全屏，确认侧边栏和表单不遮挡主要操作。

### 4.2 首次成功与失败边界

1. 新建一个经授权的 Windows 目标，输入错误凭据并选择保存；确认认证失败后不出现 Profile，
   Keychain 中也没有该 Profile 项。
2. 使用正确凭据连接成功，分别验证“不保存密码”和“保存密码”。
3. 确认保存密码与证书信任是独立选择；选择一个不会自动勾选或执行另一个。
4. 首次连接自签名证书时拒绝，确认不会保存 Profile 信任；再次连接并仅接受当前会话，确认
   应用不会静默写入全局 FreeRDP 信任。

### 4.3 重启后一键连接

1. 成功保存 Profile 和密码后退出并重新启动应用。
2. 单击侧边栏 Profile，确认直接进入认证/连接流程，无需重输密码。
3. 在 macOS Keychain Access 中锁定相关 Keychain 或拒绝访问，再单击 Profile；确认显示明确
   的安全输入/错误路径且 UI 不冻结。
4. 删除 Keychain 项后再连接，确认提示输入而不是无限重试。
5. 修改 Profile 显示名称，确认原密码仍按 UUID 找到；修改主机，确认旧证书信任被清除。

### 4.4 更新、删除和部分失败

1. 更新保存密码后重启应用，以新密码连接；旧密码不得继续生效。
2. 清除保存密码，确认 Profile 保留且下一次连接提示输入。
3. 删除 Profile，确认 Profile、Keychain 项和 Profile 级证书指纹全部消失。
4. 在 Keychain 拒绝删除的条件下删除 Profile，确认 Profile 保留、错误明确且可以重试。

### 4.5 可达性提示

1. 对可达目标刷新，确认只显示“可能在线”；实际登录成功后才显示最近成功信息。
2. 对不可达目标刷新，确认显示“暂时不可达”，但仍可以点击连接并看到真实连接错误。
3. 关闭自动检查并重启应用，确认不会自动探测；手动刷新仍可用。
4. Gateway 感知探测在 Phase 9 配置存在后补验，本阶段不伪造 Gateway 支持结论。

## 5. 人工结果记录

| 场景 | 结果 | 备注 |
|---|---|---|
| 页面基本人工操作 | 已通过 | 用户于 2026-08-01 确认未发现问题 |
| 可达性设置迁移、关闭后完全隐藏、地址下方状态及交通灯配色 | 已通过 | 用户确认暂未发现问题 |
| 新版界面浅色/深色与窗口缩放 | 已通过 | 用户完成 Phase 7 人工验收 |
| 增加对比度、减少透明度、键盘导航、VoiceOver | 已通过 | 用户完成 Phase 7 人工验收 |
| 错误认证不保存 Profile/密码 | 已通过 | 用户完成真实 Windows 验收 |
| 首次成功后保存策略 | 已通过 | 用户完成真实 Windows 验收 |
| 重启后一键连接 | 已通过 | 用户完成真实 Windows 验收 |
| Keychain 锁定、拒绝、缺失与系统提示 | 已通过 | 用户完成交互式验收 |
| 证书首次、拒绝、匹配与变更 | 已通过 | 用户完成受控环境验收 |
| 更新/清除密码和删除 Profile | 已通过 | 用户完成 Phase 7 人工验收 |
| 删除失败后的保留与重试 | 已通过 | 用户完成失败恢复验收 |
| 真实可达/不可达提示且不阻止连接 | 已通过 | 用户完成受控网络验收 |
| Gateway 感知提示 | 延后 | Phase 9 提供 Gateway 配置后验收 |

## 6. 已知限制与后续边界

- 命令行测试证明存储、状态和桥接边界，不证明 GUI 观感、系统 Keychain 对话框或真实 RDP
  身份验证行为。
- 当前应用实例仍沿用现有单活动会话协调器；多会话/“在新窗口连接”不是本阶段已验证能力。
- RD Gateway 配置、认证、证书和分层可达性属于 Phase 9；本阶段只检查直连目标。
- Profile 已持久化桌面、重定向和快捷键字段，但相关高级设置的完整编辑界面随对应功能阶段补齐。
- macOS 26 glass effect 在较早系统上使用系统 Material 回退；两种路径均需人工检查减少透明度。
