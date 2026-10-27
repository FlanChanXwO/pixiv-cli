package reversesearch

import "errors"

// ErrorCode 枚举跨 CLI/MCP 边界保持稳定的错误分类。
type ErrorCode string

// CodeUnknown 等 Code 是跨 CLI/MCP 边界保持稳定的错误分类；新增值时同步调用方映射。
const (
	CodeUnknown                   ErrorCode = "unknown"
	CodeInvalidRequest            ErrorCode = "invalid_request"
	CodeInvalidSource             ErrorCode = "invalid_source"
	CodeSourceNotRegularFile      ErrorCode = "source_not_regular_file"
	CodeSourceReadFailed          ErrorCode = "source_read_failed"
	CodeSourceHTTPStatus          ErrorCode = "source_http_status"
	CodeSnapshotFailed            ErrorCode = "snapshot_failed"
	CodeProviderNotConfigured     ErrorCode = "provider_not_configured"
	CodeMissingCredential         ErrorCode = "missing_credential"
	CodeMalformedUpstreamResponse ErrorCode = "malformed_upstream_response"
	CodeUpstreamHTTPStatus        ErrorCode = "upstream_http_status"
	CodeProviderFailed            ErrorCode = "provider_failed"
	CodeAllProvidersFailed        ErrorCode = "all_providers_failed"
	CodeChallengeRequired         ErrorCode = "challenge_required"
	CodeSolverUnavailable         ErrorCode = "solver_unavailable"
	CodeSolverFailed              ErrorCode = "solver_failed"
	CodeMalformedSolverResponse   ErrorCode = "malformed_solver_response"
)

// Error 只渲染预先审查的安全消息；cause 用于 errors.Is/As，但不会进入 Error 文本。
type Error struct {
	code    ErrorCode
	message string
	cause   error
}

// NewError 创建带稳定 code 的安全错误。message 不得包含 source、凭据、临时路径
// 或未经清洗的上游响应。
func NewError(code ErrorCode, message string, cause error) *Error {
	return &Error{code: code, message: message, cause: cause}
}

// Error 只返回预先审查过的安全消息，cause 不会出现在文本中。
func (e *Error) Error() string { return e.message }

// Unwrap 暴露原始 cause 供 errors.Is/As 判断，同时不把它拼进错误文本。
func (e *Error) Unwrap() error { return e.cause }

// Code 返回跨 CLI/MCP 边界保持稳定的错误分类，供 [CodeOf] 读取。
func (e *Error) Code() ErrorCode { return e.code }

// CodeOf 返回领域错误的稳定分类；其他错误归为 unknown。
func CodeOf(err error) ErrorCode {
	var classified interface{ Code() ErrorCode }
	if errors.As(err, &classified) {
		return classified.Code()
	}
	return CodeUnknown
}
