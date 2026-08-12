package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"tangoforge/internal/config"
)

// 拷贝模式（docs/KNOWLEDGE-BASE.md §2.1/§2.5，QA-K2）。
const (
	CopyNone = "none" // 原样引用，不拷贝
	CopyCopy = "copy" // 外部文件一律拷贝进默认文档目录
	CopyAuto = "auto" // 缺省：项目内原样引用，项目外拷贝
)

// validCopyMode 校验拷贝模式；空 = auto。
func validCopyMode(m string) string {
	switch m {
	case CopyNone, CopyCopy:
		return m
	default:
		return CopyAuto
	}
}

// isTextFile 判定文本类型（QA-K3：UTF-8 文本全支持；二进制仅注册）。
//
// 判定依据：扩展名黑名单 + 内容嗅探。无扩展名/未知扩展名按文本处理（读取失败回退二进制）。
// 文本：.md/.markdown/.txt/.go/.ts/.tsx/.js/.jsx/.py/.rs/.yaml/.yml/.json/.toml/.html/.css/
//
//	.sh/.bash/.sql/.c/.h/.cpp/.java/.rb/.php/.vue/.svelte/.proto/.mod/.sum/.tmpl/.env 等。
//
// 二进制：图片/文档/压缩/可执行等已知二进制扩展名。
func isTextFile(path string, size int64) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".bmp", ".tiff",
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar",
		".exe", ".dll", ".so", ".dylib", ".bin", ".class", ".jar",
		".mp3", ".mp4", ".mov", ".avi", ".wav", ".flac", ".ogg", ".mkv", ".webm",
		".woff", ".woff2", ".ttf", ".otf", ".eot":
		return false
	}
	// 已知文本扩展名 → 文本。
	switch ext {
	case ".md", ".markdown", ".txt", ".text", ".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs",
		".py", ".rs", ".yaml", ".yml", ".json", ".toml", ".ini", ".cfg", ".conf",
		".html", ".htm", ".css", ".scss", ".sass", ".less", ".xml", ".csv",
		".sh", ".bash", ".zsh", ".fish", ".sql", ".c", ".h", ".cpp", ".hpp", ".cc",
		".java", ".kt", ".rb", ".php", ".vue", ".svelte", ".proto", ".mod", ".sum",
		".tmpl", ".env", ".gitignore", ".dockerignore", ".lock", ".log", ".diff", ".patch",
		".properties", ".plist", ".gradle", ".swift", ".m", ".mm":
		return true
	}
	// 无扩展名/未知：内容嗅探（前 512 字节含 NUL → 二进制）。
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	for _, b := range buf[:n] {
		if b == 0 {
			return false
		}
	}
	_ = size
	return true
}

