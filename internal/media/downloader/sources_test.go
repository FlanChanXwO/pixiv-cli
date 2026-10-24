package downloader_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestDownloadSourcesDeduplicatesCanonicalArtwork(t *testing.T) {
	client := &downloadSourcesStub{}
	var downloaded []int64
	service := downloader.DownloadService{NewBatchDownloader: func(downloader.DownloadClient, string, string) (downloader.BatchDownloader, error) {
		return &batchDownloaderStub{download: func(_ context.Context, request downloader.DownloadRequest) (downloader.DownloadReport, error) {
			downloaded = append(downloaded, request.IllustIDs...)
			return downloader.DownloadReport{Items: []downloader.DownloadedArtwork{{IllustID: 1}}}, nil
		}}, nil
	}}

	report, err := service.DownloadSources(context.Background(), client, []string{"1", "https://www.pixiv.net/artworks/1", "2"}, downloader.DownloadRequest{})
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, downloaded)
	require.Len(t, report.Failures, 0)
}

func TestDownloadSourcesExpandsUserArtworksAndDeduplicates(t *testing.T) {
	client := &downloadSourcesStub{}
	client.userArtworks = func(_ context.Context, request pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{
			{ID: 1, Kind: pixiv.ArtworkKindIllustration},
			{ID: 2, Kind: pixiv.ArtworkKindIllustration},
		}}, nil
	}
	var downloaded []int64
	service := downloader.DownloadService{NewBatchDownloader: func(downloader.DownloadClient, string, string) (downloader.BatchDownloader, error) {
		return &batchDownloaderStub{download: func(_ context.Context, request downloader.DownloadRequest) (downloader.DownloadReport, error) {
			downloaded = append(downloaded, request.IllustIDs...)
			return downloader.DownloadReport{Items: []downloader.DownloadedArtwork{{IllustID: 1}}}, nil
		}}, nil
	}}

	report, err := service.DownloadSources(context.Background(), client, []string{"1", "https://www.pixiv.net/users/7/artworks"}, downloader.DownloadRequest{})
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, downloaded)
	require.Len(t, report.Failures, 0)
}

func TestDownloadSourcesPreservesBatchFailuresAndCommittedItems(t *testing.T) {
	client := &downloadSourcesStub{}
	cause := sdk.NewError("pixiv", "SaveResource", sdk.ResourceForbidden)
	wantItems := []downloader.DownloadedArtwork{{IllustID: 42}}
	wantFailures := []downloader.DownloadFailure{{
		IllustID: 84,
		Message:  cause.Error(),
		Cause:    cause,
	}}
	service := downloader.DownloadService{NewBatchDownloader: func(downloader.DownloadClient, string, string) (downloader.BatchDownloader, error) {
		return &batchDownloaderStub{download: func(_ context.Context, request downloader.DownloadRequest) (downloader.DownloadReport, error) {
			require.Equal(t, []int64{42}, request.IllustIDs)
			return downloader.DownloadReport{Items: wantItems, Failures: wantFailures}, nil
		}}, nil
	}}

	report, err := service.DownloadSources(context.Background(), client, []string{"42"}, downloader.DownloadRequest{})
	require.NoError(t, err)
	require.Equal(t, wantItems, report.Items)
	require.Equal(t, wantFailures, report.Failures)
	require.True(t, report.Committed)
}

func TestDownloadSourcesRedactsRejectedSource(t *testing.T) {
	source := "https://signed.example/private?signature=secret"
	client := &downloadSourcesStub{}

	report, err := (downloader.DownloadService{}).DownloadSources(context.Background(), client, []string{source}, downloader.DownloadRequest{})

	require.NoError(t, err)
	require.Len(t, report.Failures, 1)
	require.Equal(t, "[redacted source]", report.Failures[0].URL)
	require.NotContains(t, report.Failures[0].URL, source)
}

