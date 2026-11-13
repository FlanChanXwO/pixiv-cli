package toolhost

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// SDKExecutor 在账号池重放边界内交出新 client 与本次尝试的已选 UID。
type SDKExecutor func(context.Context, Account, func(context.Context, int64, *pixiv.Client) (bool, error)) error

type sdkUserIDKey struct{}

func (a *App) resolveSDKAccount(ctx context.Context) (Account, error) {
	if a.sdk.ResolveSDKAccount != nil {
		return a.sdk.ResolveSDKAccount(ctx, a.sdkAccount)
	}
	return a.resolveAccount(ctx)
}

// SDKReadExecutor 在 userID 为零时保留池调度，续页固定生成账号并校验显式选择。
// SDK 对象只在 attempt 内使用。
func (a *App) SDKReadExecutor(userID int64) pagination.Execute[*pixiv.Client] {
	if a.sdk.ExecuteSDK == nil {
		return nil
	}
	return func(ctx context.Context, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		account, err := a.resolveSDKAccount(ctx)
		if err != nil {
			return err
		}
		if userID != 0 {
			if account.UserID != 0 && account.UserID != userID {
				return &sdk.Error{Reason: sdk.InvalidArgument, Operation: "pixiv.PagedRead", Message: "cursor does not match the selected account"}
			}
			account.UserID = userID
		}
		return a.sdk.ExecuteSDK(ctx, account, func(ctx context.Context, selected int64, client *pixiv.Client) (bool, error) {
			return attempt(context.WithValue(ctx, sdkUserIDKey{}, selected), client)
		})
	}
}

// ReadSDK 在新 SDK 的完整读取边界成功后返回投影结果。
func ReadSDK[T any](a *App, ctx context.Context, invoke func(context.Context, *pixiv.Client) (T, error)) (T, error) {
	var zero, result T
	execute := a.SDKReadExecutor(0)
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

// WriteSDK 从进入修改回调起按已提交处理，防止写入跨账号重放。
func WriteSDK(a *App, ctx context.Context, invoke func(context.Context, *pixiv.Client) error) error {
	if a.sdk.ExecuteSDK == nil {
		return errors.New("sdk pooled operation is not configured")
	}
	account, err := a.resolveSDKAccount(ctx)
	if err != nil {
		return err
	}
	return a.sdk.ExecuteSDK(ctx, account, func(ctx context.Context, _ int64, client *pixiv.Client) (bool, error) {
		return true, invoke(ctx, client)
	})
}

// SDKUserID 在读取回调内把省略的用户 ID 解析为本次尝试的账号。
func SDKUserID(ctx context.Context, requested int64) int64 {
	if requested != 0 {
		return requested
	}
	return ctx.Value(sdkUserIDKey{}).(int64)
}

// SDKLease 打开固定账号的绑定 client；调用方负责关闭租约。
type SDKLease func(context.Context, Account) (*lifecycle.Lease[*pixiv.Client], error)

// OpenSDKClient 为一次媒体或下载操作固定账号与 client。
func (a *App) OpenSDKClient(ctx context.Context) (*lifecycle.Lease[*pixiv.Client], error) {
	if a.sdk.OpenSDKLease == nil {
		return nil, errors.New("pixiv sdk is not configured")
	}
	account, err := a.resolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	return a.sdk.OpenSDKLease(ctx, account)
}

// Artwork 在 Gallery 的固定账号与 grant 范围内读取元数据及实际原图页号。
func (a *App) Artwork(ctx context.Context, id int64) (*pixiv.Artwork, []int, error) {
	account, err := a.resolveAccount(ctx)
	if err != nil {
		return nil, nil, err
	}
	return a.sdk.Media.Artwork(ctx, account.UserID, id)
}
