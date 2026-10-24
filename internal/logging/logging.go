// Package logging 是 CLI、MCP、SDK 共用的 slog 层：ctx 中的调用信息、属性键常量
// 和可配置的 text/JSON handler。调用边界通过 WithCall 写入 operation 和 call_id。
package logging

import (
	"context"
	"log/slog"
)

// 日志属性键。SDK 传输层、执行边界和 JSON 输出共用这些字段名。
const (
	AttrMethod       = "method"
	AttrHost         = "host"
	AttrPath         = "path"
	AttrStatus       = "status"
	AttrDuration     = "duration"
	AttrAttempt      = "attempt"
	AttrWait         = "wait"
	AttrSDKOperation = "sdk_operation"
	AttrError        = "error"
	AttrCount        = "count"
	AttrOperation    = "operation"
	AttrCallID       = "call_id"
	AttrReason       = "reason"
)

type contextKey struct{}

// callInfo 是一次 CLI 命令或 MCP tool 调用的关联信息，由执行边界写入 ctx。
type callInfo struct {
	operation string
	callID    string
}

type callContext struct {
	logger *slog.Logger
	info   callInfo
}

// WithCall 把调用信息和 logger 放进 ctx；同一次调用的所有日志都会附带这些信息。
func WithCall(ctx context.Context, logger *slog.Logger, operation, callID string) context.Context {
	return context.WithValue(ctx, contextKey{}, callContext{
		logger: logger,
		info:   callInfo{operation: operation, callID: callID},
	})
}

// From 返回 ctx 中的 logger；没有时返回丢弃所有输出的 logger。
func From(ctx context.Context) *slog.Logger {
	if value, ok := ctx.Value(contextKey{}).(callContext); ok {
		return value.logger
	}
	return Discard()
}

// HasLogger 报告 ctx 是否携带执行边界放入的 logger。
func HasLogger(ctx context.Context) bool {
	_, ok := ctx.Value(contextKey{}).(callContext)
	return ok
}

// Discard 返回不输出任何记录的 logger。
func Discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func callInfoFrom(ctx context.Context) (callInfo, bool) {
	value, ok := ctx.Value(contextKey{}).(callContext)
	return value.info, ok
}

type operationKey struct{}

// WithOperation 把 SDK 操作名放进 ctx；只作为日志属性，不影响请求。
func WithOperation(ctx context.Context, operation string) context.Context {
	if existing, _ := ctx.Value(operationKey{}).(string); existing != "" {
		return ctx
	}
	return context.WithValue(ctx, operationKey{}, operation)
}

// OperationAttr 返回 SDK 操作名属性；没有登记时为空字符串。
func OperationAttr(ctx context.Context) slog.Attr {
	operation, _ := ctx.Value(operationKey{}).(string)
	return slog.String(AttrSDKOperation, operation)
}