func TestDownloadSourcesMarksOpaqueResourceAsResource(t *testing.T) {
	dir := t.TempDir()
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"artwork","id":42,"p":0}`))
	if err != nil {
		t.Fatalf("sdk.NewResourceRef: %v", err)
	}
	client := &downloadSourcesStub{
		saveResource: func(_ context.Context, _ sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
			if err := os.WriteFile(options.Path, []byte("resource"), 0o644); err != nil {
				return sdk.SavedResource{}, err
			}
			return sdk.SavedResource{Path: options.Path, Size: int64(len("resource"))}, nil
		},
	}

	report, err := (downloader.DownloadService{}).DownloadSources(context.Background(), client, []string{ref.String()}, downloader.DownloadRequest{DownloadPath: dir})

	if err != nil {
		t.Fatalf("DownloadSources returned error: %v", err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("items=%+v, want one direct resource item", report.Items)
	}
	if report.Items[0].Type != "resource" {
		t.Fatalf("direct resource type=%q, want %q", report.Items[0].Type, "resource")
	}
	if report.Items[0].IllustID != 0 || report.Items[0].Title != "" || report.Items[0].Author != "" {
		t.Fatalf("direct resource leaked artwork metadata: %+v", report.Items[0])
	}
}

func TestDownloadSourcesDownloadsDirectCDNURLWithSafeBasename(t *testing.T) {
	dir := t.TempDir()
	source := "https://i.pximg.net/img-original/img/2026/09/04/12/34/56/789_p0/photo%3A%7C.png?signature=secret"
	var gotURL string
	var gotPath string
	client := &downloadSourcesStub{
		saveResourceURL: func(_ context.Context, rawURL string, options sdk.SaveOptions) (sdk.SavedResource, error) {
			gotURL = rawURL
			gotPath = options.Path
			if err := os.WriteFile(options.Path, []byte("direct-image"), 0o644); err != nil {
				return sdk.SavedResource{}, err
			}
			return sdk.SavedResource{Path: options.Path, Size: int64(len("direct-image")), ContentType: "image/png"}, nil
		},
	}

	report, err := (downloader.DownloadService{}).DownloadSources(context.Background(), client, []string{source}, downloader.DownloadRequest{
		DownloadPath:     dir,
		Pages:            []int{2},
		Quality:          downloader.DownloadQualityRegular,
		UgoiraFormat:     downloader.UgoiraFormatAPNG,
		FilenameTemplate: "{unsupported}",
	})

	require.NoError(t, err)
	require.True(t, report.Committed)
	require.Empty(t, report.Failures)
	require.Equal(t, source, gotURL)
	require.Regexp(t, regexp.MustCompile(`^photo__-[0-9a-f]{12}\.png$`), filepath.Base(gotPath))
	require.Len(t, report.Items, 1)
	require.Zero(t, report.Items[0].IllustID)
	require.Empty(t, report.Items[0].Title)
	require.Empty(t, report.Items[0].Author)
	require.Equal(t, "resource", report.Items[0].Type)
	require.Equal(t, []downloader.DownloadedFile{{Path: gotPath, Page: 1}}, report.Items[0].Files)
	assertFileBody(t, gotPath, "direct-image")
}

func TestDownloadSourcesUsesDistinctPathsForDirectURLsWithSameBasename(t *testing.T) {
	dir := t.TempDir()
	sources := []string{
		"https://i.pximg.net/a/photo.png",
		"https://i.pximg.net/b/photo.png",
	}
	var paths []string
	client := &downloadSourcesStub{
		saveResourceURL: func(_ context.Context, rawURL string, options sdk.SaveOptions) (sdk.SavedResource, error) {
			paths = append(paths, options.Path)
			body := []byte(rawURL)
			if err := os.WriteFile(options.Path, body, 0o600); err != nil {
				return sdk.SavedResource{}, err
			}
			return sdk.SavedResource{Path: options.Path, Size: int64(len(body)), ContentType: "image/png"}, nil
		},
	}

	report, err := (downloader.DownloadService{}).DownloadSources(context.Background(), client, sources, downloader.DownloadRequest{DownloadPath: dir})

	require.NoError(t, err)
	require.True(t, report.Committed)
	require.Empty(t, report.Failures)
	require.Len(t, report.Items, len(sources))
	require.Len(t, paths, len(sources))
	require.NotEqual(t, paths[0], paths[1])
	for index, path := range paths {
		assertFileBody(t, path, sources[index])
	}
}

func TestDownloadSourcesUsesDistinctPathsForDistinctDirectURLQueries(t *testing.T) {
	dir := t.TempDir()
	sources := []string{
		"https://i.pximg.net/a/photo.png?signature=one",
		"https://i.pximg.net/a/photo.png?signature=two",
	}
	var paths []string
	client := &downloadSourcesStub{
		saveResourceURL: func(_ context.Context, rawURL string, options sdk.SaveOptions) (sdk.SavedResource, error) {
			paths = append(paths, options.Path)
			body := []byte(rawURL)
			if err := os.WriteFile(options.Path, body, 0o600); err != nil {
				return sdk.SavedResource{}, err
			}
			return sdk.SavedResource{Path: options.Path, Size: int64(len(body)), ContentType: "image/png"}, nil
		},
	}

	report, err := (downloader.DownloadService{}).DownloadSources(context.Background(), client, sources, downloader.DownloadRequest{DownloadPath: dir})

	require.NoError(t, err)
	require.True(t, report.Committed)
	require.Empty(t, report.Failures)
	require.Len(t, report.Items, len(sources))
	require.Len(t, paths, len(sources))
	require.NotEqual(t, paths[0], paths[1])
	for index, path := range paths {
		assertFileBody(t, path, sources[index])
	}
}

func TestDownloadSourcesPreservesDiagnosticTextWhenRedactingDirectURL(t *testing.T) {
	source := "https://i.pximg.net/a/photo.png?page=1"
	client := &downloadSourcesStub{
		saveResourceURL: func(_ context.Context, rawURL string, _ sdk.SaveOptions) (sdk.SavedResource, error) {
			return sdk.SavedResource{}, errors.New("GET " + rawURL + " failed: HTTP/1.1 attempt 1 page 1")
		},
	}

	report, err := (downloader.DownloadService{}).DownloadSources(context.Background(), client, []string{source}, downloader.DownloadRequest{DownloadPath: t.TempDir()})

	require.NoError(t, err)
	require.Len(t, report.Failures, 1)
	require.Equal(t, "GET [redacted source] failed: HTTP/1.1 attempt 1 page 1", report.Failures[0].Message)
	require.NotContains(t, report.Failures[0].Message, source)
	require.NotContains(t, report.Failures[0].Message, "page=1")
}

func TestDownloadSourcesReportsDirectURLFailuresWithoutSensitiveData(t *testing.T) {
	sources := []string{
		"https://i.pximg.net/img-original/img/2026/09/04/123_p0.jpg?signature=secret-one",
		"https://i.pximg.net/img-original/img/2026/09/04/456_p0.jpg?signature=secret-two",
	}
	var calls []string
	client := &downloadSourcesStub{
		saveResourceURL: func(_ context.Context, rawURL string, _ sdk.SaveOptions) (sdk.SavedResource, error) {
			calls = append(calls, rawURL)
			return sdk.SavedResource{}, errors.New("GET " + rawURL + " failed: signature=secret")
		},
	}

	report, err := (downloader.DownloadService{}).DownloadSources(context.Background(), client, sources, downloader.DownloadRequest{DownloadPath: t.TempDir()})

	require.NoError(t, err)
	require.Equal(t, sources, calls)
	require.Len(t, report.Failures, len(sources))
	for _, failure := range report.Failures {
		require.Equal(t, "[redacted source]", failure.URL)
		require.NotNil(t, failure.Cause)
		require.NotContains(t, failure.Message, "https://i.pximg.net/")
		require.NotContains(t, failure.Message, "secret")
		require.NotContains(t, failure.Cause.Error(), "https://i.pximg.net/")
		require.NotContains(t, failure.Cause.Error(), "secret")
	}
}

func TestDownloadSourcesRejectsDirectURLWithoutUsableBasename(t *testing.T) {
	client := &downloadSourcesStub{
		saveResourceURL: func(context.Context, string, sdk.SaveOptions) (sdk.SavedResource, error) {
			t.Fatal("direct resource URL without a basename must not be downloaded")
			return sdk.SavedResource{}, nil
		},
	}

	report, err := (downloader.DownloadService{}).DownloadSources(context.Background(), client, []string{"https://i.pximg.net/img-original/img/"}, downloader.DownloadRequest{DownloadPath: t.TempDir()})

	require.NoError(t, err)
	require.Len(t, report.Failures, 1)
	require.Equal(t, "[redacted source]", report.Failures[0].URL)
	require.Contains(t, report.Failures[0].Message, "basename")
	require.NotNil(t, report.Failures[0].Cause)
}

func TestDownloadSourcesPropagatesDirectContextErrors(t *testing.T) {
	for _, test := range []struct {
		name    string
		wantErr error
	}{
		{name: "canceled", wantErr: context.Canceled},
		{name: "deadline exceeded", wantErr: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := &directResourceErrorContext{
				Context: context.Background(),
				wantErr: test.wantErr,
				done:    make(chan struct{}),
			}
			calls := 0
			client := &downloadSourcesStub{
				saveResourceURL: func(_ context.Context, _ string, options sdk.SaveOptions) (sdk.SavedResource, error) {
					calls++
					if calls == 1 {
						if err := os.WriteFile(options.Path, []byte("done"), 0o600); err != nil {
							return sdk.SavedResource{}, err
						}
						return sdk.SavedResource{Path: options.Path, Size: 4}, nil
					}
					ctx.trigger()
					return sdk.SavedResource{}, test.wantErr
				},
			}

			report, err := (downloader.DownloadService{}).DownloadSources(ctx, client, []string{
				"https://i.pximg.net/first.jpg",
				"https://i.pximg.net/second.jpg",
			}, downloader.DownloadRequest{DownloadPath: t.TempDir()})

			require.ErrorIs(t, err, test.wantErr)
			require.Equal(t, 2, calls)
			require.Len(t, report.Items, 1)
			require.Empty(t, report.Failures)
			require.True(t, report.Committed)
		})
	}
}

func TestDownloadSourcesPropagatesOpaqueContextErrors(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"artwork","id":42,"p":0}`))
	require.NoError(t, err)
	ctx := &directResourceErrorContext{
		Context: context.Background(),
		wantErr: context.Canceled,
		done:    make(chan struct{}),
	}
	client := &downloadSourcesStub{
		saveResource: func(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error) {
			ctx.trigger()
			return sdk.SavedResource{}, context.Canceled
		},
	}

	report, err := (downloader.DownloadService{}).DownloadSources(ctx, client, []string{ref.String()}, downloader.DownloadRequest{DownloadPath: t.TempDir()})

	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, report.Items)
	require.Empty(t, report.Failures)
}

