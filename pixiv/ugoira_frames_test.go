package pixiv

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/ugoira"
	"github.com/stretchr/testify/require"
)

func TestExtractUgoiraFramesPublishesTimingManifest(t *testing.T) {
	directory := t.TempDir()
	zipPath := filepath.Join(directory, "source.zip")
	archive, err := os.Create(zipPath)
	require.NoError(t, err)
	writer := zip.NewWriter(archive)
	entry, err := writer.Create("000000.jpg")
	require.NoError(t, err)
	_, err = entry.Write([]byte("frame"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, archive.Close())

	output := filepath.Join(directory, "frames")
	require.NoError(t, extractUgoiraFrames(context.Background(), zipPath, output, []ugoira.Frame{{File: "000000.jpg", Delay: 80}}))
	body, err := os.ReadFile(filepath.Join(output, "frames.json"))
	require.NoError(t, err)
	require.JSONEq(t, `{"frames":[{"file":"000000.jpg","delay":80}]}`, string(body))
	frame, err := os.ReadFile(filepath.Join(output, "000000.jpg"))
	require.NoError(t, err)
	require.Equal(t, "frame", string(frame))
}
