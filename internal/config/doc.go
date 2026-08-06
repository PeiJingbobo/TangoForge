// Package config 负责全局配置与项目配置的加载、合并与热重载。
//
// 约束（docs/TECHNICAL.md §3.1「显式优于隐式」）：
//   - 全局配置 ~/.taskboard-app/config.yaml：LLM 密钥、监听端口、远程访问开关、API Token、UI 会话凭据；
//   - 项目配置 {workdir}/.taskboard/config.yaml：导出模板、状态机、自定义字段等业务配置；
//   - 配置分层严禁混淆；本包仅负责加载与合并（fsnotify + 原子替换热重载），不做任何业务判断。
package config
