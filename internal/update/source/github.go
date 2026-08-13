package source

import (
	"context"
	"fmt"
)

// GitHubUserAgent 让匿名 Releases 请求满足 GitHub 的可识别客户端要求，
// 且不包含版本或用户信息。
const GitHubUserAgent = "pixiv-cli"

// CheckContext 把已经取消的 context 变成带操作描述的稳定错误。
func CheckContext(ctx context.Context, action string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return nil
}
