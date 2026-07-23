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
