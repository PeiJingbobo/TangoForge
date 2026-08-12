package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"tangoforge/internal/llm"
	"time"
)

// summarySystemPrompt 摘要生成系统提示（≤200 字中文摘要）。
const summarySystemPrompt = `你是文档摘要助手。请用简洁的中文为给定文档生成摘要，不超过 200 字。
摘要需概括文档主题、核心内容与关键结论，输出严格 JSON：{"summary": "..."}`

// summarySchema 摘要输出 JSON Schema（prompt 约束用）。
const summarySchema = `{"type":"object","required":["summary"],"properties":{"summary":{"type":"string"}}}`

// maxSummaryChars 摘要长度上限。
const maxSummaryChars = 200

// GenerateSummary 用 LLM 为文档文本生成摘要（CompleteJSON，≤200 字）。
//
// 语义（docs/KNOWLEDGE-BASE.md §9）：摘要失败 → 返回空字符串，不阻断嵌入（摘要与向量解耦）。
// llm 未配置 / 超时 / 响应非法 → 空字符串 + nil error（降级为仅文件名参与分析，QA-K4）。
func GenerateSummary(ctx context.Context, cl *llm.Client, text string) string {
	if cl == nil || strings.TrimSpace(text) == "" {
		return ""
	}
	// 输入截断（索引上限内文本已由调用方截断，这里防御）。
	const maxInput = 100000 // 摘要输入 100KB 上限（避免超长文档爆 token）
	if len(text) > maxInput {
		text = text[:maxInput]
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	raw, err := cl.CompleteJSON(ctx, llm.Request{
		System:      summarySystemPrompt,
		User:        "文档内容：\n\n" + text,
		RequireJSON: true,
		Schema:      summarySchema,
	})
	if err != nil {
		return ""
	}
	var out struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return ""
	}
	out.Summary = strings.TrimSpace(out.Summary)
	if len([]rune(out.Summary)) > maxSummaryChars {
		out.Summary = string([]rune(out.Summary)[:maxSummaryChars])
	}
	return out.Summary
}

// SummarizeAndCache 生成摘要并按 content_hash 写入文档记录（缓存 key = content_hash，QA-K13）。
//
// 仅当文档 summary 为空或缓存 hash 与当前文件 hash 不一致时调用 LLM。
// 返回生成的摘要（可能为空字符串 = LLM 失败，不更新缓存）。
func (s *service) SummarizeAndCache(ctx context.Context, workdir, docID, contentHash, text string) (string, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return "", err
	}
	if s.llmClient == nil || strings.TrimSpace(contentHash) == "" {
		return "", nil
	}
	// 缓存命中：文档已有摘要且 hash 一致 → 跳过 LLM。
	var cached string
	var cachedHash sql.NullString
	if err := conn.QueryRowContext(ctx,
		`SELECT summary, content_hash FROM knowledge_documents WHERE id = ?`, docID).Scan(&cached, &cachedHash); err != nil {
		return "", fmt.Errorf("knowledge: read summary cache: %w", err)
	}
	if cached != "" && cachedHash.String == contentHash {
		return cached, nil
	}
	summary := GenerateSummary(ctx, s.llmClient, text)
	if summary == "" {
		return "", nil // LLM 失败不更新缓存（下次重试）
	}
	if _, err := conn.ExecContext(ctx,
		`UPDATE knowledge_documents SET summary = ?, content_hash = ?, updated_at = ? WHERE id = ?`,
		summary, contentHash, nowRFC3339(), docID); err != nil {
		return "", fmt.Errorf("knowledge: cache summary: %w", err)
	}
	return summary, nil
}

// IsSummaryFailed 判断摘要生成失败（返回空串表示降级，不视为业务错误）。
func IsSummaryFailed(summary string) bool { return summary == "" }

// IndexOptions 索引任务参数。
type IndexOptions struct {
	// ContentHash 当前文件 sha256；空 = 不更新 hash（未知）。
	ContentHash string
	// Embedding 嵌入运行配置（nil = 不嵌入，仅分块/摘要）。
	Embedding *llm.EmbeddingConfig
	// MaxIndexSize 超过该大小的文件不嵌入（仅注册 + 摘要，§2.7）。
	MaxIndexSize int
	// ForceReembed 强制重嵌入（模型漂移检测 / relink 重置后）。
	ForceReembed bool
}

// IndexResult 索引结果摘要（供审计/WS 事件）。
type IndexResult struct {
	Chunks     int  `json:"chunks"`
	Embedded   bool `json:"embedded"`
	Summarized bool `json:"summarized"`
	Skipped    bool `json:"skipped"` // 超限跳过嵌入
}

