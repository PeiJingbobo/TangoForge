# P3-05 分层错误映射与恢复动作验收

- 日期：2026-08-01
- 任务：P3-05
- 状态：待人工验收
- 验收目标：网络、TLS、安全协商、证书、认证、服务端拒绝和协议错误可区分，且每次失败后都能安全恢复和重试。

## 验收原则

- 使用一次性虚拟机或专用测试账号制造失败，不要用日常账号反复测试错误密码。
- 不为测试降低 TLS、NLA 或证书校验，不在生产主机上修改远程登录权限。
- 真实端点、用户名和密码只放在 `config/local/` 的 Git 忽略配置中，不进入命令行、日志、截图或本文档。
- 每个场景使用全新 App 会话单独执行并记录界面错误文本及 `code 0x...`；原生错误码可以因服务端实现不同而不同，不作为固定断言。
- 失败测试脚本以非零状态退出是预期行为；是否通过取决于错误分类和恢复行为，而不是脚本退出码为 0。

## 每个失败场景的共同通过条件

1. 显示符合场景的独立、可理解错误类别，并保留脱敏原生十六进制错误码。
2. 不打开远程画面窗口；如果失败发生在窗口打开后，窗口会自动关闭。
3. 按钮最终恢复为 `Connect`，没有停留在 `Cancel / Disconnect`。
4. 修正配置后可以立即重新连接，不需要重启 App。
5. 不发生自动无限重试，不出现旧会话回调覆盖新会话状态。
6. 日志和界面不泄露密码、完整证书、主机或用户名。

## 建议执行顺序

| 顺序 | 分类 | 安全构造方式 | 预期类别 |
|---|---|---|---|
| 1 | DNS（补充项） | 使用保留的 `.invalid` 域名 | `host name could not be resolved` |
| 2 | 网络 | 先确认本机某端口未监听，再连接 `localhost` 的该端口 | `remote desktop service is unreachable` |
| 3 | 证书 | 对专用测试主机的未知或变化证书选择 `Reject` | `certificate was rejected` 或 `certificate changed` |
| 4 | 认证 | 对可重置的专用测试账号只提交一次错误密码 | `authentication failed` |
| 5 | 服务端拒绝 | 使用正确凭据，但在专用 Windows 实验环境撤销“通过远程桌面服务登录”权限 | `server refused the connection` |
| 6 | TLS | 使用专用故障注入端点：选择 TLS 后中止或破坏 TLS 握手 | `TLS connection failed` |
| 7 | 安全协商 | 使用专用实验服务返回不兼容的 RDP 安全协商结果 | `TLS/NLA security negotiation failed` |
| 8 | 协议 | 使用本地一次性畸形 RDP fixture 返回非法协商数据 | `RDP protocol negotiation failed` |

P3-05 的最低完成条件按开发计划是网络、TLS、证书、认证、服务端和协议六类均有证据；DNS 和安全协商是额外的细分回归。若没有一次性 Windows 实验环境或故障注入 fixture，不应在真实日常主机上强行制造后四类失败，任务继续保持“待人工验收”。

## App 人工验证步骤

对上表的每个场景执行：

1. 启动 Debug App，填写该场景的本地测试配置。
2. 点击 `Connect`，保存错误类别和 `code 0x...`，不要截取包含连接信息的完整表单。
3. 确认窗口和按钮满足共同通过条件。
4. 改回正确配置并连接成功，再点击 `Disconnect`，确认窗口关闭且按钮恢复为 `Connect`。
5. 完全退出并重新启动 App，再执行下一场景，避免场景之间共享状态。

## Bridge 集成测试辅助命令

为每个场景复制一份 `config/local/` 下的未跟踪 JSON 配置，然后在 Mac 项目根目录执行：

~~~sh
FARFRAME_INTEGRATION_CONFIG=config/local/<scenario>.json \
/bin/sh scripts/test-rdp-integration.sh
~~~

当前依赖产物已存在时，脚本不要求额外设置 CMake/Ninja；只有依赖需要重建时，才需要设置 `FARFRAME_CMAKE` 和 `FARFRAME_NINJA`。

该 harness 适合确认 Bridge 返回的分类和资源回收；按钮恢复、错误文案和窗口联动仍必须在解锁的 macOS 图形会话中验收。

## 当前自动化证据

- 原生 sanitizer harness 已断言 TLS、安全协商、认证、服务端拒绝和未知协议的 FreeRDP 错误映射。
- `FREERDP_ERROR_CONNECT_LOGON_TYPE_NOT_GRANTED` 归入服务端拒绝，而不是错误密码认证失败。
- 2026-08-01：18 项 Xcode 测试通过，0 失败。
- 2026-08-01：Bridge ASan/UBSan 测试通过。

这些自动化证明映射表和内存安全回归，但不能替代 TLS、认证、服务端拒绝和畸形协议的真实运行环境证据。

## 验收记录模板

| 分类 | 实际界面类别 | 原生错误码 | 按钮恢复 | 修正后重连 | 无敏感输出 | 结果 |
|---|---|---|---|---|---|---|
| 网络 |  |  |  |  |  |  |
| TLS |  |  |  |  |  |  |
| 证书 |  |  |  |  |  |  |
| 认证 |  |  |  |  |  |  |
| 服务端 |  |  |  |  |  |  |
| 协议 |  |  |  |  |  |  |

六行全部通过后，把 `01-executable-backlog.md` 和 `phase-3-validation.md` 中的 P3-05 状态改为“已完成”。
