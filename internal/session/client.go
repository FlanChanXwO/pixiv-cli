package session

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// ExecuteSDK 在新旧 client 共用的账号池与轮换门内执行一次新 SDK 操作。
// 回调接收已选 UID；返回 committed=true 后，账号池停止重放。
func (l *Local) ExecuteSDK(ctx context.Context, request ProxyRequest, callback func(context.Context, int64, *pixiv.Client) (bool, error)) error {
	ports, err := l.ports()
	if err != nil {
		return err
	}
	settings, err := l.loadPixiv(request)
	if err != nil {
		return err
	}
	return ports.facade.useAccount(ctx, request.UserID, func(ctx context.Context, userID int64, attempt *lifecycle.Attempt) (err error) {
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

// OpenSDKLease 固定账号与网络配置，返回新 SDK client 和已选 UID；调用方负责关闭租约。
func (l *Local) OpenSDKLease(ctx context.Context, request ProxyRequest) (*lifecycle.Lease[*pixiv.Client], int64, error) {
	ports, err := l.ports()
	if err != nil {
		return nil, 0, err
	}
	settings, err := l.loadPixiv(request)
	if err != nil {
		return nil, 0, err
	}
	return openSDKLease(ctx, ports, request.UserID, settings.SDKOptions)
}

func openSDKLease(ctx context.Context, ports *pixivPorts, userID int64, options pixiv.Options) (*lifecycle.Lease[*pixiv.Client], int64, error) {
	if err := ports.facade.gate.Acquire(ctx); err != nil {
		return nil, 0, err
	}
	if userID == 0 {
		current, err := ports.accounts.CurrentUser(ctx)
		if err != nil {
			ports.facade.gate.Release()
			return nil, 0, err
		}
		userID = current.UserID
	}
	client, err := ports.accounts.OpenSDKClient(ctx, userID, options)
	if err != nil {
		ports.facade.gate.Release()
		return nil, 0, err
	}
	return lifecycle.NewLease(client, func() error {
		defer ports.facade.gate.Release()
		client.CloseIdleConnections()
		return nil
	}), userID, nil
}
