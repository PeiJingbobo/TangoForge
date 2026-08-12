package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
