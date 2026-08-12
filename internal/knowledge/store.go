package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"tangoforge/internal/db"
	"tangoforge/internal/llm"

	"github.com/google/uuid"
)

// Options Service 构造选项。
type Options struct {
	Logger *slog.Logger
	// Tasks 任务服务只读接口（校验任务存在性；TF-045 关联校验用）。
	Tasks TaskLister
	// LLM 摘要生成客户端（TF-046）；nil = 摘要功能禁用（文档仍可注册/嵌入）。
	LLM *llm.Client
}

// TaskLister 任务服务的最小只读接口（knowledge 不依赖 task 全接口，保持职责单一）。
// Get 返回 any（校验存在性即可，不关心具体类型）。
type TaskLister interface {
	Get(ctx context.Context, workdir, id string) (any, error)
}

// taskListerAdapter 适配：将返回具体类型的 Get 函数适配为 TaskLister。
type taskListerAdapter struct {
	GetFn func(ctx context.Context, workdir, id string) (any, error)
}

func (a taskListerAdapter) Get(ctx context.Context, workdir, id string) (any, error) {
	return a.GetFn(ctx, workdir, id)
}

// TaskListerAdapter 由任意 Get 函数构造 TaskLister（传输层组装用；
// 例如 task.Service.Get 返回 task.Task，此处转换为 any）。
func TaskListerAdapter(fn func(ctx context.Context, workdir, id string) (any, error)) TaskLister {
	return taskListerAdapter{GetFn: fn}
}

