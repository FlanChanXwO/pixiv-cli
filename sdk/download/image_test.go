package download_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/download"
	"github.com/stretchr/testify/require"
)

func TestSaveImageQualities(t *testing.T) {
	for _, tc := range []struct {
		quality  download.Quality
		expected string
	}{
		{download.QualityOriginal, "/img-original/img/2026/23_p0.png"},
		{download.QualityRegular, "/c/1200x1200/img-master/img/2026/23_p0_master1200.jpg"},
		{download.QualitySmall, "/c/540x540_70/img-master/img/2026/23_p0_master1200.jpg"},
		{download.QualityThumb, "/c/250x250_80_a2/img-master/img/2026/23_p0_square1200.jpg"},
		{download.QualityMini, "/c/48x48/img-master/img/2026/23_p0_square1200.jpg"},
	} {
		t.Run(string(tc.quality), func(t *testing.T) {
			client := localClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == api.PathIllustDetail {
					fmt.Fprint(w, strings.ReplaceAll(staticArtwork, "/0.png", "/img-original/img/2026/23_p0.png"))
					return
				}
				require.Equal(t, tc.expected, r.URL.Path)
				require.Empty(t, r.Header.Get("Authorization"))
				w.Write([]byte("\x89PNG\r\n\x1a\n"))
			})
			art, err := client.Artwork(23).Get(t.Context())
			require.NoError(t, err)
			report, err := download.New(download.Options{DownloadPath: t.TempDir(), Quality: tc.quality}).Save(t.Context(), art.Pages[0])
			require.NoError(t, err)
			require.Len(t, report.Files, 1)
			require.Equal(t, ".png", filepath.Ext(report.Files[0].Path))
			require.FileExists(t, report.Files[0].Path)
		})
	}
}

func TestSaveImagePreservesExistingFile(t *testing.T) {
	client := localClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == api.PathIllustDetail {
			fmt.Fprint(w, staticArtwork)
			return
		}
		w.Header().Set("Content-Length", "1024")
		fmt.Fprint(w, "\x89PNG\r\n\x1a\n"+strings.Repeat("x", 600))
	})
	art, err := client.Artwork(23).Get(t.Context())
	require.NoError(t, err)
	dir := t.TempDir()
	target := filepath.Join(dir, "23", "23_0.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0700))
	require.NoError(t, os.WriteFile(target, []byte("existing"), 0600))
	report, err := download.New(download.Options{DownloadPath: dir, FilenameTemplate: "{id}_{num}", DirectoryTemplate: "{id}"}).Save(t.Context(), art.Pages[0])
	require.Error(t, err)
	require.Len(t, report.Failures, 1)
	require.Empty(t, report.Files)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "existing", string(data))
	entries, err := os.ReadDir(filepath.Dir(target))
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestSaveRejectsInvalidPathsBeforeMediaIO(t *testing.T) {
	client := localClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, api.PathIllustDetail, r.URL.Path)
		fmt.Fprint(w, staticArtwork)
	})
	art, err := client.Artwork(23).Get(t.Context())
	require.NoError(t, err)
	for _, options := range []download.Options{
		{FilenameTemplate: "{unknown}"},
		{DirectoryTemplate: "../outside"},
		{Quality: "invalid"},
		{UgoiraFormat: "invalid"},
	} {
		options.DownloadPath = t.TempDir()
		_, err := download.New(options).Save(t.Context(), art.Pages[0])
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
		entries, err := os.ReadDir(options.DownloadPath)
		require.NoError(t, err)
		require.Empty(t, entries)
	}
}

func TestSaveRejectsHTMLResponse(t *testing.T) {
	client := localClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == api.PathIllustDetail {
			fmt.Fprint(w, staticArtwork)
			return
		}
		fmt.Fprint(w, "<html><body>upstream error</body></html>")
	})
	art, err := client.Artwork(23).Get(t.Context())
	require.NoError(t, err)
	dir := t.TempDir()
	report, err := download.New(download.Options{DownloadPath: dir}).Save(t.Context(), art.Pages[0])
	require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
	require.Len(t, report.Failures, 1)
	require.Empty(t, report.Files)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}