// TestDirectResourceUsesCollisionResistantNames 验证两个共享前缀的 resource ref
// 必须落盘到不同文件，而不是互相覆盖；命名对完整 ref 做 sha256 摘要。
func TestDirectResourceUsesCollisionResistantNames(t *testing.T) {
	dir := t.TempDir()
	// 两个 ref 仅末尾页号不同，共享长前缀；截断式文件名会让它们冲突。
	refA, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"artwork","id":42,"p":0}`))
	if err != nil {
		t.Fatal(err)
	}
	refB, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"artwork","id":42,"p":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if refA.String() == refB.String() {
		t.Fatal("test fixtures must produce distinct refs")
	}
	var seen []string
	client := &downloadSourcesStub{
		saveResource: func(_ context.Context, _ sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
			seen = append(seen, options.Path)
			if err := os.WriteFile(options.Path, []byte("data"), 0o644); err != nil {
				return sdk.SavedResource{}, err
			}
			return sdk.SavedResource{Path: options.Path, Size: 4}, nil
		},
	}
	service := downloader.DownloadService{NewBatchDownloader: func(downloader.DownloadClient, string, string) (downloader.BatchDownloader, error) {
		return &batchDownloaderStub{download: func(context.Context, downloader.DownloadRequest) (downloader.DownloadReport, error) {
			return downloader.DownloadReport{}, nil
		}}, nil
	}}

	report, err := service.DownloadSources(context.Background(), client, []string{refA.String(), refB.String()}, downloader.DownloadRequest{DownloadPath: dir})
	require.NoError(t, err)
	require.Len(t, report.Failures, 0)
	require.Len(t, report.Items, 2)
	if len(seen) != 2 || seen[0] == seen[1] {
		t.Fatalf("expected 2 distinct resource paths, got %v", seen)
	}
	// 两个 ref 必须都落盘（第二个不会覆盖第一个）。
	for _, path := range seen {
		assertFileBody(t, path, "data")
	}
}
