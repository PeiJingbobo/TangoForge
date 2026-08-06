package task

import (
	"errors"
	"fmt"
)

// 业务错误码（docs/TASK-SEMANTICS.md §8）。
//
// HTTP 状态码映射在 TF-013 落地；TF-006 追加 INVALID_TRANSITION / STATUS_IN_USE，
// TF-008 追加 DEPENDENCY_NOT_FOUND / CIRCULAR_DEPENDENCY。
const (
	CodeProjectNotFound = "PROJECT_NOT_FOUND"
	CodeTaskNotFound    = "TASK_NOT_FOUND"
	CodeTaskInvalid     = "TASK_INVALID"
	CodeParentNotFound  = "PARENT_NOT_FOUND"
	CodeParentCycle     = "PARENT_CYCLE"
	CodeStatusNotFound  = "STATUS_NOT_FOUND"
)

// Error 业务错误：携带机器可读 Code 与人类可读 Message。
// 通过 errors.Is 按 Code 匹配哨兵，errors.As 可提取 Code 供传输层映射。
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Is 支持 errors.Is(err, &Error{Code: ...}) 按 Code 判等。
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t != nil && t.Code == e.Code
}

// 哨兵错误（业务层判等用，传输层经 errors.As 提取 Code）。
var (
	// ErrProjectNotFound 表示工作目录未导入为项目（无 {workdir}/.taskboard/meta.db）。
	// 项目识别语义见 docs/TASK-SEMANTICS.md §1（不依赖全局注册表）。
	ErrProjectNotFound = &Error{Code: CodeProjectNotFound, Message: "该目录尚未导入为项目（无 .taskboard/meta.db）"}
	ErrTaskNotFound    = &Error{Code: CodeTaskNotFound, Message: "任务不存在"}
	ErrTaskInvalid     = &Error{Code: CodeTaskInvalid, Message: "任务参数非法"}
	ErrParentNotFound  = &Error{Code: CodeParentNotFound, Message: "父任务不存在或不属于该项目"}
	ErrParentCycle     = &Error{Code: CodeParentCycle, Message: "parent_id 变更会引入父链环"}
	ErrStatusNotFound  = &Error{Code: CodeStatusNotFound, Message: "状态不在项目状态机中"}
)

// NewInvalid 构造携带具体原因的 TASK_INVALID 错误。
func NewInvalid(format string, args ...any) error {
	return &Error{Code: CodeTaskInvalid, Message: fmt.Sprintf(format, args...)}
}

// codeOf 提取错误的业务码；非任务域错误返回空串（供传输层兜底）。
func codeOf(err error) string {
	var te *Error
	if errors.As(err, &te) {
		return te.Code
	}
	return ""
}
