// Package task 是任务核心域：模型、CRUD、状态机校验、归档/还原、依赖校验。
//
// 分层铁律（docs/TECHNICAL.md §2.3）：
//   - 本包禁止引用 api / mcp / cmd 包；
//   - 数据库事务边界必须在本层控制，db 层仅提供原生 SQL 或 Query Builder；
//   - 传输层（api / mcp / cli）共享本层实现，接口先行。
//
// 测试要求（docs/TECHNICAL.md §3.8）：本包覆盖率不低于 90%（含状态机校验、
// 归档/还原、依赖校验、草稿导入流程），单元测试使用 sqlite:memory: 隔离。
package task

import "time"

// 默认状态机（docs/REQUIREMENTS.md §2.2）。
// archived 为系统保留态，仅由"归档/还原"操作使用，不可出现在普通流转编辑中。
const (
	StatusTodo     = "todo"
	StatusDoing    = "doing"
	StatusDone     = "done"
	StatusArchived = "archived"
)

// Task 任务实体，字段与 docs/TECHNICAL.md §3.2 Task 结构体一一对应。
type Task struct {
	ID            string    `json:"id" db:"id"`                         // UUID v4
	ProjectID     int64     `json:"project_id" db:"project_id"`         // 所属项目
	ParentID      *string   `json:"parent_id" db:"parent_id"`           // 父任务 ID（nil = 顶层任务）
	Title         string    `json:"title" db:"title"`                   //
	Number        string    `json:"number" db:"number"`                 // 简短唯一编号（TF-040，如 T01；创建/导入自动分配，文档编号可沿用）
	Description   string    `json:"description" db:"description"`       //
	Status        string    `json:"status" db:"status"`                 // 项目状态机 key（默认 todo/doing/done/archived）
	Priority      int       `json:"priority" db:"priority"`             // 0-5，0=最低
	Tags          []string  `json:"tags" db:"tags"`                     // JSON 数组
	Assignee      string    `json:"assignee" db:"assignee"`             //
	DependsOn     []string  `json:"depends_on" db:"depends_on"`         // JSON 数组，存储 Task ID
	ArchivedFrom  string    `json:"archived_from" db:"archived_from"`   // 归档前状态（还原用）
	SourceFile    string    `json:"source_file" db:"source_file"`       // 原始 Markdown 路径
	SourceSection string    `json:"source_section" db:"source_section"` // LLM 解析映射段落
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}
