//go:build !darwin && !linux && !windows

package urlscheme

import (
	"context"
	"testing"
)

func TestInstallReportsUnsupportedPlatform(t *testing.T) {
	cleanup, err := Install(context.Background(), "http://127.0.0.1/manual")
	if cleanup != nil {
		t.Fatal("unsupported platform returned a cleanup function")
	}
	if err == nil || err.Error() != "pixiv:// callback handler is only supported on macOS" {
		t.Fatalf("unexpected error: %v", err)
	}
}
