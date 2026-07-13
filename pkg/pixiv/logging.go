package pixiv

import (
	"errors"
	"log/slog"
	"time"
)

// operationLog 在公开 SDK 边界写入稳定、安全的结果事件。它只记录本包已经
// 规格化的 Error 元数据，绝不把任意错误字符串、URL、请求参数或响应体交给 logger。
func (c *Client) operationLog(operation Operation, started time.Time, err error, illustID, userID int64) {
	if c == nil || c.logger == nil {
		return
	}
	backend := "local"
	code := ""
	status := 0
	result := "success"
	var pixivErr *Error
	if err != nil {
		result = "error"
		if errors.As(err, &pixivErr) {
			backend = string(pixivErr.Backend)
			if backend == "" {
				backend = "local"
			}
			code = string(pixivErr.Code)
			status = pixivErr.UpstreamStatus
			if pixivErr.IllustID != 0 {
				illustID = pixivErr.IllustID
			}
			if pixivErr.UserID != 0 {
				userID = pixivErr.UserID
			}
		}
	}
	attrs := []slog.Attr{
		slog.String("component", "pixiv_sdk"),
		slog.String("operation", string(operation)),
		slog.String("backend", backend),
		slog.Duration("duration", time.Since(started)),
		slog.String("result", result),
		slog.String("error_code", code),
		slog.Int("status", status),
	}
	if illustID != 0 {
		attrs = append(attrs, slog.Int64("illust_id", illustID))
	}
	if userID != 0 {
		attrs = append(attrs, slog.Int64("user_id", userID))
	}
	c.logger.LogAttrs(nil, slog.LevelInfo, "pixiv operation", attrs...)
}

// delegatedOperationLog 用于 OpenDefault 会代理给 scoped Client 的内容/资源操作。
// 外层 client 只负责建立快照，真实请求仅由 scoped Client 写一条事件。
func (c *Client) delegatedOperationLog(operation Operation, started time.Time, err error, illustID, userID int64) {
	if c != nil && c.defaults != nil {
		return
	}
	c.operationLog(operation, started, err, illustID, userID)
}
