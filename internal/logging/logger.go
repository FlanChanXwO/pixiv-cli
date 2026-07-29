// Package logging 统一项目内 operation diagnostics 的结构化输出，不配置全局 logger。
package logging

import (
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// OperationEvent 是可跨边界关联的最小诊断记录。它只接受稳定元数据，避免调用方把
// URL、header、refresh token 或上游响应体放入日志。
type OperationEvent struct {
	Component string
	Operation string
	Backend   string
	Duration  time.Duration
	Result    string
	ErrorCode string
	Status    int
	// TransportKind 只接受公开枚举；不能把网络错误文本放入日志。
	TransportKind string
	IllustID      int64
	UserID        int64
}

// OrDiscard 将可选 logger 规范为明确的 discard logger；SDK 与服务边界绝不回退到
// slog.Default，避免库调用污染宿主程序的日志协议。
func OrDiscard(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// LogOperation 仅输出固定的安全字段。error result 使用 error level，其余结果为
// info；错误正文由调用链返回给调用方，不复制进 operation diagnostics。
func LogOperation(logger *slog.Logger, event OperationEvent) {
	level := slog.LevelInfo
	if event.Result == ResultError {
		level = slog.LevelError
	}
	attrs := []slog.Attr{
		slog.String(LogFieldComponent, event.Component),
		slog.String(LogFieldOperation, event.Operation),
		slog.String(LogFieldBackend, event.Backend),
		slog.Duration(LogFieldDuration, event.Duration),
		slog.String(LogFieldResult, event.Result),
		slog.String(LogFieldErrorCode, event.ErrorCode),
		slog.Int(LogFieldStatus, event.Status),
	}
	if source := operationCallsite(); source != "" {
		attrs = append(attrs, slog.String(LogFieldSource, source))
	}
	if transportKind := safeTransportKind(event.TransportKind); transportKind != "" {
		attrs = append(attrs, slog.String(LogFieldTransportKind, transportKind))
	}
	if event.IllustID != 0 {
		attrs = append(attrs, slog.Int64(LogFieldIllustID, event.IllustID))
	}
	if event.UserID != 0 {
		attrs = append(attrs, slog.Int64(LogFieldUserID, event.UserID))
	}
	OrDiscard(logger).LogAttrs(nil, level, OperationLogMessage, attrs...)
}

// operationCallsite 记录调用 LogOperation/LogRateLimitRetry 的项目内文件与行号。
// 它只保留仓库相对路径；未知路径退化为文件名，避免把用户 home 或构建机目录写入日志。
func operationCallsite() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return ""
	}
	path := filepath.ToSlash(file)
	const repositoryMarker = "/pixiv-cli/"
	if index := strings.LastIndex(path, repositoryMarker); index >= 0 {
		path = path[index+len(repositoryMarker):]
	} else {
		path = filepath.Base(path)
	}
	return path + ":" + strconv.Itoa(line)
}

func safeTransportKind(value string) string {
	switch value {
	case "dns", "tls", "proxy", "connection_refused", "connection_reset", "timeout", "unknown":
		return value
	default:
		return ""
	}
}

// LogRateLimitRetry 记录已解析的等待时长和唯一的重试序号；不记录 Retry-After 原文、
// 请求 URL、鉴权 header 或响应 body。
func LogRateLimitRetry(logger *slog.Logger, retryAfter time.Duration, attempt int) {
	attrs := []slog.Attr{
		slog.String(LogFieldComponent, "pixiv_app_api"),
		slog.String(LogFieldOperation, "read"),
		slog.String(LogFieldResult, ResultRateLimitRetry),
		slog.Int(LogFieldStatus, http.StatusTooManyRequests),
		slog.Duration(LogFieldRetryAfter, retryAfter),
		slog.Int(LogFieldAttempt, attempt),
	}
	if source := operationCallsite(); source != "" {
		attrs = append(attrs, slog.String(LogFieldSource, source))
	}
	OrDiscard(logger).LogAttrs(nil, slog.LevelInfo, RateLimitRetryLogMessage, attrs...)
}
