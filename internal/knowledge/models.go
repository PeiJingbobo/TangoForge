package knowledge

import "time"

// KnowledgeBase 命名知识库（knowledge_bases 表，docs/KNOWLEDGE-BASE.md §3）。
type KnowledgeBase struct {
	ID          int64  `json:"id"`
	ProjectID   int64  `json:"project_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	// DocCount 库内文档数（列表查询联表填充，非表字段）。
	DocCount int `json:"doc_count,omitempty"`
}

// Document 文档引用记录（knowledge_documents 表，§3）。
//
// 唯一性：同一项目内按 abs_path 唯一（idx_kd_abs）；多库/多任务共享同一记录与同一份向量。
type Document struct {
	ID             string `json:"id"`
	ProjectID      int64  `json:"project_id"`
	Path           string `json:"path"`        // 引用路径（相对 workdir 或绝对，原始形态）
	AbsPath        string `json:"abs_path"`    // 规范化绝对路径（唯一键）
	RelPath        string `json:"rel_path"`    // 相对 workdir 路径（项目内时）
	OriginPath     string `json:"origin_path"` // 外部文件拷贝前的原始绝对路径
	DisplayName    string `json:"display_name"`
	Type           string `json:"type"`  // text / binary
	Size           int64  `json:"size"`  // 字节
	MTime          string `json:"mtime"` // RFC3339（文件修改时间）
	ContentHash    string `json:"content_hash"`
	Summary        string `json:"summary"`
	Status         string `json:"status"`          // ok / missing / indexing / failed
	Embedded       int    `json:"embedded"`        // 0 未嵌入 / 1 已嵌入 / 2 失败
	EmbeddingModel string `json:"embedding_model"` // 生成向量时的模型（漂移检测）
	IndexError     string `json:"index_error"`
	// Archived 归档标记（TF-052）：归档后从默认列表/检索/扫描隐藏，
	// 任务引用（task_documents）与文件保留可访问。
	Archived  bool          `json:"archived"`
	History   []PathHistory `json:"history"` // JSON：[{path, relinked_at}]
	CreatedAt string        `json:"created_at"`
	UpdatedAt string        `json:"updated_at"`
	// 详情扩展（非表字段）：关联任务数 / 所属库列表。
	TaskCount int     `json:"task_count,omitempty"`
	KBs       []int64 `json:"kb_ids,omitempty"`
}

// PathHistory relink 历史条目（history JSON 数组元素）。
type PathHistory struct {
	Path       string `json:"path"`
	RelinkedAt string `json:"relinked_at"`
}

// Chunk 向量分块（knowledge_chunks 表，检索最小单元，§2.6/§3）。
type Chunk struct {
	ID         string `json:"id"`
	DocumentID string `json:"document_id"`
	Seq        int    `json:"seq"`
	Heading    string `json:"heading"`
	Content    string `json:"content"`
	Vector     []byte `json:"-"` // f32 little-endian BLOB
	Dim        int    `json:"dim"`
	CreatedAt  string `json:"created_at"`
}

// nowRFC3339 返回当前时间（RFC3339 本地时区，QA Q7 时间约定）。
func nowRFC3339() string {
	return time.Now().Format(time.RFC3339)
}
