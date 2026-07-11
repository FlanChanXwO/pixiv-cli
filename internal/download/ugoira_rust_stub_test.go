//go:build !cgo || (!darwin && !linux && !windows) || ((darwin || linux || windows) && !amd64 && !arm64)

package download

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultRustUgoiraEncoderReportsUnavailableCgoPlatform(t *testing.T) {
	err := NewRustUgoiraEncoder().Encode(context.Background(), UgoiraEncodeInput{
		OutputPath: t.TempDir() + "/out.gif",
		Format:     AnimationFormatGIF,
	})
	if err == nil || !strings.Contains(err.Error(), "rust ugoira encoder unavailable") || !strings.Contains(err.Error(), "CGO_ENABLED=1") {
		t.Fatalf("Encode error = %v", err)
	}
}
