//go:build cgo && ((darwin && (amd64 || arm64)) || (linux && (amd64 || arm64)) || (windows && (amd64 || arm64)))

package ugoira_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	ugoira "github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
	"github.com/stretchr/testify/require"
)

// 目标路径已经是目录时，Rust 编码完成后的发布必须显式失败且不能替换现有目标。
func TestRustEncoderDoesNotReplaceExistingDestination(t *testing.T) {
	directory := t.TempDir()
	zipPath := filepath.Join(directory, "ugoira.zip")
	createZip(t, zipPath, "000000.jpg", rustUgoiraJPEG(t))
	outputPath := filepath.Join(directory, "existing.gif")
	require.NoError(t, os.Mkdir(outputPath, 0o755))

	err := ugoira.NewRustEncoder().Encode(context.Background(), ugoira.Input{
		ZipPath:    zipPath,
		Frames:     []ugoira.Frame{{File: "000000.jpg", Delay: 80}},
		WorkDir:    directory,
		OutputPath: outputPath,
		Format:     ugoira.FormatGIF,
	})
	require.Error(t, err)

	info, statErr := os.Stat(outputPath)
	require.NoError(t, statErr)
	require.True(t, info.IsDir())
	temporary, globErr := filepath.Glob(filepath.Join(directory, ".ugoira-*.gif"))
	require.NoError(t, globErr)
	require.Empty(t, temporary)
}
