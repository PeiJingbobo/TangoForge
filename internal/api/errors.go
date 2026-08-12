package api

import (
	"errors"
	"net/http"
	"tangoforge/internal/audit"
	"tangoforge/internal/auth"
	"tangoforge/internal/exporter"
	"tangoforge/internal/knowledge"
	"tangoforge/internal/llm"
	"tangoforge/internal/parser"
	"tangoforge/internal/project"
	"tangoforge/internal/skill"
	"tangoforge/internal/task"
)

// isErr 判断错误是否与目标哨兵相等（errors.Is 简写）。
func isErr(err error, target error) bool { return err != nil && errors.Is(err, target) }

// mapError 将业务错误映射为统一错误响应（QA P3-7 映射表，登记 TASK-SEMANTICS §10）。
//
// 映射规则：
//   - 404: PROJECT_NOT_FOUND / TASK_NOT_FOUND / PARENT_NOT_FOUND
//   - 400: TASK_INVALID / DELETE_NOT_ALLOWED / PROJECT_INVALID（目录非法）
//   - 422: INVALID_TRANSITION / STATUS_IN_USE / STATUS_NOT_FOUND /
//     PARENT_CYCLE / DEPENDENCY_NOT_FOUND / CIRCULAR_DEPENDENCY
//   - 403: PERMISSION_DENIED（权限中间件直接返回，不经此处）
//   - 500: INTERNAL（未识别错误）
func mapError(err error) (status int, code, message string) {
	var te *task.Error
	if errors.As(err, &te) {
		switch te.Code {
		case task.CodeProjectNotFound:
			return http.StatusNotFound, te.Code, te.Message
		case task.CodeTaskNotFound, task.CodeParentNotFound:
			return http.StatusNotFound, te.Code, te.Message
		case task.CodeTaskInvalid, task.CodeDeleteNotAllowed:
			return http.StatusBadRequest, te.Code, te.Message
		case task.CodeInvalidTransition, task.CodeStatusInUse, task.CodeStatusNotFound,
			task.CodeParentCycle, task.CodeDependencyNotFound, task.CodeCircularDependency:
			return http.StatusUnprocessableEntity, te.Code, te.Message
		default:
			return http.StatusInternalServerError, te.Code, te.Message
		}
	}
	switch {
	case errors.Is(err, auth.ErrProjectNotFound), errors.Is(err, audit.ErrProjectNotFound),
		errors.Is(err, parser.ErrProjectNotFound):
		return http.StatusNotFound, task.CodeProjectNotFound, "该目录尚未导入为项目（无 .taskboard/meta.db）"
	case errors.Is(err, project.ErrNotFound):
		return http.StatusNotFound, task.CodeProjectNotFound, "项目注册记录不存在"
	case errors.Is(err, project.ErrInvalidWorkdir):
		return http.StatusBadRequest, "PROJECT_INVALID", err.Error()
	case errors.Is(err, auth.ErrInvalidAction):
		return http.StatusBadRequest, "TASK_INVALID", err.Error()
	case errors.Is(err, parser.ErrDraftNotFound):
		return http.StatusNotFound, parser.CodeDraftNotFound, err.Error()
	case errors.Is(err, parser.ErrDraftInvalid):
		return http.StatusUnprocessableEntity, "TASK_INVALID", err.Error()
	case errors.Is(err, parser.ErrImportFailed):
		return http.StatusUnprocessableEntity, parser.CodeImportFailed, err.Error()
	case errors.Is(err, exporter.ErrExportFailed):
		return http.StatusUnprocessableEntity, exporter.CodeExportFailed, err.Error()
	case errors.Is(err, exporter.ErrTemplateInvalid):
		return http.StatusUnprocessableEntity, exporter.CodeTemplateInvalid, err.Error()
	case errors.Is(err, exporter.ErrProjectNotFound):
		return http.StatusNotFound, task.CodeProjectNotFound, "该目录尚未导入为项目（无 .taskboard/meta.db）"
	case errors.Is(err, skill.ErrSkillNotFound):
		return http.StatusNotFound, "SKILL_NOT_FOUND", err.Error()
	case errors.Is(err, skill.ErrInvalidPackage), errors.Is(err, skill.ErrUnknownHost):
		return http.StatusUnprocessableEntity, "SKILL_INVALID", err.Error()
	case errors.Is(err, knowledge.ErrKnowledgeNotFound), errors.Is(err, knowledge.ErrDocumentNotFound):
		return http.StatusNotFound, knowledge.CodeOf(err), err.Error()
	case errors.Is(err, knowledge.ErrKnowledgeInvalid), errors.Is(err, knowledge.ErrDocumentInvalid),
		errors.Is(err, knowledge.ErrDocumentMissing), errors.Is(err, knowledge.ErrCopyFailed):
		return http.StatusUnprocessableEntity, knowledge.CodeOf(err), err.Error()
	case errors.Is(err, knowledge.ErrEmbeddingNotConfigured):
		return http.StatusUnprocessableEntity, knowledge.CodeEmbeddingNotConfigured, err.Error()
	case errors.Is(err, knowledge.ErrEmbeddingFailed), errors.Is(err, knowledge.ErrSummaryFailed),
		errors.Is(err, knowledge.ErrIndexFailed):
		return http.StatusUnprocessableEntity, knowledge.CodeOf(err), err.Error()
	}
	// LLM 错误（TF-015，§14.3）→ 422 对应码。
	if code := llm.ErrorCode(err); code != "" {
		return http.StatusUnprocessableEntity, code, err.Error()
	}
	return http.StatusInternalServerError, "INTERNAL", err.Error()
}

// writeBizError 按 mapError 写统一错误响应。
func writeBizError(w http.ResponseWriter, err error) {
	status, code, message := mapError(err)
	writeError(w, status, code, message, "")
}
