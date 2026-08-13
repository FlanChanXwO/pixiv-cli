package downloader_test

import (
	"context"
	"errors"
	"testing"

	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestDownloadServicePropagatesFactoryFailure(t *testing.T) {
	want := errors.New("create download manager")
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return nil, want
	}}

	_, err := service.Download(context.Background(), &downloadClientStub{}, downloader.DownloadRequest{
		IllustIDs:        []int64{42},
		DownloadPath:     "/tmp/downloads",
		FilenameTemplate: "{id}",
	})

	require.ErrorIs(t, err, want)
}

func TestDownloadServiceDelegatesOperationClientAndRequest(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "same-context")
	client := &downloadClientStub{}
	want := []downloader.DownloadedArtwork{{
		IllustID: 42,
		Title:    "work",
		Author:   "artist",
		Type:     "illust",
		Files:    []downloader.DownloadedFile{{Path: "/tmp/downloads/42.jpg", Page: 3}},
	}}
	manager := &downloadManagerStub{download: func(gotContext context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
		require.Same(t, ctx, gotContext)
		require.Equal(t, []int64{42, 84}, request.IllustIDs)
		return want, nil
	}}
	service := downloader.DownloadService{NewManager: func(gotClient downloader.DownloadClient, gotPath, gotTemplate string) (downloader.DownloadManager, error) {
		require.Same(t, client, gotClient)
		require.Equal(t, "/tmp/downloads", gotPath)
		require.Equal(t, "{id}-{title}", gotTemplate)
		return manager, nil
	}}

	got, err := service.Download(ctx, client, downloader.DownloadRequest{
		IllustIDs:        []int64{42, 84},
		DownloadPath:     "/tmp/downloads",
		FilenameTemplate: "{id}-{title}",
	})

	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestDownloadSourcesDeduplicatesCanonicalArtwork(t *testing.T) {
	client := &downloadSourcesStub{}
	var downloaded []int64
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return &downloadManagerStub{download: func(_ context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			downloaded = append(downloaded, request.IllustIDs...)
			return []downloader.DownloadedArtwork{{IllustID: 1}}, nil
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
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return &downloadManagerStub{download: func(_ context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			downloaded = append(downloaded, request.IllustIDs...)
			return []downloader.DownloadedArtwork{{IllustID: 1}}, nil
		}}, nil
	}}

	report, err := service.DownloadSources(context.Background(), client, []string{"1", "https://www.pixiv.net/users/7/artworks"}, downloader.DownloadRequest{})
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, downloaded)
	require.Len(t, report.Failures, 0)
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

func TestDownloadServiceStopsImmediatelyWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return &downloadManagerStub{download: func(ctx context.Context, _ downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			calls++
			return nil, ctx.Err()
		}}, nil
	}}

	report, err := service.DownloadSources(ctx, &downloadSourcesStub{}, []string{"1"}, downloader.DownloadRequest{})
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls)
	require.Empty(t, report.Items)
	require.Empty(t, report.Failures)
}

func TestDownloadServiceRejectsMissingFactory(t *testing.T) {
	_, err := (downloader.DownloadService{}).Download(context.Background(), &downloadClientStub{}, downloader.DownloadRequest{})
	require.EqualError(t, err, "download manager factory is not configured")
}

func TestDownloadServiceRejectsMissingOperationClient(t *testing.T) {
	factoryCalled := false
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		factoryCalled = true
		return &downloadManagerStub{download: func(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			return nil, nil
		}}, nil
	}}

	_, err := service.Download(context.Background(), nil, downloader.DownloadRequest{})
	require.EqualError(t, err, "download operation client is not configured")
	require.False(t, factoryCalled)
}

func TestDownloadServiceRejectsTypedNilOperationClient(t *testing.T) {
	var client *typedNilDownloadClient
	factoryCalls := 0
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		factoryCalls++
		return &downloadManagerStub{download: func(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			return nil, nil
		}}, nil
	}}

	_, err := service.Download(context.Background(), client, downloader.DownloadRequest{})
	require.EqualError(t, err, "download operation client is not configured")
	require.Zero(t, factoryCalls)
}

func TestDownloadServiceRejectsMissingManager(t *testing.T) {
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return nil, nil
	}}

	_, err := service.Download(context.Background(), &downloadClientStub{}, downloader.DownloadRequest{})
	require.EqualError(t, err, "download manager factory returned nil")
}