// RegisterDocument 注册文档（存在则复用并返回既有记录，QA-K16）。
//
// path 为磁盘路径（绝对或相对 workdir）；copy 决定外部文件是否拷贝进默认文档目录。
// kbIDs 为要加入的库 id（空 = 加入默认库）。
func (s *service) RegisterDocument(ctx context.Context, workdir, path, copyMode string, kbIDs []int64) (Document, error) {
	clean := filepath.Clean(workdir)
	copyMode = validCopyMode(copyMode)

	abs, err := filepath.Abs(path)
	if err != nil {
		return Document{}, NewDocumentInvalid("路径非法: %s", path)
	}
	// 相对路径先相对 workdir 解析（显式优于隐式：禁止依赖进程 cwd）。
	if !filepath.IsAbs(path) {
		abs = filepath.Join(clean, filepath.FromSlash(path))
	}
	abs = filepath.Clean(abs)
	if _, err := os.Stat(abs); err != nil {
		return Document{}, NewDocumentMissing("目标文件不可达: %s", path)
	}

	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return Document{}, err
	}

	// 已在项目内 → 原样引用（不拷贝）。
	rel := relPathOf(clean, abs)
	insideProject := rel != ""

	// 外部文件 → 按 copy 模式决定是否拷贝。
	origin := ""
	refPath := abs
	if !insideProject && copyMode != CopyNone {
		if copyMode == CopyAuto || copyMode == CopyCopy {
			destDir, err := s.resolveDefaultDocDir(ctx, workdir)
			if err != nil {
				return Document{}, err
			}
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				return Document{}, fmt.Errorf("knowledge: mkdir %s: %w", destDir, err)
			}
			dest := filepath.Join(destDir, filepath.Base(abs))
			if err := copyFile(abs, dest); err != nil {
				return Document{}, fmt.Errorf("%w: %v", ErrCopyFailed, err)
			}
			origin = abs
			abs = dest
			rel = relPathOf(clean, dest)
			refPath = dest
		}
	}
	// 重新规范化拷贝后路径。
	if abs2, err := filepath.Abs(abs); err == nil {
		abs = filepath.Clean(abs2)
		refPath = abs
		if rel == "" {
			rel = relPathOf(clean, abs)
		}
	}

	// 已注册 → 复用（并把 kbIDs 加入对应库）。
	if existing, err := s.getDocumentByAbs(ctx, conn, abs); err != nil {
		return Document{}, err
	} else if existing.ID != "" {
		if err := s.addToBases(ctx, conn, workdir, existing.ID, kbIDs); err != nil {
			return Document{}, err
		}
		return existing, nil
	}

	// 新建文档记录。
	now := nowRFC3339()
	fi, err := os.Stat(abs)
	if err != nil {
		return Document{}, NewDocumentMissing("目标文件不可达: %s", abs)
	}
	typ := DocTypeText
	if !isTextFile(abs, fi.Size()) {
		typ = DocTypeBinary
	}
	// 文档数据源路径：项目内 → refPath（相对或绝对）；外部不拷贝 → 绝对路径。
	pathCol := refPath
	if insideProject || rel != "" {
		pathCol = rel
	}
	doc := Document{
		ID:          uuidv4(),
		ProjectID:   1,
		Path:        pathCol,
		AbsPath:     abs,
		RelPath:     rel,
		OriginPath:  origin,
		DisplayName: filepath.Base(abs),
		Type:        typ,
		Size:        fi.Size(),
		MTime:       fi.ModTime().Format(timeRFC3339),
		Status:      DocStatusOK,
		Embedded:    EmbedNo,
		History:     []PathHistory{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	histJSON := "[]"
	_, err = conn.ExecContext(ctx, `
		INSERT INTO knowledge_documents
			(id, project_id, path, abs_path, rel_path, origin_path, display_name, type, size, mtime,
			 content_hash, summary, status, embedded, embedding_model, index_error, history, created_at, updated_at)
		VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, ?, '', '', ?, 0, '', '', ?, ?, ?)`,
		doc.ID, doc.Path, doc.AbsPath, sqlNull(doc.RelPath), sqlNull(doc.OriginPath), doc.DisplayName,
		doc.Type, doc.Size, sqlNull(doc.MTime), doc.Status, histJSON, now, now)
	if err != nil {
		// 并发注册竞态：唯一约束冲突 → 读既有记录。
		if isUniqueViolation(err) {
			if existing, err2 := s.getDocumentByAbs(ctx, conn, abs); err2 == nil && existing.ID != "" {
				return existing, nil
			}
		}
		return Document{}, fmt.Errorf("knowledge: insert document: %w", err)
	}
	if err := s.addToBases(ctx, conn, workdir, doc.ID, kbIDs); err != nil {
		return Document{}, err
	}
	s.logger.Info("knowledge document registered", "workdir", workdir, "id", doc.ID, "abs", doc.AbsPath)
	s.fireWrite(ctx, workdir, "document_added", doc.ID)
	return doc, nil
}

// addToBases 将文档加入指定库；kbIDs 为空时加入默认库（幂等：已存在跳过）。
func (s *service) addToBases(ctx context.Context, conn *sql.DB, workdir, docID string, kbIDs []int64) error {
	ids := kbIDs
	if len(ids) == 0 {
		kb, err := s.EnsureDefaultBase(ctx, workdir)
		if err != nil {
			return err
		}
		ids = []int64{kb.ID}
	}
	now := nowRFC3339()
	for _, kbID := range ids {
		// 校验库存在。
		var n int
		if err := conn.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM knowledge_bases WHERE id = ?`, kbID).Scan(&n); err != nil {
			return fmt.Errorf("knowledge: check base %d: %w", kbID, err)
		}
		if n == 0 {
			return fmt.Errorf("%w: id %d", ErrKnowledgeNotFound, kbID)
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT OR IGNORE INTO knowledge_base_documents (kb_id, document_id, created_at) VALUES (?, ?, ?)`,
			kbID, docID, now); err != nil {
			return fmt.Errorf("knowledge: link base %d: %w", kbID, err)
		}
	}
	return nil
}

