package pixiv

import (
	"context"
	"errors"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application/config"
)

// PooledSDKOperationOptions 是账号池用例的显式依赖。bootstrap 只提供配置读取、
// authdb state adapter 和 SDK client factory；账号选择、冻结与安全重放仍在这里。
type PooledSDKOperationOptions struct {
	LoadRuntime func() (config.RuntimeConfig, error)
	State       AccountPoolStateStore
	Factory     ClientFactory
	Now         func() time.Time
}

// NewPooledSDKOperation 构造 CLI/MCP 共用的账号池 operation。未启用账号池时
// 直接执行一次 factory；启用后由 AccountPoolExecutor 决定是否可在 commit 前换号。
func NewPooledSDKOperation(options PooledSDKOperationOptions) PooledOperation {
	return func(ctx context.Context, request SDKClientRequest, attempt func(context.Context, ClientSet) (bool, error)) error {
		if options.LoadRuntime == nil {
			return errors.New("account pool runtime loader is not configured")
		}
		runtime, err := options.LoadRuntime()
		if err != nil {
			return err
		}
		if options.Factory == nil {
			return errors.New("pixiv sdk client factory is not configured")
		}
		if !runtime.AccountPool.Enabled {
			client, err := options.Factory(request)
			if err != nil {
				return err
			}
			_, err = attempt(ctx, client)
			return err
		}
		executor := AccountPoolExecutor{
			Config: runtime.AccountPool,
			State:  options.State,
			Now:    options.Now,
		}
		return executor.Run(ctx, func(ctx context.Context, userID int64) (bool, error) {
			poolRequest := request
			poolRequest.UserID = userID
			poolRequest.RefreshToken = ""
			client, err := options.Factory(poolRequest)
			if err != nil {
				return false, err
			}
			return attempt(ctx, client)
		})
	}
}
