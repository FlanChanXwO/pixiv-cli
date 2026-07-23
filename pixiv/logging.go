package pixiv

import (
	"errors"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

// operationLog 在公开 SDK 边界写入稳定、安全的结果事件。它只记录本包已经
// 规格化的 Error 元数据，绝不把任意错误字符串、URL、请求参数或响应体交给 logger。
func (c *Client) operationLog(operation Operation, started time.Time, err error, illustID, userID int64) {
	if c == nil || c.logger == nil {
		return
	}
	backend := constants.LogBackendLocal
	code := ""
	status := 0
	transportKind := ""
	result := logging.ResultSuccess
	var pixivErr *Error
	if err != nil {
		result = logging.ResultError
		if errors.As(err, &pixivErr) {
			backend = string(pixivErr.Backend)
			if backend == "" {
				backend = constants.LogBackendLocal
			}
			code = string(pixivErr.Code)
			status = pixivErr.UpstreamStatus
			transportKind = string(pixivErr.TransportKind)
			if pixivErr.IllustID != 0 {
				illustID = pixivErr.IllustID
			}
			if pixivErr.UserID != 0 {
				userID = pixivErr.UserID
			}
		}
	}
	logging.LogOperation(c.logger, logging.OperationEvent{
		Component:     "pixiv_sdk",
		Operation:     string(operation),
		Backend:       backend,
		Duration:      time.Since(started),
		Result:        result,
		ErrorCode:     code,
		Status:        status,
		TransportKind: transportKind,
		IllustID:      illustID,
		UserID:        userID,
	})
}

// delegatedOperationLog 用于 OpenDefault 会代理给 scoped Client 的内容/资源操作。
// 外层 client 只负责建立快照，真实请求仅由 scoped Client 写一条事件。
func (c *Client) delegatedOperationLog(operation Operation, started time.Time, err error, illustID, userID int64) {
	if c != nil && c.defaults != nil {
		return
	}
	c.operationLog(operation, started, err, illustID, userID)
}