// IndexDocument 对文档执行完整索引流水线（docs/KNOWLEDGE-BASE.md §9）：
//
//	读文件 → 分块（标题分块 + 大小兜底）→ 摘要（LLM，失败不阻断）
//	→ 逐 chunk 嵌入（llm.Embedding）→ 写 knowledge_chunks + 更新文档元数据。
//
// 失败容错：嵌入失败 → 文档 status=failed + index_error（下次扫描重试）；
// 摘要失败 → summary="" 文档仍嵌入（解耦，§9）。
func (s *service) IndexDocument(ctx context.Context, workdir, docID string, opts IndexOptions) (IndexResult, error) {
	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return IndexResult{}, err
	}
	doc, err := s.getDocumentByID(ctx, conn, docID)
	if err != nil {
		return IndexResult{}, err
	}
	// 二进制不索引。
	if doc.Type == DocTypeBinary {
		return IndexResult{Embedded: false}, nil
	}
	// 文件缺失 → missing 状态（内容/向量保留，检索仍命中并标注）。
	if _, err := os.Stat(doc.AbsPath); err != nil {
		if _, uerr := conn.ExecContext(ctx,
			`UPDATE knowledge_documents SET status = ?, updated_at = ? WHERE id = ?`,
			DocStatusMissing, nowRFC3339(), docID); uerr != nil {
			return IndexResult{}, fmt.Errorf("knowledge: mark missing: %w", uerr)
		}
		return IndexResult{Embedded: false}, nil
	}
	// 读文件（max_index_size 限制仅影响嵌入，摘要输入单独截断）。
	text, err := readTextFile(doc.AbsPath)
	if err != nil {
		s.logger.Warn("knowledge: read document failed", "id", docID, "path", doc.AbsPath, "err", err)
		return IndexResult{}, fmt.Errorf("knowledge: read %s: %w", doc.AbsPath, err)
	}

	// 分块。
	chunks := splitChunks(text)
	if len(chunks) == 0 {
		// 空文档：清空旧向量，标记 ok（无内容可索引）。
		if _, err := conn.ExecContext(ctx,
			`DELETE FROM knowledge_chunks WHERE document_id = ?`, docID); err != nil {
			return IndexResult{}, fmt.Errorf("knowledge: clear chunks: %w", err)
		}
		if _, err := conn.ExecContext(ctx,
			`UPDATE knowledge_documents SET status = ?, content_hash = ?, updated_at = ? WHERE id = ?`,
			DocStatusOK, opts.ContentHash, nowRFC3339(), docID); err != nil {
			return IndexResult{}, fmt.Errorf("knowledge: update empty doc: %w", err)
		}
		return IndexResult{Embedded: false}, nil
	}

	// 摘要（失败不阻断）。
	summary := ""
	if s.llmClient != nil {
		summary, _ = s.SummarizeAndCache(ctx, workdir, docID, opts.ContentHash, text)
	}

	// 嵌入。
	res := IndexResult{Chunks: len(chunks), Summarized: summary != ""}
	overLimit := opts.MaxIndexSize > 0 && doc.Size > int64(opts.MaxIndexSize)
	if opts.Embedding == nil || overLimit {
		// 未配置嵌入 / 超限：仅注册 + 摘要（QA-K8 max_index_size）。
		if _, err := conn.ExecContext(ctx, `
			UPDATE knowledge_documents
			SET status = ?, content_hash = ?, index_error = ?, updated_at = ?
			WHERE id = ?`,
			DocStatusOK, opts.ContentHash, overLimitNote(overLimit, opts.MaxIndexSize), nowRFC3339(), docID); err != nil {
			return res, fmt.Errorf("knowledge: update doc: %w", err)
		}
		res.Skipped = overLimit
		return res, nil
	}

	// 逐 chunk 嵌入（串行，singleflight 由调用方保证）。
	// 先清空旧向量（重索引幂等）。
	if _, err := conn.ExecContext(ctx, `DELETE FROM knowledge_chunks WHERE document_id = ?`, docID); err != nil {
		return res, fmt.Errorf("knowledge: clear old chunks: %w", err)
	}
	model := opts.Embedding.Model
	now := nowRFC3339()
	for i, ck := range chunks {
		vec, err := llm.Embedding(ctx, *opts.Embedding, ck.Content)
		if err != nil {
			// 嵌入失败 → 文档 failed + index_error（下次扫描重试）。
			if _, uerr := conn.ExecContext(ctx, `
				UPDATE knowledge_documents SET status = ?, index_error = ?, updated_at = ? WHERE id = ?
				`, DocStatusFailed, err.Error(), now, docID); uerr != nil {
				return res, fmt.Errorf("knowledge: mark failed: %w", uerr)
			}
			s.logger.Warn("knowledge: embed chunk failed", "id", docID, "seq", i, "err", err)
			return res, fmt.Errorf("knowledge: embed chunk %d: %w", i, err)
		}
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO knowledge_chunks (id, document_id, seq, heading, content, vector, dim, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			uuidv4(), docID, i, ck.Heading, ck.Content, encodeVectorF32(vec), len(vec), now); err != nil {
			return res, fmt.Errorf("knowledge: insert chunk %d: %w", i, err)
		}
	}
	// 全部成功 → ok + embedded=1 + embedding_model。
	if _, err := conn.ExecContext(ctx, `
		UPDATE knowledge_documents
		SET status = ?, content_hash = ?, embedded = 1, embedding_model = ?, index_error = '', updated_at = ?
		WHERE id = ?`,
		DocStatusOK, opts.ContentHash, model, now, docID); err != nil {
		return res, fmt.Errorf("knowledge: finalize doc: %w", err)
	}
	res.Embedded = true
	return res, nil
}

// overLimitNote 生成超限说明（index_error 字段语义）。
func overLimitNote(over bool, limit int) string {
	if over {
		return "too_large"
	}
	return ""
}

// readTextFile 读取文本文件（UTF-8；上限 2MB 防御，超限截断）。
func readTextFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	const maxRead = 2 << 20 // 2MB
	if len(data) > maxRead {
		data = data[:maxRead]
	}
	return string(data), nil
}