// resolveDefaultDocDir 解析外部文件默认拷贝目录（QA-K13）：
// 项目 config.yaml knowledge.default_doc_dir → 全局逻辑（TF-048 落地隐式检测）→ .taskboard/knowledge。
// 当前实现：项目配置覆盖 + 默认 .taskboard/knowledge。
func (s *service) resolveDefaultDocDir(ctx context.Context, workdir string) (string, error) {
	pc, err := config.LoadProject(workdir)
	if err != nil {
		return "", fmt.Errorf("knowledge: load project config: %w", err)
	}
	dir := pc.Knowledge.DefaultDocDir
	if dir == "" {
		dir = filepath.Join(workdir, ".taskboard", "knowledge")
	} else if !filepath.IsAbs(dir) {
		dir = filepath.Join(workdir, dir)
	}
	return filepath.Clean(dir), nil
}

// copyFile 复制文件（保留模式位）。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// DeleteDocument 解除引用（删文档 + chunks + 库边 + 任务边，仅 knowledge.write 可操作）。
func (s *service) DeleteDocument(ctx context.Context, workdir, id string) error {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return err
	}
	if _, err := s.getDocumentByID(ctx, conn, id); err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range []string{
		`DELETE FROM knowledge_chunks WHERE document_id = ?`,
		`DELETE FROM knowledge_base_documents WHERE document_id = ?`,
		`DELETE FROM task_documents WHERE document_id = ?`,
		`DELETE FROM knowledge_documents WHERE id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, stmt, id); err != nil {
			return fmt.Errorf("knowledge: delete document %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("knowledge: commit delete document: %w", err)
	}
	s.fireWrite(ctx, workdir, "document_removed", id)
	return nil
}

// RelinkDocument 重新链接（QA-K15）：校验新路径 → 更新 abs_path/rel_path/display_name →
// history 追加旧路径 → 重置并重建索引（清摘要+向量）→ 清除 missing，保留库成员与任务关联。
func (s *service) RelinkDocument(ctx context.Context, workdir, id, newPath, copyMode string) (Document, error) {
	copyMode = validCopyMode(copyMode)
	clean := filepath.Clean(workdir)

	abs, err := filepath.Abs(newPath)
	if err != nil {
		return Document{}, NewDocumentInvalid("新路径非法: %s", newPath)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return Document{}, NewDocumentMissing("新路径不可达: %s", newPath)
	}
	if fi.IsDir() {
		return Document{}, NewDocumentInvalid("新路径是目录: %s", newPath)
	}
	if !isTextFile(abs, fi.Size()) {
		return Document{}, NewDocumentInvalid("新路径非文本文件: %s", newPath)
	}

	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return Document{}, err
	}
	cur, err := s.getDocumentByID(ctx, conn, id)
	if err != nil {
		return Document{}, err
	}

	// 新路径唯一性：不能被其它文档占用。
	if other, err := s.getDocumentByAbs(ctx, conn, abs); err != nil {
		return Document{}, err
	} else if other.ID != "" && other.ID != id {
		return Document{}, NewDocumentInvalid("新路径已被文档 %s 注册", other.ID)
	}

	rel := relPathOf(clean, abs)
	origin := cur.OriginPath
	pathCol := rel
	if rel == "" {
		pathCol = abs
	}
	if copyMode != CopyNone && rel == "" {
		// 外部文件 → 拷贝进默认文档目录。
		destDir, err := s.resolveDefaultDocDir(ctx, workdir)
		if err != nil {
			return Document{}, err
		}
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return Document{}, fmt.Errorf("knowledge: mkdir %s: %w", destDir, err)
		}
		dest := filepath.Join(destDir, filepath.Base(abs))
		if err := copyFile(abs, dest); err != nil {
			return Document{}, fmt.Errorf("%w: %v", ErrCopyFailed, err)
		}
		origin = abs
		abs = dest
		rel = relPathOf(clean, dest)
		pathCol = rel
	}

	// history 追加旧路径。
	hist := cur.History
	if cur.AbsPath != "" && cur.AbsPath != abs {
		hist = append(hist, PathHistory{Path: cur.AbsPath, RelinkedAt: nowRFC3339()})
	}
	histJSON, _ := json.Marshal(hist)
	now := nowRFC3339()

	// 事务：更新文档 + 重置索引（删旧 chunks）+ 清摘要。
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE knowledge_documents
		SET path = ?, abs_path = ?, rel_path = ?, origin_path = ?, display_name = ?,
		    size = ?, mtime = ?, content_hash = '', summary = '', status = ?, embedded = 0,
		    embedding_model = '', index_error = '', history = ?, updated_at = ?
		WHERE id = ?`,
		pathCol, abs, sqlNull(rel), sqlNull(origin), filepath.Base(abs),
		fi.Size(), fi.ModTime().Format(timeRFC3339), DocStatusIndexing, histJSON, now, id); err != nil {
		return Document{}, fmt.Errorf("knowledge: relink update: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_chunks WHERE document_id = ?`, id); err != nil {
		return Document{}, fmt.Errorf("knowledge: relink reset chunks: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Document{}, fmt.Errorf("knowledge: commit relink: %w", err)
	}
	s.fireWrite(ctx, workdir, "document_relinked", id)
	// 触发异步索引（TF-047 接入；当前返回已重置文档，索引状态 indexing）。
	return s.getDocumentByID(ctx, conn, id)
}

// LinkTask 任务关联（§2.5，QA-K16）：task_id + document_id 或 path。
// path 未注册 → 自动入库（注册 + 关联）；参数 copy/kb_ids 仅 path 形态生效。
func (s *service) LinkTask(ctx context.Context, workdir, taskID, documentID, path, copyMode string, kbIDs []int64) error {
	if taskID == "" {
		return NewDocumentInvalid("task_id 必填")
	}
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return err
	}
	// 任务存在性校验。
	if s.tasks != nil {
		if _, err := s.tasks.Get(ctx, workdir, taskID); err != nil {
			return fmt.Errorf("知识库关联: 任务校验失败: %w", err)
		}
	}

	docID := documentID
	if docID == "" && path != "" {
		doc, err := s.RegisterDocument(ctx, workdir, path, copyMode, kbIDs)
		if err != nil {
			return err
		}
		docID = doc.ID
	}
	if docID == "" {
		return NewDocumentInvalid("document_id 或 path 必填其一")
	}
	if _, err := s.getDocumentByID(ctx, conn, docID); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT OR IGNORE INTO task_documents (task_id, document_id, created_at) VALUES (?, ?, ?)`,
		taskID, docID, nowRFC3339()); err != nil {
		return fmt.Errorf("knowledge: link task: %w", err)
	}
	s.fireWrite(ctx, workdir, "task_linked", taskID)
	return nil
}

// UnlinkTask 解除任务关联（文档本身保留）。
func (s *service) UnlinkTask(ctx context.Context, workdir, taskID, documentID string) error {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return err
	}
	res, err := conn.ExecContext(ctx,
		`DELETE FROM task_documents WHERE task_id = ? AND document_id = ?`, taskID, documentID)
	if err != nil {
		return fmt.Errorf("knowledge: unlink task: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return NewDocumentInvalid("任务与文档之间无关联")
	}
	s.fireWrite(ctx, workdir, "task_unlinked", taskID)
	return nil
}

// TaskDocuments 任务关联的文档列表。
func (s *service) TaskDocuments(ctx context.Context, workdir, taskID string) ([]Document, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx, `
		SELECT `+qualifiedDocumentColumns("d")+` FROM knowledge_documents d
		JOIN task_documents td ON td.document_id = d.id
		WHERE td.task_id = ? ORDER BY d.display_name`, taskID)
	if err != nil {
		return nil, fmt.Errorf("knowledge: task documents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, fmt.Errorf("knowledge: scan task document: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListDocuments 文档列表（filter[kb_id]/filter[status]/q/分页，§6）。
func (s *service) ListDocuments(ctx context.Context, workdir string, f DocumentFilter) (DocumentListResult, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return DocumentListResult{}, err
	}
	page := f.Page
	if page < 0 {
		page = 0
	}
	size := f.Size
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}

	var conds []string
	var args []any
	if f.KBID > 0 {
		conds = append(conds, `d.id IN (SELECT document_id FROM knowledge_base_documents WHERE kb_id = ?)`)
		args = append(args, f.KBID)
	}
	if f.Status != "" {
		conds = append(conds, `d.status = ?`)
		args = append(args, f.Status)
	}
	if f.Q != "" {
		conds = append(conds, `(d.display_name LIKE ? OR d.path LIKE ? OR d.abs_path LIKE ?)`)
		like := "%" + f.Q + "%"
		args = append(args, like, like, like)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_documents d`+where, args...).Scan(&total); err != nil {
		return DocumentListResult{}, fmt.Errorf("knowledge: count documents: %w", err)
	}

	query := `SELECT ` + qualifiedDocumentColumns("d") + ` FROM knowledge_documents d` + where +
		` ORDER BY d.updated_at DESC LIMIT ? OFFSET ?`
	offset := page * size
	args = append(args, size, offset)
	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return DocumentListResult{}, fmt.Errorf("knowledge: list documents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var items []Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return DocumentListResult{}, fmt.Errorf("knowledge: scan document: %w", err)
		}
		items = append(items, d)
	}
	if err := rows.Err(); err != nil {
		return DocumentListResult{}, err
	}
	return DocumentListResult{Items: items, Total: total, Page: page, Size: size}, nil
}

