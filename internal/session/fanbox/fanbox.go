// Package fanbox 提供 product-scoped FANBOX session lifecycle。
// 每次调用都由调用方传入独立 client，不维护进程级 registry 或 mutex。
package fanbox

import (
	"context"
	"errors"

	accountfanbox "github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

// OpenClient 根据一个 account 打开独立 FANBOX client；Factory 可由 command/tool
// 注入，以保持不同 operation 的 transport/session 隔离。
type OpenClient func(context.Context, accountfanbox.Account) (*fanboxsdk.Client, error)

// Run 打开并使用一个独立 FANBOX client。close 只作用于本次 client。
func Run(ctx context.Context, account accountfanbox.Account, open OpenClient, use func(context.Context, *fanboxsdk.Client, *session.Attempt) error) error {
	if open == nil {
		return errors.New("session/fanbox: open function is nil")
	}
	if use == nil {
		return errors.New("session/fanbox: use function is nil")
	}
	return session.Run(ctx,
		func(ctx context.Context) (*fanboxsdk.Client, func() error, error) {
			client, err := open(ctx, account)
			if err != nil {
				return nil, nil, err
			}
			return client, func() error {
				if client != nil {
					client.CloseIdleConnections()
				}
				return nil
			}, nil
		},
		func(ctx context.Context, client *fanboxsdk.Client, attempt *session.Attempt) error {
			return use(ctx, client, attempt)
		},
	)
}