// KnowledgeFile 知识库文件建议（parser 草稿透传；KB 为库名或空=默认库）。
type KnowledgeFile struct {
	Path   string `json:"path"`
	KB     string `json:"kb,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// LinkFilesResult 批量关联结果。
type LinkFilesResult struct {
	// Linked 成功建立关联的文档数。
	Linked int `json:"linked"`
	// Dropped 路径缺失/无法读取被跳过的文件数（QA-K17：仅警告不阻断）。
	Dropped int `json:"dropped"`
	// InvalidKB 引用了不存在的库（整次失败返回错误，此处预留）。
	InvalidKB int `json:"invalid_kb"`
}

// Service 知识库业务服务（三端共享：HTTP / MCP / CLI，docs/KNOWLEDGE-BASE.md §5.1）。
//
// 方法签名统一携带 workdir（多项目显式标识，QA Q2-B）；数据库事务边界在本层控制。
// 写操作成功后通过 OnWrite 钩子触发审计与 WS 事件（模式同 task.WriteHook）。
type Service interface {
	// ListBases 库列表（含文档数，§6 GET /api/knowledge/bases）。
	ListBases(ctx context.Context, workdir string) ([]KnowledgeBase, error)
	// GetBase 库详情。
	GetBase(ctx context.Context, workdir string, id int64) (KnowledgeBase, error)
	// CreateBase 创建库（重名 → KNOWLEDGE_INVALID，QA-K1）。
	CreateBase(ctx context.Context, workdir, name, description string) (KnowledgeBase, error)
	// UpdateBase 重命名/改描述（默认库可改；is_default 不可转移）。
	UpdateBase(ctx context.Context, workdir string, id int64, name, description *string) (KnowledgeBase, error)
	// DeleteBase 删除库（仅移除库↔文档边，QA-K14）。
	DeleteBase(ctx context.Context, workdir string, id int64) error
	// EnsureDefaultBase 确保默认库存在（懒创建；存量项目补建，幂等）。
	EnsureDefaultBase(ctx context.Context, workdir string) (KnowledgeBase, error)

	// ListDocuments 文档列表（filter[kb_id]/filter[status]/q/分页）。
	ListDocuments(ctx context.Context, workdir string, f DocumentFilter) (DocumentListResult, error)
	// GetDocument 文档详情 + 真实路径 + 关联任务数/库列表。
	GetDocument(ctx context.Context, workdir, id string) (Document, error)
	// RegisterDocument 注册文档 {path, copy, kb_ids[]}（存在则复用并返回既有记录，QA-K16）。
	RegisterDocument(ctx context.Context, workdir, path, copyMode string, kbIDs []int64) (Document, error)
	// UpdateContent 编辑原文（QA-K7：直接写盘原文件 → 触发重新索引；二进制拒绝）。
	UpdateContent(ctx context.Context, workdir, id, content string) error
	// DeleteDocument 解除引用（删文档 + chunks + 边）。
	DeleteDocument(ctx context.Context, workdir, id string) error
	// RelinkDocument 重新链接 {new_path, copy}（更新路径 + history + 重置重建索引，QA-K15）。
	RelinkDocument(ctx context.Context, workdir, id, newPath, copyMode string) (Document, error)

	// LinkTask 任务关联 {task_id, document_id 或 path, copy, kb_ids[]}（路径未注册自动入库）。
	LinkTask(ctx context.Context, workdir, taskID, documentID, path, copyMode string, kbIDs []int64) error
	// UnlinkTask 解除任务关联 {task_id, document_id}（文档本身保留）。
	UnlinkTask(ctx context.Context, workdir, taskID, documentID string) error
	// TaskDocuments 任务关联的文档列表。
	TaskDocuments(ctx context.Context, workdir, taskID string) ([]Document, error)
	// LinkFiles 批量关联知识库文件到任务（TF-049 confirm 用，QA-K11/K17）：
	// 逐条注册/复用 + 拷贝 + 库归属 + 建立任务关联 + 触发异步索引；
	// 路径不存在/无法读取 → 仅警告跳过（dropped 计数返回，不阻断导入）。
	LinkFiles(ctx context.Context, workdir string, taskIDs []string, files []KnowledgeFile, copyMode string) (LinkFilesResult, error)

	// OnWrite 写操作回调（由 api 层注入审计/WS 事件）。
	SetOnWrite(fn func(ctx context.Context, workdir, action, target string))
	// SetEmbeddingConfig 设置向量检索用 embedding 配置（TF-047；daemon 启动时注入）。
	SetEmbeddingConfig(cfg *llm.EmbeddingConfig)
	// SummarizeAndCache 生成摘要并按 content_hash 缓存（TF-046；llmClient nil 时跳过）。
	SummarizeAndCache(ctx context.Context, workdir, docID, contentHash, text string) (string, error)
	// IndexDocument 文档索引流水线（TF-047：分块 + 摘要 + 嵌入 + 写 chunks）。
	IndexDocument(ctx context.Context, workdir, docID string, opts IndexOptions) (IndexResult, error)
	// Search 向量检索（TF-047：纯 Go 余弦，文档 + 命中片段）。
	Search(ctx context.Context, workdir string, q SearchQuery) (SearchResult, error)
	// Close 关闭缓存的项目库连接。
	Close() error
}

// DocumentFilter 文档列表过滤/分页。
type DocumentFilter struct {
	KBID   int64  // 0 = 全部
	Status string // 空 = 全部
	Q      string // 匹配 display_name/path/abs_path
	Page   int
	Size   int
}

// DocumentListResult 文档列表结果。
type DocumentListResult struct {
	Items []Document `json:"items"`
	Total int        `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}

// 分页默认值与上限。
const (
	defaultPageSize = 50
	maxPageSize     = 500
)

// service 实现。
type service struct {
	mu        sync.Mutex
	dbs       map[string]*sql.DB
	fp        map[string]*db.FileFingerprint
	logger    *slog.Logger
	tasks     TaskLister
	llmClient *llm.Client
	embCfg    *llm.EmbeddingConfig // 向量检索用 embedding 配置（nil = 检索不可用，QA-K23）
	onWrite   func(ctx context.Context, workdir, action, target string)
}

// NewService 构造知识库服务。
func NewService(opts Options) Service {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &service{
		dbs:       make(map[string]*sql.DB),
		fp:        make(map[string]*db.FileFingerprint),
		logger:    opts.Logger,
		tasks:     opts.Tasks,
		llmClient: opts.LLM,
	}
}

// SetOnWrite 注入写操作回调。
func (s *service) SetOnWrite(fn func(ctx context.Context, workdir, action, target string)) {
	s.onWrite = fn
}

// SetEmbeddingConfig 设置向量检索用 embedding 配置（TF-047；nil = 检索返回未配置错误）。
func (s *service) SetEmbeddingConfig(cfg *llm.EmbeddingConfig) {
	s.embCfg = cfg
}