// GetDocument 文档详情 + 真实路径 + 关联任务数/库列表（§6）。
func (s *service) GetDocument(ctx context.Context, workdir, id string) (Document, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return Document{}, err
	}
	d, err := s.getDocumentByID(ctx, conn, id)
	if err != nil {
		return Document{}, err
	}
	var taskCount int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM task_documents WHERE document_id = ?`, id).Scan(&taskCount); err != nil {
		return Document{}, fmt.Errorf("knowledge: count task links: %w", err)
	}
	d.TaskCount = taskCount
	rows, err := conn.QueryContext(ctx,
		`SELECT kb_id FROM knowledge_base_documents WHERE document_id = ? ORDER BY kb_id`, id)
	if err != nil {
		return Document{}, fmt.Errorf("knowledge: list doc bases: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var kbID int64
		if err := rows.Scan(&kbID); err != nil {
			return Document{}, fmt.Errorf("knowledge: scan doc base: %w", err)
		}
		d.KBs = append(d.KBs, kbID)
	}
	return d, rows.Err()
}

// ---- 工具函数 ----

// sqlNull 字符串转 sql.NullString（空串 → INVALID）。
func sqlNull(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}

// isUniqueViolation 判断 SQLite 唯一约束冲突（busy/constraint 场景）。
func isUniqueViolation(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "UNIQUE constraint failed") ||
		strings.Contains(err.Error(), "constraint failed"))
}

// timeRFC3339 时间格式（与 nowRFC3339 一致的 RFC3339 本地时区）。
const timeRFC3339 = "2006-01-02T15:04:05Z07:00"
