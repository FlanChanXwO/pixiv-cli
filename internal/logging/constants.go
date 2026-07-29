package logging

const (
	// OperationLogMessage 与字段名组成跨 CLI、MCP、SDK 与下载器共享的安全诊断协议。
	// 字段值不得含 URL、header、token、请求输入或响应 body。
	OperationLogMessage      = "pixiv operation"
	RateLimitRetryLogMessage = "pixiv app api rate limit retry"
	LogFieldComponent        = "component"
	LogFieldOperation        = "operation"
	LogFieldSource           = "source"
	LogFieldBackend          = "backend"
	LogFieldDuration         = "duration"
	LogFieldResult           = "result"
	LogFieldErrorCode        = "error_code"
	LogFieldStatus           = "status"
	LogFieldTransportKind    = "transport_kind"
	LogFieldIllustID         = "illust_id"
	LogFieldUserID           = "user_id"
	LogFieldRetryAfter       = "retry_after"
	LogFieldAttempt          = "attempt"

	ResultSuccess        = "success"
	ResultError          = "error"
	ResultRateLimitRetry = "rate_limit_retry"
	BackendLocal         = "local"
)
