// Package llm 提供 LLM HTTP 客户端封装（供 parser / exporter 复用）。
//
// 约束（docs/TECHNICAL.md §2.2）：仅 JSON 结构化通信，不承载业务逻辑；
// 配置（接口地址、密钥、模型名、超时、重试、max_tokens、并发）来自全局配置。
package llm
