package config

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

// NewLogger 按已解析的运行时配置创建本进程根 logger。调用方决定输出流，CLI/MCP
// 一律传 stderr；本函数绝不设置或读取 slog 默认全局 logger。
func NewLogger(writer io.Writer, cfg RuntimeConfig) (*slog.Logger, error) {
	level, err := slogLevel(cfg.LogLevel)
	if err != nil {
		return nil, err
	}
	options := &slog.HandlerOptions{Level: level}
	return slog.New(logging.NewTextHandler(writer, options)), nil
}

func normalizeLogLevel(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "trace", "debug", "info", "warn", "error":
		return value, nil
	default:
		return "", fmt.Errorf("log_level must be one of trace, debug, info, warn, error")
	}
}

func slogLevel(value string) (slog.Level, error) {
	value, err := normalizeLogLevel(value)
	if err != nil {
		return 0, err
	}
	switch value {
	case "trace":
		return slog.LevelDebug - 4, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		panic("validated log level")
	}
}
