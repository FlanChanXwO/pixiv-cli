package downloader_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestDownloadServiceStopsImmediatelyWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	service := downloader.DownloadService{NewBatchDownloader: func(downloader.DownloadClient, string, string) (downloader.BatchDownloader, error) {
		return &downloadManagerStub{download: func(ctx context.Context, _ downloader.DownloadRequest) (downloader.DownloadReport, error) {
			calls++
			return downloader.DownloadReport{}, ctx.Err()
		}}, nil
	}}

	report, err := service.DownloadSources(ctx, &downloadSourcesStub{}, []string{"1"}, downloader.DownloadRequest{})
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls)
	require.Empty(t, report.Items)
	require.Empty(t, report.Failures)
}

func TestDownloadReturnsContextErrorSeparatelyFromBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	manager := downloader.NewArtworkDownloader(&fakePixivClient{}, t.TempDir(), "{id}")
	batch, err := manager.Download(ctx, downloader.DownloadRequest{IllustIDs: []int64{42}})

	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, batch.Items)
	require.Empty(t, batch.Failures)
	require.Empty(t, batch.Warnings)
}

// TestDownloadReturnsStructuredBatchResultOnPartialFailure 验证批量下载的业务失败
// 不再被压成普通 error：成功作品和带 typed Cause 的失败项必须同时返回。
func TestDownloadReturnsStructuredBatchResultOnPartialFailure(t *testing.T) {
	dir := t.TempDir()
	typedFailure := sdk.NewError("pixiv", "SaveResource", sdk.ResourceForbidden)
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			1: {
				ID: 1, Title: "ok1", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
				User:  pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{artworkPage("https://i.example/1.jpg", 0)},
			},
			2: {
				ID: 2, Title: "ok2", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
				User:  pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{artworkPage("https://i.example/2.jpg", 0)},
			},
			3: {
				ID: 3, Title: "fails", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
				User:  pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{artworkPage("https://i.example/3.jpg", 0)},
			},
		},
		downloads: map[string][]byte{
			"https://i.example/1.jpg": []byte("p1"),
			"https://i.example/2.jpg": []byte("p2"),
			"https://i.example/3.jpg": []byte("p3"),
		},
	}
	// 只让第 3 个作品在 SaveResource 时失败，其余按 ref payload 查表写盘。
	var saveMu sync.Mutex
	client.saveResourceOverride = func(_ context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
		saveMu.Lock()
		defer saveMu.Unlock()
		if filepath.Base(options.Path) == "3.jpg" {
			return sdk.SavedResource{}, typedFailure
		}
		payload, err := sdk.ResourceRefPayload(ref)
		if err != nil {
			return sdk.SavedResource{}, err
		}
		body, ok := client.downloads[string(payload)]
		if !ok {
			return sdk.SavedResource{}, os.ErrNotExist
		}
		client.savedURLs = append(client.savedURLs, string(payload))
		client.destinations = append(client.destinations, options.Path)
		if err := os.WriteFile(options.Path, body, 0o644); err != nil {
			return sdk.SavedResource{}, err
		}
		return sdk.SavedResource{Path: options.Path, Size: int64(len(body)), ContentType: "image/jpeg"}, nil
	}
	m := downloader.NewArtworkDownloader(client, dir, "{id}")

	batch, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{1, 2, 3}})
	require.NoError(t, err)
	require.Len(t, batch.Items, 2)
	require.Len(t, batch.Failures, 1)
	require.Equal(t, int64(3), batch.Failures[0].IllustID)
	require.Same(t, typedFailure, batch.Failures[0].Cause)
	// 两个成功作品都已落盘。
	published := map[string]bool{}
	for _, artwork := range batch.Items {
		for _, file := range artwork.Files {
			body, rerr := os.ReadFile(file.Path)
			if rerr != nil {
				t.Fatalf("ReadFile(%q): %v", file.Path, rerr)
			}
			published[string(body)] = true
		}
	}
	if !published["p1"] || !published["p2"] {
		t.Fatalf("published bodies = %v, want p1 and p2", published)
	}
}

// TestDownloadReturnsStructuredBatchResultWhenAllArtworksFail 验证全部业务失败时
// 仍返回每个失败项，而不是把第一个失败伪装成 operation error。
func TestDownloadReturnsStructuredBatchResultWhenAllArtworksFail(t *testing.T) {
	dir := t.TempDir()
	wantFailure := errors.New("network broke")
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			1: {
				ID: 1, Title: "fails", PageCount: 1, Kind: pixiv.ArtworkKindIllustration,
				User:  pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{artworkPage("https://i.example/1.jpg", 0)},
			},
		},
		downloadErr: wantFailure,
	}
	m := downloader.NewArtworkDownloader(client, dir, "{id}")

	batch, err := m.Download(context.Background(), downloader.DownloadRequest{IllustIDs: []int64{1}})
	require.NoError(t, err)
	require.Empty(t, batch.Items)
	require.Len(t, batch.Failures, 1)
	require.Equal(t, int64(1), batch.Failures[0].IllustID)
	require.ErrorIs(t, batch.Failures[0].Cause, wantFailure)
}
