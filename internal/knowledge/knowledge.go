// Package knowledge 是知识库业务层（docs/KNOWLEDGE-BASE.md，TF-044~TF-052）。
//
// 职责：知识库 = 「文档引用注册表 + 任务关联 + 语义索引（摘要/向量）」，不存储原文；
// 原文以文件系统为唯一真实源。提供命名多库、文档注册/复用/拷贝、任务关联、
// 分块 + 摘要 + 嵌入、余弦检索、文件扫描与防抖等能力。
//
// 分层铁律（AGENTS.md §3.2）：本包为业务层，禁止引用 api / mcp / cmd；
// 数据库事务边界在本层控制；HTTP / MCP / CLI 共享本层实现。
package knowledge

// 常量定义（docs/KNOWLEDGE-BASE.md §2/§4 语义）。
const (
	// DefaultKBName 默认库名称（项目初始化时自动创建，is_default=1）。
	DefaultKBName = "默认库"

	// DocTypeText / DocTypeBinary 文档类型。
	DocTypeText   = "text"
	DocTypeBinary = "binary"

	// DocStatusOK / DocStatusMissing / DocStatusIndexing / DocStatusFailed 文档状态。
	DocStatusOK       = "ok"
	DocStatusMissing  = "missing"
	DocStatusIndexing = "indexing"
	DocStatusFailed   = "failed"

	// EmbedNo / EmbedYes / EmbedFailed 嵌入状态（embedded 列）。
	EmbedNo     = 0
	EmbedYes    = 1
	EmbedFailed = 2
)