// fireWrite 触发写操作回调（不阻塞业务）。
func (s *service) fireWrite(ctx context.Context, workdir, action, target string) {
	if s.onWrite != nil {
		s.onWrite(ctx, workdir, action, target)
	}
}

// projectDB 打开并缓存项目库连接（模式同 task.Service.projectDB，TF-001 指纹校验）。
func (s *service) projectDB(ctx context.Context, workdir string) (*sql.DB, error) {
	clean := filepath.Clean(workdir)
	if !filepath.IsAbs(clean) {
		return nil, fmt.Errorf("%w: %s 不是绝对路径", ErrDocumentInvalid, workdir)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if conn, ok := s.dbs[clean]; ok {
		if s.fp[clean].SameAs(db.MetaDBPath(clean)) {
			return conn, nil
		}
		s.logger.Warn("project db file replaced, reopening", "workdir", clean, "path", db.MetaDBPath(clean))
		_ = conn.Close()
		delete(s.dbs, clean)
		delete(s.fp, clean)
	}
	if _, err := os.Stat(db.MetaDBPath(clean)); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrDocumentInvalid, workdir)
	}
	fp, err := db.CaptureFingerprint(db.MetaDBPath(clean))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrDocumentInvalid, workdir)
	}
	conn, err := db.EnsureProject(ctx, db.MetaDBPath(clean))
	if err != nil {
		return nil, fmt.Errorf("knowledge: open project db %s: %w", clean, err)
	}
	s.dbs[clean] = conn
	s.fp[clean] = fp
	return conn, nil
}

// Close 关闭全部缓存的项目库连接。
func (s *service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for wd, conn := range s.dbs {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("knowledge: close %s: %w", wd, err)
		}
		delete(s.dbs, wd)
		delete(s.fp, wd)
	}
	return firstErr
}

// ---- 库 CRUD ----

// ListBases 库列表（含文档数）。
func (s *service) ListBases(ctx context.Context, workdir string) ([]KnowledgeBase, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx, `
		SELECT kb.id, kb.project_id, kb.name, kb.description, kb.is_default, kb.created_at, kb.updated_at,
		       COUNT(kbd.document_id) AS doc_count
		FROM knowledge_bases kb
		LEFT JOIN knowledge_base_documents kbd ON kbd.kb_id = kb.id
		GROUP BY kb.id
		ORDER BY kb.is_default DESC, kb.id ASC`)
	if err != nil {
		return nil, fmt.Errorf("knowledge: list bases: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []KnowledgeBase
	for rows.Next() {
		var kb KnowledgeBase
		var isDefault int
		if err := rows.Scan(&kb.ID, &kb.ProjectID, &kb.Name, &kb.Description, &isDefault,
			&kb.CreatedAt, &kb.UpdatedAt, &kb.DocCount); err != nil {
			return nil, fmt.Errorf("knowledge: scan base: %w", err)
		}
		kb.IsDefault = isDefault != 0
		out = append(out, kb)
	}
	return out, rows.Err()
}

// GetBase 库详情。
func (s *service) GetBase(ctx context.Context, workdir string, id int64) (KnowledgeBase, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return KnowledgeBase{}, err
	}
	var kb KnowledgeBase
	var isDefault int
	err = conn.QueryRowContext(ctx, `
		SELECT kb.id, kb.project_id, kb.name, kb.description, kb.is_default, kb.created_at, kb.updated_at,
		       COUNT(kbd.document_id)
		FROM knowledge_bases kb
		LEFT JOIN knowledge_base_documents kbd ON kbd.kb_id = kb.id
		WHERE kb.id = ?
		GROUP BY kb.id`, id).Scan(&kb.ID, &kb.ProjectID, &kb.Name, &kb.Description, &isDefault,
		&kb.CreatedAt, &kb.UpdatedAt, &kb.DocCount)
	if errors.Is(err, sql.ErrNoRows) {
		return KnowledgeBase{}, fmt.Errorf("%w: id %d", ErrKnowledgeNotFound, id)
	}
	if err != nil {
		return KnowledgeBase{}, fmt.Errorf("knowledge: get base %d: %w", id, err)
	}
	kb.IsDefault = isDefault != 0
	return kb, nil
}

