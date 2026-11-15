package environment

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// PixivRequest 携带命令或 MCP 请求的可选账号与代理覆盖；组合根解析配置。
type PixivRequest struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// PixivExecutor 交出本次选择的账号与 client；回调提交后停止账号池重放。
type PixivExecutor func(context.Context, PixivRequest, func(context.Context, int64, *pixiv.Client) (bool, error)) error
