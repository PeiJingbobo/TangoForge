package knowledge

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"tangoforge/internal/llm"
)

// SearchQuery 检索参数（docs/KNOWLEDGE-BASE.md §2.6）。
type SearchQuery struct {
	Q         string
	KBID      int64
	TopK      int
	Threshold float64
}

// SearchHit 单文档命中结果。
type SearchHit struct {
	Document Document        `json:"document"`
	Score    float32         `json:"score"`
	Chunks   []SearchSnippet `json:"chunks"`
	Missing  bool            `json:"missing"`
}

// SearchSnippet 命中片段（返回 chunk 文本，QA-K9）。
type SearchSnippet struct {
	Heading string  `json:"heading"`
	Text    string  `json:"text"`
	Score   float32 `json:"score"`
	Seq     int     `json:"seq"`
}

// SearchResult 检索结果。
type SearchResult struct {
	Query string      `json:"query"`
	Total int         `json:"total"`
	Items []SearchHit `json:"items"`
}

// 检索默认值与上限。
const (
	defaultTopK       = 10
	maxTopK           = 50
	defaultThreshold  = float32(0.3)
	maxSnippetsPerDoc = 3
)

// Search 向量检索（纯 Go 余弦，全表线性扫描，QA-K22）。
//
// 流程：查询文本嵌入 → 逐 chunk 余弦匹配（过滤 kb_id）→ 文档得分 = 命中 chunk 最大相似度
// → 按得分排序取 topK → 每文档附加命中片段（≤3）→ 懒校验 missing 标记。
// embedding 未配置 → EMBEDDING_NOT_CONFIGURED（422）。
func (s *service) Search(ctx context.Context, workdir string, q SearchQuery) (SearchResult, error) {
	query := strings.TrimSpace(q.Q)
	if query == "" {
		return SearchResult{}, NewDocumentInvalid("检索关键词不能为空")
	}
	if q.TopK <= 0 {
		q.TopK = defaultTopK
	}
	if q.TopK > maxTopK {
		q.TopK = maxTopK
	}
	threshold := float32(q.Threshold)
	if q.Threshold <= 0 {
		threshold = defaultThreshold
	}
	// embedding 配置（由调用方传入，见 search 配置注入——此处通过 service 字段访问）。
	if s.embCfg == nil {
		return SearchResult{}, ErrEmbeddingNotConfigured
	}
	qvec, err := llm.Embedding(ctx, *s.embCfg, query)
	if err != nil {
		return SearchResult{}, err
	}

	conn, err := s.projectDB(ctx, workdir)
	if err != nil {
		return SearchResult{}, err
	}

	// 全表扫描 chunks（kb 过滤用子查询）。
	kbFilter := ""
	args := []any{}
	if q.KBID > 0 {
		kbFilter = ` AND c.document_id IN (SELECT document_id FROM knowledge_base_documents WHERE kb_id = ?)`
		args = append(args, q.KBID)
	}
	rows, err := conn.QueryContext(ctx, `
		SELECT c.document_id, c.seq, c.heading, c.content, c.vector, d.id, d.path, d.abs_path,
		       d.rel_path, d.origin_path, d.display_name, d.type, d.size, d.mtime, d.content_hash,
		       d.summary, d.status, d.embedded, d.embedding_model, d.index_error, d.history,
		       d.created_at, d.updated_at
		FROM knowledge_chunks c
		JOIN knowledge_documents d ON d.id = c.document_id
		WHERE d.embedded = 1 AND d.status != 'failed' AND d.archived = 0`+kbFilter, args...)
	if err != nil {
		return SearchResult{}, fmt.Errorf("knowledge: search scan: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// 文档聚合：docID → 最佳相似度 + 命中断言列表。
	type docAcc struct {
		doc      Document
		best     float32
		snippets []SearchSnippet
	}
	acc := map[string]*docAcc{}
	var order []string
	for rows.Next() {
		var docID string
		var seq int
		var heading, content string
		var vector []byte
		var d Document
		var relPath, originPath, mTime, contentHash, summary, status, embedModel, indexErr, history sql.NullString
		var size int64
		var embedded int
		var created, updated string
		if err := rows.Scan(&docID, &seq, &heading, &content, &vector, &d.ID, &d.Path, &d.AbsPath,
			&relPath, &originPath, &d.DisplayName, &d.Type, &size, &mTime, &contentHash, &summary,
			&status, &embedded, &embedModel, &indexErr, &history, &created, &updated); err != nil {
			return SearchResult{}, fmt.Errorf("knowledge: scan search row: %w", err)
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

		score := cosineSimilarity(qvec, decodeVectorF32(vector))
		if score < threshold {
			continue
		}
		a, ok := acc[docID]
		if !ok {
			a = &docAcc{doc: d}
			acc[docID] = a
			order = append(order, docID)
		}
		if score > a.best {
			a.best = score
		}
		if len(a.snippets) < maxSnippetsPerDoc {
			a.snippets = append(a.snippets, SearchSnippet{
				Heading: heading, Text: content, Score: score, Seq: seq,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return SearchResult{}, fmt.Errorf("knowledge: search rows: %w", err)
	}

	// 排序：按文档最佳得分降序取 topK。
	// 简单选择排序（文档量 < 5k，O(n²) 可接受；保持纯 Go 零依赖）。
	// 用稳定排序实现：直接收集切片后冒泡/插入。
	items := make([]SearchHit, 0, len(order))
	for _, docID := range order {
		a := acc[docID]
		items = append(items, SearchHit{Document: a.doc, Score: a.best, Chunks: a.snippets})
	}
	// 稳定降序（同分保持扫描顺序）。
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].Score > items[j-1].Score; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
	if len(items) > q.TopK {
		items = items[:q.TopK]
	}

	// 懒校验 missing 标记（检索前 stat，QA-K9）。
	for i := range items {
		if _, err := os.Stat(items[i].Document.AbsPath); err != nil {
			items[i].Missing = true
			items[i].Document.Status = DocStatusMissing
		}
	}
	return SearchResult{Query: query, Total: len(items), Items: items}, nil
}

// optionalKB 返回 kb 过滤参数（0 → 无过滤，append 空）。
func optionalKB(kbID int64) any {
	if kbID > 0 {
		return kbID
	}
	return nil
}