func TestDownloadServiceRejectsTypedNilManager(t *testing.T) {
	var manager *typedNilDownloadManager
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return manager, nil
	}}

	_, err := service.Download(context.Background(), &downloadClientStub{}, downloader.DownloadRequest{})
	require.EqualError(t, err, "download manager factory returned nil")
}

func TestDownloadServicePropagatesManagerFailure(t *testing.T) {
	want := errors.New("download failed")
	service := downloader.DownloadService{NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return &downloadManagerStub{download: func(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			return nil, want
		}}, nil
	}}

	_, err := service.Download(context.Background(), &downloadClientStub{}, downloader.DownloadRequest{IllustIDs: []int64{42}})
	require.ErrorIs(t, err, want)
}

type downloadManagerStub struct {
	download func(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error)
}

func (m *downloadManagerStub) Download(ctx context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
	return m.download(ctx, request)
}

type typedNilDownloadManager struct{}

func (*typedNilDownloadManager) Download(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
	panic("typed-nil download manager must not be called")
}

type downloadClientStub struct {
	artwork          func(context.Context, pixiv.ArtworkRequest) (pixiv.Artwork, error)
	ugoiraMetadata   func(context.Context, pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error)
	parseResourceRef func(string) (sdk.ResourceRef, error)
	saveResource     func(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
}

func (c *downloadClientStub) Artwork(ctx context.Context, request pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	if c.artwork != nil {
		return c.artwork(ctx, request)
	}
	return pixiv.Artwork{}, nil
}

func (c *downloadClientStub) UgoiraMetadata(ctx context.Context, request pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error) {
	if c.ugoiraMetadata != nil {
		return c.ugoiraMetadata(ctx, request)
	}
	return pixiv.UgoiraMetadata{}, nil
}

func (c *downloadClientStub) ParseResourceRef(value string) (sdk.ResourceRef, error) {
	if c.parseResourceRef != nil {
		return c.parseResourceRef(value)
	}
	return sdk.ResourceRef{}, nil
}

func (c *downloadClientStub) SaveResource(ctx context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	if c.saveResource != nil {
		return c.saveResource(ctx, ref, options)
	}
	return sdk.SavedResource{}, nil
}

type typedNilDownloadClient struct{}

func (*typedNilDownloadClient) Artwork(context.Context, pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	panic("typed-nil download client must not be called")
}

func (*typedNilDownloadClient) UgoiraMetadata(context.Context, pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error) {
	panic("typed-nil download client must not be called")
}

func (*typedNilDownloadClient) ParseResourceRef(string) (sdk.ResourceRef, error) {
	panic("typed-nil download client must not be called")
}

func (*typedNilDownloadClient) SaveResource(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error) {
	panic("typed-nil download client must not be called")
}

// downloadSourcesStub 实现下载用例需要的窄 port，只覆写 DownloadSources
// 触达的方法。
type downloadSourcesStub struct {
	userArtworks         func(context.Context, pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	userArtworkBookmarks func(context.Context, pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error)
	saveResource         func(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
}

func (c *downloadSourcesStub) UserArtworks(ctx context.Context, request pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if c.userArtworks != nil {
		return c.userArtworks(ctx, request)
	}
	return sdk.Page[pixiv.Artwork]{}, nil
}

func (c *downloadSourcesStub) UserArtworkBookmarks(ctx context.Context, request pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error) {
	if c.userArtworkBookmarks != nil {
		return c.userArtworkBookmarks(ctx, request)
	}
	return sdk.Page[pixiv.Artwork]{}, nil
}

func (c *downloadSourcesStub) SaveResource(ctx context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	if c.saveResource != nil {
		return c.saveResource(ctx, ref, options)
	}
	return sdk.SavedResource{}, nil
}

func (c *downloadSourcesStub) Artwork(context.Context, pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	panic("downloadSourcesStub.Artwork must not be called by source expansion")
}

func (c *downloadSourcesStub) UgoiraMetadata(context.Context, pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error) {
	panic("downloadSourcesStub.UgoiraMetadata must not be called by source expansion")
}

var _ downloader.DownloadTargetClient = (*downloadSourcesStub)(nil)
