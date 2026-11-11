package download_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk/download"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

const staticArtwork = `{"illust":{"id":23,"title":"pages","type":"manga","create_date":"2026-01-01T00:00:00Z","user":{"id":11,"name":"author"},"page_count":3,"meta_pages":[{"image_urls":{"original":"https://i.pximg.net/0.png"}},{"image_urls":{"original":"https://i.pximg.net/1.png"}},{"image_urls":{"original":"https://i.pximg.net/2.png"}}]}}`
const animatedArtwork = `{"illust":{"id":24,"title":"animation","type":"ugoira","create_date":"2026-01-01T00:00:00Z","user":{"id":11},"page_count":1,"image_urls":{"large":"https://i.pximg.net/cover.jpg"}}}`
const animationMetadata = `{"ugoira_metadata":{"zip_urls":{"original":"https://i.pximg.net/frames.zip"},"frames":[{"file":"0.png","delay":80},{"file":"1.png","delay":120}]}}`

func TestSaveSelectedPages(t *testing.T) {
	var paths []string
	client := localClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == api.PathIllustDetail {
			fmt.Fprint(w, strings.Replace(staticArtwork, `"id":23`, `"id":`+r.URL.Query().Get(api.ParamIllustID), 1))
			return
		}
		paths = append(paths, r.URL.Path)
		w.Write([]byte("\x89PNG\r\n\x1a\n"))
	})
	art, err := client.Artwork(23).Get(t.Context())
	require.NoError(t, err)
	other, err := client.Artwork(25).Get(t.Context())
	require.NoError(t, err)
	dir := t.TempDir()
	report, err := download.New(download.Options{DownloadPath: dir, FilenameTemplate: "{id}_{num}", DirectoryTemplate: "{author}/{num}"}).Save(t.Context(), art.Pages[1], other.Pages[2])
	require.NoError(t, err)
	require.Empty(t, report.Failures)
	require.Len(t, report.Files, 2)
	require.Equal(t, []string{"/1.png", "/2.png"}, paths)
	require.Equal(t, download.File{ArtworkID: 23, PageIndex: 1, Path: filepath.Join(dir, "author", "1", "23_1.png")}, report.Files[0])
	require.Equal(t, download.File{ArtworkID: 25, PageIndex: 2, Path: filepath.Join(dir, "author", "2", "25_2.png")}, report.Files[1])
	data, err := os.ReadFile(report.Files[0].Path)
	require.NoError(t, err)
	require.Equal(t, []byte("\x89PNG\r\n\x1a\n"), data)
}

func TestSavePartialFailure(t *testing.T) {
	client := localClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == api.PathIllustDetail {
			fmt.Fprint(w, staticArtwork)
			return
		}
		if r.URL.Path == "/1.png" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Write([]byte("\x89PNG\r\n\x1a\n"))
	})
	art, err := client.Artwork(23).Get(t.Context())
	require.NoError(t, err)
	dir := t.TempDir()
	report, err := download.New(download.Options{DownloadPath: dir, FilenameTemplate: "{id}_{num}", DirectoryTemplate: "{id}"}).Save(t.Context(), art.Pages...)
	require.Error(t, err)
	require.Len(t, report.Failures, 1)
	require.Equal(t, 1, report.Failures[0].PageIndex)
	require.EqualValues(t, 23, report.Failures[0].ArtworkID)
	require.ErrorIs(t, err, report.Failures[0].Cause)
	require.Len(t, report.Files, 2)
	for i, page := range []int{0, 2} {
		require.Equal(t, page, report.Files[i].PageIndex)
		require.FileExists(t, report.Files[i].Path)
	}
	require.NoFileExists(t, filepath.Join(dir, "23", "23_1.png"))
}

func TestSaveCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var mediaCalls atomic.Int32
	client := localClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == api.PathIllustDetail {
			fmt.Fprint(w, staticArtwork)
			return
		}
		mediaCalls.Add(1)
		cancel()
		<-r.Context().Done()
	})
	art, err := client.Artwork(23).Get(t.Context())
	require.NoError(t, err)
	dir := t.TempDir()
	report, err := download.New(download.Options{DownloadPath: dir}).Save(ctx, art.Pages...)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, report.Files)
	require.EqualValues(t, 1, mediaCalls.Load())
	files, err := filepath.Glob(filepath.Join(dir, "*", "*"))
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestSaveUgoira(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for i := range 2 {
		frame, err := writer.Create(fmt.Sprintf("%d.png", i))
		require.NoError(t, err)
		pixels := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		pixels.Set(0, 0, color.NRGBA{R: uint8(255 * i), A: 255})
		require.NoError(t, png.Encode(frame, pixels))
	}
	require.NoError(t, writer.Close())
	client := localClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case api.PathIllustDetail:
			fmt.Fprint(w, animatedArtwork)
		case api.PathUgoiraMetadata:
			fmt.Fprint(w, animationMetadata)
		case "/frames.zip":
			w.Write(archive.Bytes())
		}
	})
	art, err := client.Artwork(24).Get(t.Context())
	require.NoError(t, err)
	for _, format := range []download.UgoiraFormat{download.UgoiraFormatGIF, download.UgoiraFormatAPNG} {
		t.Run(string(format), func(t *testing.T) {
			report, err := download.New(download.Options{DownloadPath: t.TempDir(), FilenameTemplate: "{unknown}", UgoiraFormat: format}).Save(t.Context(), art.Pages...)
			require.NoError(t, err)
			require.Len(t, report.Warnings, 1)
			require.Error(t, report.Warnings[0].Cause)
			require.Empty(t, report.Failures)
			require.Len(t, report.Files, 1)
			data, err := os.ReadFile(report.Files[0].Path)
			require.NoError(t, err)
			require.Equal(t, "."+string(format), filepath.Ext(report.Files[0].Path))
			assertAnimation(t, format, data)
		})
	}
}

func assertAnimation(t *testing.T, format download.UgoiraFormat, data []byte) {
	t.Helper()
	if format == download.UgoiraFormatGIF {
		animation, err := gif.DecodeAll(bytes.NewReader(data))
		require.NoError(t, err)
		require.Len(t, animation.Image, 2)
		require.Equal(t, []int{8, 12}, animation.Delay)
		require.Equal(t, image.Rect(0, 0, 2, 2), animation.Image[0].Bounds())
	} else {
		frame, err := png.Decode(bytes.NewReader(data))
		require.NoError(t, err)
		require.Equal(t, image.Rect(0, 0, 2, 2), frame.Bounds())
		require.Contains(t, string(data), "acTL")
		require.Equal(t, 2, bytes.Count(data, []byte("fcTL")))
	}
}

func localClient(t *testing.T, handler http.HandlerFunc) *pixiv.Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	connection := server.Client()
	transport := connection.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = server.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	value := pixiv.NewCredentials("fixture-refresh")
	value.SetTokens("fixture-access", "fixture-refresh")
	value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
	store, err := pixiv.NewMemoryCredentials(value)
	require.NoError(t, err)
	client, err := pixiv.New(store, pixiv.Options{HTTPClient: connection})
	require.NoError(t, err)
	t.Cleanup(connection.CloseIdleConnections)
	return client
}
