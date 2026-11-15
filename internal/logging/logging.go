// Package logging 提供 CLI 与 MCP 的 slog 层：ctx 中的调用信息、属性键常量和
// 可配置的 text/JSON handler。
//
// [WithCall] 由执行边界写入 operation 与 call_id，之后同一 ctx 上的日志都会附带这些
// 字段；SDK 使用默认 slog logger，属性键由各 SDK 自持。
//
// 包的组成：[New] 创建带调用上下文与脱敏行为的 handler，[From] 从 ctx 取 logger，
// [AttrOperation] 等常量是执行边界的字段名，[WithCall] 负责绑定。
package logging

import (
	"context"
	"log/slog"
)

// AttrOperation 等常量是执行边界与 JSON 输出使用的日志属性键。
const (
	AttrDuration  = "duration"
	AttrError     = "error"
	AttrCount     = "count"
	AttrStage     = "stage"
	AttrOperation = "operation"
	AttrCallID    = "call_id"
	AttrReason    = "reason"
)

type contextKey struct{}

// callInfo 保存一次 CLI 命令或 MCP tool 调用的关联信息，由执行边界写入 ctx。
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
