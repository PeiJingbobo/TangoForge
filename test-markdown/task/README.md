# Farframe RDP 任务文档

本目录是 Farframe RDP 的行动计划与任务执行入口。产品目标、技术边界和最终验收仍以
[`../Farframe-RDP-Development-Plan.md`](../Farframe-RDP-Development-Plan.md) 为准。

## 文档导航

| 文档 | 用途 |
|---|---|
| [`00-master-action-plan.md`](00-master-action-plan.md) | 阶段、依赖、里程碑、质量门和交付顺序 |
| [`01-executable-backlog.md`](01-executable-backlog.md) | 可领取、可验证的任务清单 |
| [`task-template.md`](task-template.md) | 新增或拆分任务时使用的统一模板 |
| [`manual-verification/README.md`](manual-verification/README.md) | 每个任务必须产出的人工验证手册规则与索引 |
| [`manual-verification/template.md`](manual-verification/template.md) | 任务级人工验证手册模板 |
| [`phase-7-validation.md`](phase-7-validation.md) | Phase 7 Profile、Keychain、一键连接、新版界面与人工验收步骤 |
| [`branch-settings-sidebar-auto-quit.md`](branch-settings-sidebar-auto-quit.md) | B7-01 设置固定侧边栏与最后窗口关闭自动退出支线记录 |
| [`manual-verification/B7-01.md`](manual-verification/B7-01.md) | B7-01 可重复执行的设置 UI 与窗口生命周期人工验证手册 |
| [`phase-6a-validation.md`](phase-6a-validation.md) | Phase 6A 快捷键实现、自动化证据与人工验收步骤 |
| [`internationalization.md`](internationalization.md) | 简体中文默认语言、String Catalog 约定与发布前语言适配流程 |
| [`phase-5-validation.md`](phase-5-validation.md) | Phase 5 基础输入实现、自动化证据、人工验收与限制 |
| [`phase-4-validation.md`](phase-4-validation.md) | Phase 4 画面链路、自动化证据、人工验收与限制 |
| [`phase-3-validation.md`](phase-3-validation.md) | Phase 3 实现、自动化、真实连接证据与未验收项 |
| [phase-3-p3-05-acceptance.md](phase-3-p3-05-acceptance.md) | P3-05 分层错误映射与恢复动作人工验收清单 |
| [phase-0-validation.md](phase-0-validation.md) | Phase 0 决策与 Mac 环境探测记录 |
| [phase-1-validation.md](phase-1-validation.md) | Phase 1 构建、测试、GUI 与已知限制记录 |
| [phase-2-native-dependency-decision.md](phase-2-native-dependency-decision.md) | FreeRDP、OpenSSL、能力开关和 Bridge 边界决策 |
| [phase-2-validation.md](phase-2-validation.md) | Phase 2 构建、sanitizer、测试与 Release 链接审计 |

## 使用方式

1. 从 `01-executable-backlog.md` 中选择所有依赖均已完成的最前置任务。
2. 开始前把状态改为 `进行中`，确认任务范围和验证方法。
3. 一次只交付一个完整纵向切片；行为、失败路径和测试应在同一任务中完成。
4. 交付实现时，在 `manual-verification/<任务 ID>.md` 新增或更新可由他人照做的人工验证手册。
5. 完成后记录实际执行的命令、测试结果、未运行项、人工验收和残余风险。
6. 必需人工验证尚未执行时保持 `待人工验收`；只有验证完成并满足阶段质量门时才改为 `已完成`。

## 状态定义

- `待办`：尚未开始。
- `进行中`：已有负责人正在实施。
- `阻塞`：存在明确外部阻塞，必须记录失败命令、关键错误和最小解阻动作。
- `待人工验收`：自动化检查已通过，但仍需解锁的 Mac 图形会话或真实 Windows 环境。
- `已完成`：实现、自动化验证、必要人工验收与文档更新均已完成。

无可执行人工步骤的任务也必须有验证手册，并在其中标记“不适用”、说明理由及替代的自动化证据。

## 维护规则

- 不在任务文档中记录真实主机、用户名、密码、证书、SSH 地址、签名身份或账号标识。
- 不因进度变化修改开发计划；只有产品范围、架构、验收或兼容目标变化时才更新源计划。
- 兼容能力只能依据实际测试标记为“支持、部分支持、实验性、不支持”。
- Windows 侧端点与凭据只来自本地、未跟踪的操作者配置。
- 构建产物、Derived Data、真实诊断包和机器本地配置不得进入版本库。

