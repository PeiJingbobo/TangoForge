# Phase 3 连接闭环验证记录

- 日期：2026-08-01
- 范围：P3-01 至 P3-06
- 结论：最小连接闭环已实现并通过一个真实 RDP 主机的 NLA 连接验证；跨 Windows 版本、错误凭据和图形界面人工验收仍未完成

## 已实现

- ConnectionEndpoint 对主机、端口、用户名、域和 UTF-8 长度做确定性校验。
- 单向 SessionStateMachine 使用每次连接独立 UUID，拒绝非法迁移和旧会话迟到事件。
- SessionCoordinator 只在 @MainActor 更新 UI；独立串行队列创建、连接并销毁 Bridge 会话。
- Bridge 把配置复制进 FreeRDP settings，明确启用 TLS/NLA，关闭传统 RDP 安全层、忽略证书和自动接受证书。
- FreeRDP 认证回调最多允许原始凭据一次，包括操作者明确提供的空密码；第二次回调立即拒绝，避免无界重试。
- 未知证书必须由 UI 选择拒绝、仅本次信任或信任并记住；证书变化使用独立回调和醒目标记。
- DNS、网络、TLS、TLS/NLA 协商、证书拒绝、证书变化、认证、服务端拒绝和协议错误使用独立类别。
- 连接中取消会唤醒证书等待和 FreeRDP abort；连接线程返回后清除密码 setting 并精确释放 context、instance 和 Bridge 分配。
- 关闭 FreeRDP/WinPR 默认原生日志，避免其绕过 OSLog 隐私规则输出主机和证书信息。用户只看到 Farframe 的脱敏错误类别。
- 应用外壳提供直接连接表单、状态、取消/断开按钮和证书决策对话框。Phase 4 前远程窗口仍是空画布。

## 通过的自动化验证

### Debug Xcode 测试

~~~sh
FARFRAME_CMAKE=/path/to/cmake \
FARFRAME_NINJA=/path/to/ninja \
/bin/sh scripts/test.sh
~~~

结果：18 项测试，0 失败。

- FarframeCoreTests：10 项，包括配置边界、合法/非法状态迁移、迟到会话隔离、worker 完成收敛和既有脱敏测试。
- FarframeRDPTests：3 项，验证应用启动、FreeRDP 版本和远程窗口生命周期。
- FarframeRDPBridgeTests：5 项，包括 250 次创建/销毁、配置复制、空参数、线程所有权和未连接协议状态。

无头测试环境仍会输出 AppIntents/linkd 系统服务日志，不涉及连接数据或测试失败。

### 原生 sanitizer

~~~sh
FARFRAME_CMAKE=/path/to/cmake \
FARFRAME_NINJA=/path/to/ninja \
/bin/sh scripts/test-native-bridge.sh
~~~

结果：ASan 与 UBSan 通过，无 Bridge 越界、UAF 或未定义行为报告。

Command Line Tools clang 继续输出上游静态对象的 +zcm/+zcz target-feature 提示；与 Phase 2
记录一致，不影响测试结果。

### Release 构建

~~~sh
FARFRAME_CONFIGURATION=Release \
FARFRAME_CMAKE=/path/to/cmake \
FARFRAME_NINJA=/path/to/ninja \
/bin/sh scripts/build.sh
~~~

结果：arm64、macOS 14.0 deployment target、CODE_SIGNING_ALLOWED=NO Release 构建成功，
Swift 6 严格并发和警告即错误均通过。

## 真实连接与失败路径验证

真实端点、用户名和临时密码只存在于 config/local/ 下的 Git 忽略配置。脚本先用
git check-ignore 验证配置不会被跟踪，再通过进程环境把值交给测试程序；它们不进入参数和输出。

~~~sh
FARFRAME_INTEGRATION_CONFIG=config/local/integration.json \
FARFRAME_CMAKE=/path/to/cmake \
FARFRAME_NINJA=/path/to/ninja \
/bin/sh scripts/test-rdp-integration.sh
~~~

已观察结果：

- 与一个局域网 Windows RDP 主机完成连接。
- FreeRDP 最终协商结果为 NLA/CredSSP，而不是只在设置中声明启用。
- 自签名/无法由系统直接验证的证书经过明确“仅本次信任”后继续连接。
- 连接后主动取消，Bridge live-session 计数回到 0。
- 对保留 TEST-NET 地址在连接阶段发出取消，FreeRDP 及时返回并完整清理。
- 保留 .invalid 域名被映射为 DNS 解析失败。
- 本机关闭端口被映射为远程桌面服务不可达。
- 显式拒绝测试主机证书时连接终止，并映射为证书拒绝。
- 修复前发现 FreeRDP 默认日志会输出连接和证书信息；关闭原生日志后重新执行真实连接，
  输出只剩不含用户数据的结果摘要。

## 未执行与具体原因

