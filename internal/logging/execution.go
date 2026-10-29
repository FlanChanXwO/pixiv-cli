package logging

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
)

// Start 为一次命令执行创建 stderr logger 与带调用 ID 的上下文；JSON 格式把
// 失败记录提升到 ERROR，文本格式使用 DEBUG。
func Start(ctx context.Context, out io.Writer, commandPath, level, format string) (State, error) {
	logger, err := New(out, level, format)
	if err != nil {
		return State{}, err
	}
	call := State{
		Logger:       logger,
		Ctx:          WithCall(ctx, logger, commandPath, rand.Text()),
		FailureLevel: slog.LevelDebug,
	}
	if format == FormatJSON {
		call.FailureLevel = slog.LevelError
	}
	return call, nil
}

// State 保存一次执行的 logger、调用上下文与失败记录的级别。
type State struct {
	Logger       *slog.Logger
	Ctx          context.Context
	FailureLevel slog.Level
}
