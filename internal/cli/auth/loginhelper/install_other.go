//go:build !darwin && !linux && !windows

// Unsupported platforms report that pixiv:// callback handlers are unavailable.

package loginhelper

import (
	"context"
	"errors"
)

// urlHandlerInstaller keeps the platform files' shared shape so the handler and
// persistent-manifest code can call it without build-specific branches.
type urlHandlerInstaller struct{}

func newURLHandlerInstaller() urlHandlerInstaller { return urlHandlerInstaller{} }

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
