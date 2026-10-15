package download

import (
	"os"
	"path/filepath"
	"testing"

	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/stretchr/testify/require"
)

func TestBuildDownloadOutDoesNotFabricateArtworkURLForDirectResource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asset.jpg")
	require.NoError(t, os.WriteFile(path, []byte("jpeg"), 0o644))

	out, err := buildDownloadOut(deliveryLocalPath, []downloader.DownloadedArtwork{
		{
			Type:  "resource",
			Files: []downloader.DownloadedFile{{Path: path, Page: 1}},
		},
	})

	require.NoError(t, err)
	require.Len(t, out.Items, 1)
	require.Empty(t, out.Items[0].URL)
	require.Zero(t, out.Items[0].IllustID)
	require.Equal(t, "resource", out.Items[0].Type)
	require.NotContains(t, out.Text, "artworks/0")
}
