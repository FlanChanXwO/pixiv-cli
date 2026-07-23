package constants

const (
	// AppDataDirName 是所有本地应用数据的唯一根目录名。它直接位于当前用户的
	// home 目录，避免与其他名为 pixiv 的应用共用配置、认证或日志路径。
	AppDataDirName  = "pixiv-cli"
	PrivateDirMode  = 0o700
	PrivateFileMode = 0o600

	// OperationLogMessage 与字段名属于跨 CLI、MCP、SDK 与下载层共享的诊断协议。
	// 它们不得包含请求 URL、header、token 或响应 body 等敏感数据。
	OperationLogMessage      = "pixiv operation"
	RateLimitRetryLogMessage = "pixiv app api rate limit retry"
	LogFieldComponent        = "component"
	LogFieldOperation        = "operation"
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

	LogResultSuccess        = "success"
	LogResultError          = "error"
	LogResultRateLimitRetry = "rate_limit_retry"
	LogBackendLocal         = "local"
)