- 未确认真实主机的 Windows 版本和账户类型，因此不声明 Windows 10/11、Windows Server
  或本地/域账户兼容已经通过。
- 未提供第二个 Windows Server 测试环境，源计划要求的 Server 验收尚未执行。
- 未用错误密码攻击真实账户，避免因重复认证触发账户锁定；“错误密码只尝试一次”目前由
  认证回调计数约束实现，仍需专用可重置测试账户验证。
- 未验证受信任 CA 证书、证书真实轮换、保存后重连、非默认端口、IPv6 和 TLS-only 主机。
- 未在解锁的 Mac 图形会话中人工点击连接表单和三种证书按钮；命令行构建不能替代 GUI 验收。
- Phase 4 图形尚未实现，因此本阶段只证明协议连接进入 connected，不证明能看到桌面。
- 尚未执行长时间和高次数真实连接/断开压力测试；Phase 11 仍需覆盖持续资源增长。

## 残余风险

- 当前为防止敏感输出而关闭 FreeRDP/WinPR 原生日志。Farframe 已保留脱敏错误类别和原生错误码；
  后续如需要更深诊断，应实现可审计的脱敏日志适配器，而不是重新打开默认日志。
- “信任并记住”目前使用 FreeRDP 的本机证书存储。Phase 7 删除 Profile 时必须同步删除
  Profile 级信任记录，或把信任存储迁移到 Farframe 管理层。
- 证书变化回调和 UI 已实现，但尚缺真实证书轮换环境的端到端验证。
- 认证、TLS、服务端拒绝和协议错误均有独立映射，但除 NLA 成功外尚缺相应真实失败环境。
- 主动断开与取消共用线程安全取消路径；会话内服务端主动断开已处理，但尚未做突然断网人工验收。

## 连接后指针更新崩溃修复

2026-08-01 的首次 GUI 验收发现：建立连接并打开远程窗口后，程序在
FreeRDP `update_pointer_new` 中以 `EXC_BAD_ACCESS` 崩溃。崩溃线程是独立连接队列，
空地址偏移为 `0x10`。

根因是 Bridge 未安装 `PostConnect` 客户端初始化。FreeRDP 已注册指针更新回调，
但 `rdpContext.cache` 仍为空；服务器发送新指针形状时会访问尚未创建的 pointer cache。

修复内容：

- 在 `PostConnect` 中调用 `gdi_init`，建立 BGRA 后备帧缓冲、graphics 和 cache；
- 注册 Phase 3 使用的安全指针消费者，接收并释放指针数据但暂不显示；
- 在 `PostDisconnect` 中调用 `gdi_free`，覆盖正常断开和连接失败清理；
- 原生 sanitizer harness 增加 GDI/cache 创建与释放回归；
- 真实连接在 connected 后继续消费图形/指针消息 3 秒再取消。

修复后验证：Bridge ASan/UBSan 通过，18 项 Xcode 测试全部通过，真实主机 NLA
连接、持续事件消费、取消和资源清理通过。实际远程画面呈现仍属于 Phase 4。

## 会话窗口与状态联动修复

2026-08-01 的 GUI 验收还发现两个生命周期问题：

- 一次首次点击连接返回“remote desktop service is unreachable”，再次连接成功；
- 已连接时点击“Cancel / Disconnect”不会关闭远程窗口，手动关闭窗口也不会断开会话或恢复 Connect 按钮。

第二项的根因已确认：远程窗口和 `SessionCoordinator` 没有双向生命周期通知。现已实现：

- 点击断开时先请求线程安全取消并立即关闭远程窗口；
- 用户手动关闭远程窗口时反向请求取消连接；
- session 进入 disconnecting、disconnected 或 failed 时统一关闭窗口；
- worker 最终退出时把仍处于活动状态的状态机收敛为 disconnected；
- App 外壳消失时清除窗口回调、取消连接并关闭窗口。

首次不可达在同一真实配置的三次全新会话中均未复现，因此没有加入可能掩盖故障的自动重试。
界面现在会在脱敏错误类别后显示 FreeRDP 原生十六进制错误码；再次出现时可据此区分连接拒绝、
超时或传输协商问题，不包含主机、用户名或凭据。

回归结果：18 项 Xcode 测试全部通过，窗口主动关闭通知和状态机完成收敛有确定性测试；
真实 NLA 连接、取消与资源清理通过；Release 构建通过。GUI 点击行为仍需在解锁的 Mac 会话中人工确认。

## 当前任务状态

- P3-01：已完成。
- P3-02：已完成。
- P3-03：已完成（2026-08-01 由操作员确认人工验收通过）。
- P3-04：已完成（2026-08-01 由操作员确认人工验收通过）。
- P3-05：开发工作已完成；按 2026-08-01 安排，真实失败环境回归并入后续统一人工测试批次。
- P3-06：已完成。
