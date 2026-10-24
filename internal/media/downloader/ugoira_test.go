package downloader_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestDownloadUgoiraZipFailureCleansTemporaryZip(t *testing.T) {
	dir := t.TempDir()
	zipURL := "https://i.example/ugoira.zip"
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			9: {
				ID: 9, Title: "ugo", PageCount: 1, Kind: pixiv.ArtworkKindUgoira,
				User: pixiv.User{Name: "author"},
			},
		},
		ugoira: map[int64]pixiv.UgoiraMetadata{
			9: ugoiraMetadata(zipURL),
		},
		downloadErr: errors.New("zip download failed"),
	}
	m := downloader.NewArtworkDownloader(client, dir, "{id}")

	batch, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{9}})
	require.NoError(t, err)
	require.Empty(t, batch.Items)
	require.Len(t, batch.Failures, 1)
	require.Equal(t, int64(9), batch.Failures[0].IllustID)
	matches, err := filepath.Glob(filepath.Join(dir, "9 - ugo", "ugoira-*.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary ugoira zip files remain: %v", matches)
	}
}

func TestDownloadUgoiraDefaultEncoderPublishesAnimation(t *testing.T) {
	for _, test := range []struct {
		name   string
		format downloader.UgoiraFormat
		ext    string
	}{
		{name: "default GIF", ext: ".gif"},
		{name: "explicit APNG", format: downloader.UgoiraFormatAPNG, ext: ".apng"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			zipURL := "https://i.example/ugoira.zip"
			var frame bytes.Buffer
			require.NoError(t, png.Encode(&frame, image.NewNRGBA(image.Rect(0, 0, 3, 2))))
			meta := ugoiraMetadata(zipURL)
			meta.Frames[0].Filename = "000000.png"
			client := &fakePixivClient{
				details: map[int64]pixiv.Artwork{
					9: {ID: 9, Title: "ugo", PageCount: 1, Kind: pixiv.ArtworkKindUgoira, User: pixiv.User{Name: "author"}},
				},
				ugoira:    map[int64]pixiv.UgoiraMetadata{9: meta},
				downloads: map[string][]byte{zipURL: makeZip(t, "000000.png", frame.Bytes())},
			}
			m := downloader.NewArtworkDownloader(client, dir, "{id}")
			output := filepath.Join(dir, "9 - ugo", "9"+test.ext)
			require.NoError(t, os.MkdirAll(filepath.Dir(output), 0o755))
			require.NoError(t, os.WriteFile(output, []byte("old-animation"), 0o600))

			batch, err := m.Download(t.Context(), downloader.DownloadRequest{IllustIDs: []int64{9}, UgoiraFormat: test.format})
			require.NoError(t, err)
			require.Empty(t, batch.Failures)
			require.Empty(t, batch.Warnings)
			require.Len(t, batch.Items, 1)
			require.Equal(t, int64(9), batch.Items[0].IllustID)
			require.Equal(t, "ugoira", batch.Items[0].Type)
			require.Equal(t, []downloader.DownloadedFile{{Path: output, Page: 1}}, batch.Items[0].Files)
			require.Equal(t, []string{zipURL}, client.savedURLs)
			body, err := os.ReadFile(output)
			require.NoError(t, err)
			if test.ext == ".gif" {
				animation, err := gif.DecodeAll(bytes.NewReader(body))
				require.NoError(t, err)
				require.Len(t, animation.Image, 1)
				require.Equal(t, 3, animation.Config.Width)
				require.Equal(t, 2, animation.Config.Height)
				require.Equal(t, []int{8}, animation.Delay)
			} else {
				config, err := png.DecodeConfig(bytes.NewReader(body))
				require.NoError(t, err)
				require.Equal(t, 3, config.Width)
				require.Equal(t, 2, config.Height)
				require.Contains(t, string(body), "acTL")
			}
			entries, err := os.ReadDir(filepath.Dir(output))
			require.NoError(t, err)
			require.Len(t, entries, 1)
			require.Equal(t, filepath.Base(output), entries[0].Name())
		})
	}
}

func TestDownloadUgoiraDefaultEncoderFailurePreservesExistingAnimation(t *testing.T) {
	for _, format := range []downloader.UgoiraFormat{downloader.UgoiraFormatGIF, downloader.UgoiraFormatAPNG} {
		for _, cancelAfterSave := range []bool{false, true} {
			name := string(format) + "/invalid frame"
			if cancelAfterSave {
				name = string(format) + "/canceled after save"
			}
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				zipURL := "https://i.example/ugoira.zip"
				client := &fakePixivClient{
					details: map[int64]pixiv.Artwork{
						9: {ID: 9, Title: "ugo", PageCount: 1, Kind: pixiv.ArtworkKindUgoira},
					},
					ugoira: map[int64]pixiv.UgoiraMetadata{9: ugoiraMetadata(zipURL)},
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				client.saveResourceOverride = func(_ context.Context, _ sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
					body := makeZip(t, "000000.jpg", []byte("invalid image"))
					err := os.WriteFile(options.Path, body, 0o600)
					if cancelAfterSave {
						cancel()
					}
					return sdk.SavedResource{Path: options.Path, Size: int64(len(body))}, err
				}
				m := downloader.NewArtworkDownloader(client, dir, "{id}")
				output := filepath.Join(dir, "9 - ugo", "9."+string(format))
				require.NoError(t, os.MkdirAll(filepath.Dir(output), 0o755))
				require.NoError(t, os.WriteFile(output, []byte("old-animation"), 0o600))

				batch, err := m.Download(ctx, downloader.DownloadRequest{IllustIDs: []int64{9}, UgoiraFormat: format})
				require.Empty(t, batch.Items)
				if cancelAfterSave {
					require.ErrorIs(t, err, context.Canceled)
					require.Empty(t, batch.Failures)
				} else {
					require.NoError(t, err)
					require.Len(t, batch.Failures, 1)
					require.Equal(t, int64(9), batch.Failures[0].IllustID)
					require.Error(t, batch.Failures[0].Cause)
					require.Equal(t, batch.Failures[0].Cause.Error(), batch.Failures[0].Message)
				}
				assertFileBody(t, output, "old-animation")
				entries, err := os.ReadDir(filepath.Dir(output))
				require.NoError(t, err)
				require.Len(t, entries, 1)
				require.Equal(t, filepath.Base(output), entries[0].Name())
			})
		}
	}
}

