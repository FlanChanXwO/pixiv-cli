//go:build !darwin && !linux && !windows

package loginhelper

import (
	"context"
	"errors"
)

// Install 在非 macOS 平台显式报告系统 URL scheme helper 不可用。
func Install(_ context.Context, _ string) (func(), error) {
	return nil, errors.New("pixiv:// callback handler is only supported on macOS")
}

func EnsurePersistent(_ context.Context) error {
	return errors.New("persistent pixiv:// callback handler is not supported on this platform")
}

func DisablePersistent(_ context.Context) error {
	return errors.New("persistent pixiv:// callback handler is not supported on this platform")
}

func DelegateToPrevious(_ context.Context, _ string) error {
	return errors.New("previous Pixiv URL handler cannot be opened on this platform")
}
