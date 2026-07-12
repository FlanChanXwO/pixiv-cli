package pixiv

import (
	"fmt"
	"strings"
)

// ErrorCode 是供调用方稳定分支处理的机器可读错误码。
type ErrorCode string

const (
	CodeInvalidArgument           ErrorCode = "invalid_argument"
	CodeArtworkUnavailable        ErrorCode = "artwork_unavailable"
	CodeUnauthorized              ErrorCode = "unauthorized"
	CodeForbidden                 ErrorCode = "forbidden"
	CodeUnsupported               ErrorCode = "unsupported"
	CodeRateLimited               ErrorCode = "rate_limited"
	CodeUpstreamError             ErrorCode = "upstream_error"
	CodeUpstreamUnavailable       ErrorCode = "upstream_unavailable"
	CodeMalformedUpstreamResponse ErrorCode = "malformed_upstream_response"
)

// Backend 标识失败所在的协议边界。
type Backend string

const (
	BackendAppAPI   Backend = "app_api"
	BackendWebAPI   Backend = "web_api"
	BackendOAuth    Backend = "oauth"
	BackendResource Backend = "resource"
)

// Operation 标识稳定的公开 SDK 操作。
type Operation string

const (
	OperationIllustDetail Operation = "illust_detail"
	OperationIllustPages  Operation = "illust_pages"
)

// Error 是公开 SDK 的安全、可分类错误。cause 只保存已脱敏原因。
type Error struct {
	Code           ErrorCode
	Operation      Operation
	Backend        Backend
	Retryable      bool
	UpstreamStatus int
	IllustID       int64
	UserID         int64
	cause          error
}

// Error 返回不含上游响应体、URL、header 或凭据的诊断文本。
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	parts := []string{"pixiv", string(e.Code)}
	if e.Operation != "" {
		parts = append(parts, "operation="+string(e.Operation))
	}
	if e.Backend != "" {
		parts = append(parts, "backend="+string(e.Backend))
	}
	if e.UpstreamStatus != 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.UpstreamStatus))
	}
	if e.IllustID > 0 {
		parts = append(parts, fmt.Sprintf("illust_id=%d", e.IllustID))
	}
	if e.UserID > 0 {
		parts = append(parts, fmt.Sprintf("user_id=%d", e.UserID))
	}
	return strings.Join(parts, " ")
}

// Unwrap 暴露已验证安全的 cause，并保留 context 取消等标准错误链。
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Is 让调用方通过稳定 code sentinel 使用 errors.Is。
func (e *Error) Is(target error) bool {
	if e == nil {
		return target == nil
	}
	switch other := target.(type) {
	case codeSentinel:
		return e.Code == ErrorCode(other)
	case *Error:
		return other != nil && other.Code != "" && e.Code == other.Code
	default:
		return false
	}
}

type codeSentinel ErrorCode

func (e codeSentinel) Error() string { return string(e) }

var (
	ErrInvalidArgument           error = codeSentinel(CodeInvalidArgument)
	ErrArtworkUnavailable        error = codeSentinel(CodeArtworkUnavailable)
	ErrUnauthorized              error = codeSentinel(CodeUnauthorized)
	ErrForbidden                 error = codeSentinel(CodeForbidden)
	ErrUnsupported               error = codeSentinel(CodeUnsupported)
	ErrRateLimited               error = codeSentinel(CodeRateLimited)
	ErrUpstreamError             error = codeSentinel(CodeUpstreamError)
	ErrUpstreamUnavailable       error = codeSentinel(CodeUpstreamUnavailable)
	ErrMalformedUpstreamResponse error = codeSentinel(CodeMalformedUpstreamResponse)
)

func newError(code ErrorCode, operation Operation, backend Backend, retryable bool, status int, illustID int64, cause error) *Error {
	return &Error{
		Code:           code,
		Operation:      operation,
		Backend:        backend,
		Retryable:      retryable,
		UpstreamStatus: status,
		IllustID:       illustID,
		cause:          cause,
	}
}