func TestDownloadUgoiraFallsBackToDefaultFilenameWithWarning(t *testing.T) {
	for _, test := range []struct {
		name     string
		template string
	}{
		{name: "invalid template", template: "{unsupported}"},
		{name: "empty rendered name", template: "{tags}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			zipURL := "https://i.example/ugoira.zip"
			var frame bytes.Buffer
			require.NoError(t, png.Encode(&frame, image.NewNRGBA(image.Rect(0, 0, 2, 2))))
			meta := ugoiraMetadata(zipURL)
			meta.Frames[0].Filename = "000000.png"
			client := &fakePixivClient{
				details: map[int64]pixiv.Artwork{
					9: {
						ID: 9, Title: "ugo", PageCount: 1, Kind: pixiv.ArtworkKindUgoira,
						User: pixiv.User{Name: "author"},
					},
				},
				ugoira:    map[int64]pixiv.UgoiraMetadata{9: meta},
				downloads: map[string][]byte{zipURL: makeZip(t, "000000.png", frame.Bytes())},
			}
			m := downloader.NewArtworkDownloader(client, dir, test.template)

			batch, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{9}})
			require.NoError(t, err)
			require.Len(t, batch.Items, 1)
			require.Len(t, batch.Items[0].Files, 1)
			require.Equal(t, "author - ugo_9.gif", filepath.Base(batch.Items[0].Files[0].Path))
			require.NotEqual(t, ".gif", filepath.Base(batch.Items[0].Files[0].Path))
			require.Len(t, batch.Warnings, 1)
			require.Equal(t, int64(9), batch.Warnings[0].IllustID)
			require.Equal(t, string(pixiv.ArtworkKindUgoira), batch.Warnings[0].Type)
			require.Contains(t, strings.ToLower(batch.Warnings[0].Message), "default")
			body, err := os.ReadFile(batch.Items[0].Files[0].Path)
			require.NoError(t, err)
			_, err = gif.DecodeAll(bytes.NewReader(body))
			require.NoError(t, err)
		})
	}
}

func TestDownloadUgoiraUsesOriginalArchive(t *testing.T) {
	dir := t.TempDir()
	originalURL := "https://i.example/original.zip"
	mediumURL := "https://i.example/medium.zip"
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			9: {ID: 9, Title: "ugo", PageCount: 1, Kind: pixiv.ArtworkKindUgoira, User: pixiv.User{Name: "author"}},
		},
		ugoira: map[int64]pixiv.UgoiraMetadata{
			9: {
				Frames: []pixiv.UgoiraFrame{{Filename: "000000.jpg", DelayMilliseconds: 80}},
				Archives: []pixiv.UgoiraArchive{
					{Quality: pixiv.UgoiraQualityMedium, Resource: testResource(mediumURL)},
					{Quality: pixiv.UgoiraQualityOriginal, Resource: testResource(originalURL)},
				},
			},
		},
		downloads: map[string][]byte{originalURL: makeZip(t, "000000.jpg", []byte("frame"))},
	}
	m := downloader.NewArtworkDownloader(client, dir, "{id}")

	if _, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{9}}); err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if len(client.savedURLs) != 1 || client.savedURLs[0] != originalURL {
		t.Fatalf("saved URLs = %v", client.savedURLs)
	}
}

func TestDownloadUgoiraRejectsQualityAndPages(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			9: {ID: 9, Title: "u", Kind: pixiv.ArtworkKindUgoira, PageCount: 1, User: pixiv.User{Name: "a"}},
		},
	}
	m := downloader.NewArtworkDownloader(client, dir, "{id}")
	qualityBatch, err := m.Download(context.Background(), downloader.DownloadRequest{
		IllustIDs: []int64{9},
		Quality:   downloader.DownloadQualityRegular,
	})
	require.NoError(t, err)
	require.Empty(t, qualityBatch.Items)
	require.Len(t, qualityBatch.Failures, 1)
	require.Contains(t, qualityBatch.Failures[0].Message, "ugoira quality")
	pagesBatch, err := m.Download(context.Background(), downloader.DownloadRequest{
		IllustIDs: []int64{9},
		Pages:     []int{1},
		Quality:   downloader.DownloadQualityOriginal,
	})
	require.NoError(t, err)
	require.Empty(t, pagesBatch.Items)
	require.Len(t, pagesBatch.Failures, 1)
	require.Contains(t, pagesBatch.Failures[0].Message, "page selection is unsupported")
}
