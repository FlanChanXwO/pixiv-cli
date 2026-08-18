//go:build !darwin && !linux && !windows

package loginhelper_test

import (
	"context"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/auth/loginhelper"
)

func TestInstallReportsUnsupportedPlatform(t *testing.T) {
	cleanup, err := loginhelper.Install(context.Background(), "http://127.0.0.1/manual")
	if cleanup != nil {
		t.Fatal("unsupported platform returned a cleanup function")
	}
	if err == nil || err.Error() != "pixiv:// callback handler is only supported on macOS" {
		t.Fatalf("unexpected error: %v", err)
	}
}
