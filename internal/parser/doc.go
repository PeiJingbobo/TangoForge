// Package parser 负责 LLM 交互：Markdown → 结构化 JSON（草稿生成）。
//
// 约束（docs/TECHNICAL.md §3.5）：
//   - 调用 LLM 的 Prompt 必须包含严格的 JSON Schema 约束；
//   - 若 LLM 返回的 JSON 缺少 title 或 status 字段 → 必须拒绝入库并返回明确错误，禁止补默认值；
//   - 解析成功 → 生成草稿（import_drafts）→ 调用方显式确认后按 source_file 文件级全量覆盖入库。
package parser
