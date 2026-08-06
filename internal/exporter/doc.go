// Package exporter 负责从数据库重建 Markdown（模板渲染、LLM 生成模板）。
//
// 约束（docs/TECHNICAL.md §3.5）：
//   - 模板引擎仅用 Go text/template，禁止引入其他模板语言；
//   - 默认模板 templates/default.tmpl；用户自定义模板由项目配置 export.template_path 覆盖；
//   - 支持 LLM 生成模板：POST /api/export/template/generate。
package exporter
