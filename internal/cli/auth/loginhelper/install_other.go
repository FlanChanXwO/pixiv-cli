//go:build !darwin && !linux && !windows

package loginhelper

import (
	"context"
	"errors"
)

// urlHandlerInstaller 保持各平台文件共同的形状，让 handler 和持久 manifest 无需按平台分支；
// 不支持的平台实现直接返回不可用错误。
type urlHandlerInstaller struct{}

func newURLHandlerInstaller() urlHandlerInstaller { return urlHandlerInstaller{} }

// Install 在不支持的平台上直接返回不可用错误，不创建任何协议关联。
func Install(ctx context.Context, callbackRelayURL string) (func(), error) {
	return newURLHandlerInstaller().install(ctx, callbackRelayURL)
}

func (urlHandlerInstaller) install(_ context.Context, _ string) (func(), error) {
	return nil, errors.New("pixiv:// callback handler is only supported on macOS")
}

func (urlHandlerInstaller) ensurePersistent(_ context.Context) error {
	return errors.New("persistent pixiv:// callback handler is not supported on this platform")
}

func (urlHandlerInstaller) delegateToPrevious(_ context.Context, _ string) error {
	return errors.New("previous Pixiv URL handler cannot be opened on this platform")
}