// CreateBase 创建库（重名 → KNOWLEDGE_INVALID）。
func (s *service) CreateBase(ctx context.Context, workdir, name, description string) (KnowledgeBase, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return KnowledgeBase{}, NewKnowledgeInvalid("知识库名称不能为空")
	}
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return KnowledgeBase{}, err
	}
	now := nowRFC3339()
	var exists int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_bases WHERE project_id = 1 AND name = ?`, name).Scan(&exists); err != nil {
		return KnowledgeBase{}, fmt.Errorf("knowledge: check base name: %w", err)
	}
	if exists > 0 {
		return KnowledgeBase{}, NewKnowledgeInvalid("知识库 %q 已存在", name)
	}
	res, err := conn.ExecContext(ctx,
		`INSERT INTO knowledge_bases (project_id, name, description, is_default, created_at, updated_at)
		 VALUES (1, ?, ?, 0, ?, ?)`, name, description, now, now)
	if err != nil {
		return KnowledgeBase{}, fmt.Errorf("knowledge: create base: %w", err)
	}
	id, _ := res.LastInsertId()
	s.logger.Info("knowledge base created", "workdir", workdir, "id", id, "name", name)
	s.fireWrite(ctx, workdir, "kb_created", fmt.Sprintf("%d", id))
	return s.GetBase(ctx, workdir, id)
}

// UpdateBase 重命名/改描述（默认库可改；is_default 不可转移）。
func (s *service) UpdateBase(ctx context.Context, workdir string, id int64, name, description *string) (KnowledgeBase, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return KnowledgeBase{}, err
	}
	cur, err := s.GetBase(ctx, workdir, id)
	if err != nil {
		return KnowledgeBase{}, err
	}
	newName := cur.Name
	newDesc := cur.Description
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return KnowledgeBase{}, NewKnowledgeInvalid("知识库名称不能为空")
		}
		newName = trimmed
	}
	if description != nil {
		newDesc = *description
	}
	// 重名校验（排除自身）。
	if newName != cur.Name {
		var exists int
		if err := conn.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM knowledge_bases WHERE project_id = 1 AND name = ? AND id != ?`,
			newName, id).Scan(&exists); err != nil {
			return KnowledgeBase{}, fmt.Errorf("knowledge: check base name: %w", err)
		}
		if exists > 0 {
			return KnowledgeBase{}, NewKnowledgeInvalid("知识库 %q 已存在", newName)
		}
	}
	if _, err := conn.ExecContext(ctx,
		`UPDATE knowledge_bases SET name = ?, description = ?, updated_at = ? WHERE id = ?`,
		newName, newDesc, nowRFC3339(), id); err != nil {
		return KnowledgeBase{}, fmt.Errorf("knowledge: update base %d: %w", id, err)
	}
	s.fireWrite(ctx, workdir, "kb_updated", fmt.Sprintf("%d", id))
	return s.GetBase(ctx, workdir, id)
}

// DeleteBase 删除库（仅移除库↔文档边，文档引用/向量/任务关联保留，QA-K14）。
func (s *service) DeleteBase(ctx context.Context, workdir string, id int64) error {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return err
	}
	if _, err := s.GetBase(ctx, workdir, id); err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_base_documents WHERE kb_id = ?`, id); err != nil {
		return fmt.Errorf("knowledge: delete base edges: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_bases WHERE id = ?`, id); err != nil {
		return fmt.Errorf("knowledge: delete base: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("knowledge: commit delete base: %w", err)
	}
	s.fireWrite(ctx, workdir, "kb_deleted", fmt.Sprintf("%d", id))
	return nil
}

