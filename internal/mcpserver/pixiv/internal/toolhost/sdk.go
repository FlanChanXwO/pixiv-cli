package toolhost

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// SDKExecutor 在固定账号的池重放边界内交出新 client 与已选 UID。
type SDKExecutor func(context.Context, Account, func(context.Context, int64, *pixiv.Client) (bool, error)) error

// SDKReadExecutor 复用本次工具的账号快照；SDK 对象只在 attempt 内使用。
func (a *App) SDKReadExecutor() pagination.Execute[*pixiv.Client] {
	if a.sdk.ExecuteSDK == nil {
		return nil
	}
	return func(ctx context.Context, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		account, err := a.resolveAccount(ctx)
		if err != nil {
			return err
		}
		return a.sdk.ExecuteSDK(ctx, account, func(ctx context.Context, _ int64, client *pixiv.Client) (bool, error) {
			return attempt(ctx, client)
		})
	}
}

// ReadSDK 在新 SDK 的完整读取边界成功后返回投影结果。
func ReadSDK[T any](a *App, ctx context.Context, invoke func(context.Context, *pixiv.Client) (T, error)) (T, error) {
	var zero, result T
	execute := a.SDKReadExecutor()
	if execute == nil {
		return zero, errors.New("sdk pooled operation is not configured")
	}
	err := execute(ctx, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		var err error
		result, err = invoke(ctx, client)
		return false, err
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}
