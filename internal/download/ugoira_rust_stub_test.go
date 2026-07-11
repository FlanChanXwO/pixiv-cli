//go:build !ugoira_rust || !cgo

package download

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultRustUgoiraEncoderReportsMissingBuildTag(t *testing.T) {
	err := NewRustUgoiraEncoder().Encode(context.Background(), UgoiraEncodeInput{
		OutputPath: t.TempDir() + "/out.gif",
		Format:     AnimationFormatGIF,
	})
	if err == nil || !strings.Contains(err.Error(), "rust ugoira encoder unavailable") || !strings.Contains(err.Error(), "built without ugoira_rust") {
		t.Fatalf("Encode error = %v", err)
	}
}