// EnsureDefaultBase 确保默认库存在（懒创建，幂等；存量项目补建）。
func (s *service) EnsureDefaultBase(ctx context.Context, workdir string) (KnowledgeBase, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return KnowledgeBase{}, err
	}
	var id int64
	var isDefault int
	err = conn.QueryRowContext(ctx,
		`SELECT id, is_default FROM knowledge_bases WHERE project_id = 1 AND is_default = 1`).Scan(&id, &isDefault)
	if err == nil {
		return s.GetBase(ctx, workdir, id)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return KnowledgeBase{}, fmt.Errorf("knowledge: find default base: %w", err)
	}
	now := nowRFC3339()
	res, err := conn.ExecContext(ctx,
		`INSERT INTO knowledge_bases (project_id, name, description, is_default, created_at, updated_at)
		 VALUES (1, ?, '', 1, ?, ?)`, DefaultKBName, now, now)
	if err != nil {
		return KnowledgeBase{}, fmt.Errorf("knowledge: create default base: %w", err)
	}
	nid, _ := res.LastInsertId()
	s.logger.Info("default knowledge base created lazily", "workdir", workdir, "id", nid)
	return s.GetBase(ctx, workdir, nid)
}

// ---- 文档 CRUD（store.go 继续） ----

// scanDocument 扫描一行文档记录。
func scanDocument(scanner interface{ Scan(...any) error }) (Document, error) {
	var d Document
	var relPath, originPath, mTime, contentHash, summary, status, embedModel, indexErr sql.NullString
	var size int64
	var embedded int
	var history string
	var created, updated string
	err := scanner.Scan(&d.ID, &d.ProjectID, &d.Path, &d.AbsPath, &relPath, &originPath, &d.DisplayName,
		&d.Type, &size, &mTime, &contentHash, &summary, &status, &embedded, &embedModel, &indexErr,
		&history, &created, &updated)
	if err != nil {
		return Document{}, err
	}
	d.RelPath = relPath.String
	d.OriginPath = originPath.String
	d.Size = size
	d.MTime = mTime.String
	d.ContentHash = contentHash.String
	d.Summary = summary.String
	d.Status = status.String
	if d.Status == "" {
		d.Status = DocStatusOK
	}
	d.Embedded = embedded
	d.EmbeddingModel = embedModel.String
	d.IndexError = indexErr.String
	d.CreatedAt = created
	d.UpdatedAt = updated
	if history != "" {
		_ = json.Unmarshal([]byte(history), &d.History)
	}
	return d, nil
}

// documentColumns 文档表查询列（与 scanDocument 对齐）。
const documentColumns = `id, project_id, path, abs_path, rel_path, origin_path, display_name,
	type, size, mtime, content_hash, summary, status, embedded, embedding_model, index_error, history, created_at, updated_at`

// qualifiedDocumentColumns 返回带表别名前缀的文档列清单（联表查询消歧义用）。
func qualifiedDocumentColumns(alias string) string {
	cols := strings.Split(documentColumns, ",")
	for i := range cols {
		cols[i] = alias + "." + strings.TrimSpace(cols[i])
	}
	return strings.Join(cols, ", ")
}

// getDocumentByID 按 id 查文档记录。
func (s *service) getDocumentByID(ctx context.Context, conn *sql.DB, id string) (Document, error) {
	row := conn.QueryRowContext(ctx, `SELECT `+documentColumns+` FROM knowledge_documents WHERE id = ?`, id)
	d, err := scanDocument(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, fmt.Errorf("%w: id %s", ErrDocumentNotFound, id)
	}
	if err != nil {
		return Document{}, fmt.Errorf("knowledge: get document %s: %w", id, err)
	}
	return d, nil
}

// getDocumentByAbs 按 abs_path 查文档记录（注册复用）。
func (s *service) getDocumentByAbs(ctx context.Context, conn *sql.DB, abs string) (Document, error) {
	row := conn.QueryRowContext(ctx,
		`SELECT `+documentColumns+` FROM knowledge_documents WHERE project_id = 1 AND abs_path = ?`, abs)
	d, err := scanDocument(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, nil // 不存在：调用方按需新建
	}
	if err != nil {
		return Document{}, fmt.Errorf("knowledge: get document by abs: %w", err)
	}
	return d, nil
}

// relPathOf 计算相对 workdir 路径；不在项目内返回空串。
func relPathOf(workdir, abs string) string {
	rel, err := filepath.Rel(workdir, abs)
	if err != nil {
		return ""
	}
	if rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel)
}

// uuidv4 生成 UUID v4。
func uuidv4() string {
	return uuid.NewString()
}
