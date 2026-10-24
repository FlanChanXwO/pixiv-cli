// SDK 操作名在 ctx 中的登记；日志属性据此带上 sdk_operation，不影响请求。

package pixivapi

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

// WithOperation 把 SDK 操作名放进 ctx；已登记时保留最外层的操作名。
func WithOperation(ctx context.Context, operation string) context.Context {
	return logging.WithOperation(ctx, operation)
}
