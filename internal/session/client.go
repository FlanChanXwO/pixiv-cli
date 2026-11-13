package session

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// SDKExecutor 为 CLI 交出新 SDK client 与本次操作选中的账号。
type SDKExecutor func(context.Context, ProxyRequest, func(context.Context, int64, *pixiv.Client) (bool, error)) error

// ExecuteSDK 在新旧 client 共用的账号池与轮换门内执行一次新 SDK 操作。
// 回调接收已选 UID；返回 committed=true 后，账号池停止重放。
func (l *Local) ExecuteSDK(ctx context.Context, request ProxyRequest, callback func(context.Context, int64, *pixiv.Client) (bool, error)) error {
	ports, err := l.ports(ctx)
	if err != nil {
		return err
	}
	settings, err := l.loadPixiv(request)
	if err != nil {
		return err
	}
	return ports.facade.useAccount(ctx, selectionFor(request.UserID), func(ctx context.Context, userID int64, attempt *lifecycle.Attempt) (err error) {
		lease, selectedID, err := openSDKLease(ctx, ports, userID, settings.SDKOptions)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, lease.Close()) }()
		committed, err := callback(ctx, selectedID, lease.Value())
		if committed {
			attempt.Commit()
		}
		return err
	})
}

func openSDKLease(ctx context.Context, ports *pixivPorts, userID int64, options pixiv.Options) (lease *lifecycle.Lease[*pixiv.Client], selectedID int64, err error) {
	// 配置加载函数交出的传输归本次操作；SDK 只释放它自己创建的空闲连接。
	defer func() {
		if lease == nil && options.HTTPClient != nil {
			options.HTTPClient.CloseIdleConnections()
		}
	}()
	if err := ports.facade.gate.Acquire(ctx); err != nil {
		return nil, 0, err
	}
	client, err := ports.store.OpenSDKClient(ctx, userID, options)
	if err != nil {
		ports.facade.gate.Release()
		return nil, 0, err
	}
	return lifecycle.NewLease(client, func() error {
		defer ports.facade.gate.Release()
		client.CloseIdleConnections()
		if options.HTTPClient != nil {
			options.HTTPClient.CloseIdleConnections()
		}
		return nil
	}), userID, nil
}

// OpenSDKLease 固定显式账号的 SDK client，关闭租约后释放连接与轮换门。
func (l *Local) OpenSDKLease(ctx context.Context, request ProxyRequest) (*lifecycle.Lease[*pixiv.Client], error) {
	ports, err := l.ports(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := l.loadPixiv(request)
	if err != nil {
		return nil, err
	}
	userID := request.UserID
	if userID == 0 {
		userID, err = ports.facade.accounts.selectedUserID(ctx)
		if err != nil {
			return nil, err
		}
	}
	lease, _, err := openSDKLease(ctx, ports, userID, settings.SDKOptions)
	return lease, err
}
