package knowledge

import (
	"errors"
	"fmt"
)

// 业务错误码（docs/KNOWLEDGE-BASE.md §11）。
const (
	CodeKnowledgeNotFound      = "KNOWLEDGE_NOT_FOUND"
	CodeKnowledgeInvalid       = "KNOWLEDGE_INVALID"
	CodeDocumentNotFound       = "DOCUMENT_NOT_FOUND"
	CodeDocumentInvalid        = "DOCUMENT_INVALID"
	CodeDocumentMissing        = "DOCUMENT_MISSING"
	CodeCopyFailed             = "COPY_FAILED"
	CodeEmbeddingNotConfigured = "EMBEDDING_NOT_CONFIGURED"
	CodeEmbeddingFailed        = "EMBEDDING_FAILED"
	CodeSummaryFailed          = "SUMMARY_FAILED"
	CodeIndexFailed            = "INDEX_FAILED"
)

// Error 业务错误：携带机器可读 Code 与人类可读 Message（模式同 internal/task）。
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Is 支持 errors.Is 按 Code 判等。
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t != nil && t.Code == e.Code
}

// 哨兵错误。
var (
	ErrKnowledgeNotFound      = &Error{Code: CodeKnowledgeNotFound, Message: "知识库不存在"}
	ErrKnowledgeInvalid       = &Error{Code: CodeKnowledgeInvalid, Message: "知识库参数非法"}
	ErrDocumentNotFound       = &Error{Code: CodeDocumentNotFound, Message: "文档记录不存在"}
	ErrDocumentInvalid        = &Error{Code: CodeDocumentInvalid, Message: "文档参数非法"}
	ErrDocumentMissing        = &Error{Code: CodeDocumentMissing, Message: "目标文件不可达"}
	ErrCopyFailed             = &Error{Code: CodeCopyFailed, Message: "外部文件拷贝失败"}
	ErrEmbeddingNotConfigured = &Error{Code: CodeEmbeddingNotConfigured, Message: "llm.embedding 未配置"}
	ErrEmbeddingFailed        = &Error{Code: CodeEmbeddingFailed, Message: "embedding 调用失败"}
	ErrSummaryFailed          = &Error{Code: CodeSummaryFailed, Message: "摘要生成失败"}
	ErrIndexFailed            = &Error{Code: CodeIndexFailed, Message: "索引流水线失败"}
)

// NewKnowledgeInvalid 构造携带具体原因的 KNOWLEDGE_INVALID 错误。
func NewKnowledgeInvalid(format string, args ...any) error {
	return &Error{Code: CodeKnowledgeInvalid, Message: fmt.Sprintf(format, args...)}
}

// NewDocumentInvalid 构造携带具体原因的 DOCUMENT_INVALID 错误。
func NewDocumentInvalid(format string, args ...any) error {
	return &Error{Code: CodeDocumentInvalid, Message: fmt.Sprintf(format, args...)}
}

// NewDocumentMissing 构造携带具体原因的 DOCUMENT_MISSING 错误。
func NewDocumentMissing(format string, args ...any) error {
	return &Error{Code: CodeDocumentMissing, Message: fmt.Sprintf(format, args...)}
}

// CodeOf 提取错误的业务码；非知识库域错误返回空串。
func CodeOf(err error) string {
	var ke *Error
	if errors.As(err, &ke) {
		return ke.Code
	}
	return ""
}
