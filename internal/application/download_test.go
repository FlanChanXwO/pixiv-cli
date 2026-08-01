package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/stretchr/testify/require"
)

func TestDownloadServicePropagatesFactoryFailure(t *testing.T) {
	want := errors.New("create download manager")
	service := application.DownloadService{NewManager: func(application.DownloadClient, string, string) (application.DownloadManager, error) {
		return nil, want
	}}

	_, err := service.Download(context.Background(), downloadClientStub{}, application.DownloadRequest{
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
	want := []application.DownloadedArtwork{{
		IllustID: 42,
		Title:    "work",
		Author:   "artist",
		Type:     "illust",
		Files:    []application.DownloadedFile{{Path: "/tmp/downloads/42.jpg", Page: 3}},
	}}
	manager := &downloadManagerStub{download: func(gotContext context.Context, request application.DownloadRequest) ([]application.DownloadedArtwork, error) {
		require.Same(t, ctx, gotContext)
		require.Equal(t, []int64{42, 84}, request.IllustIDs)
		return want, nil
	}}
	service := application.DownloadService{NewManager: func(gotClient application.DownloadClient, gotPath, gotTemplate string) (application.DownloadManager, error) {
		require.Same(t, client, gotClient)
		require.Equal(t, "/tmp/downloads", gotPath)
		require.Equal(t, "{id}-{title}", gotTemplate)
		return manager, nil
	}}

	got, err := service.Download(ctx, client, application.DownloadRequest{
		IllustIDs:        []int64{42, 84},
		DownloadPath:     "/tmp/downloads",
		FilenameTemplate: "{id}-{title}",
	})

	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestDownloadServiceReportsArtworkFailuresAndPreservesInputOrder(t *testing.T) {
	client := downloadTargetClientStub{}
	var calls []int64
	service := application.DownloadService{NewManager: func(application.DownloadClient, string, string) (application.DownloadManager, error) {
		return &downloadManagerStub{download: func(_ context.Context, request application.DownloadRequest) ([]application.DownloadedArtwork, error) {
			require.Len(t, request.IllustIDs, 1)
			id := request.IllustIDs[0]
			calls = append(calls, id)
			if id == 2 {
				return nil, errors.New("upstream unavailable")
			}
			return []application.DownloadedArtwork{{IllustID: id, Title: "work"}}, nil
		}}, nil
	}}

	report, err := service.DownloadTargets(context.Background(), client, []sdk.Reference{
		{Kind: sdk.ReferenceKindArtwork, ID: 1},
		{Kind: sdk.ReferenceKindArtwork, ID: 2},
		{Kind: sdk.ReferenceKindArtwork, ID: 1},
	}, application.DownloadRequest{})

	require.NoError(t, err)
	require.Equal(t, []int64{1, 2, 1}, calls)
	require.Equal(t, []application.DownloadedArtwork{{IllustID: 1, Title: "work"}, {IllustID: 1, Title: "work"}}, report.Items)
	require.Len(t, report.Failures, 1)
	require.Equal(t, int64(2), report.Failures[0].IllustID)
	require.Equal(t, "https://www.pixiv.net/artworks/2", report.Failures[0].URL)
	require.Equal(t, "upstream unavailable", report.Failures[0].Message)
}

func TestDownloadServiceExpandsEveryVisualArtworkTypeForUserTargets(t *testing.T) {
	var requests []sdk.UserArtworksRequest
	client := userArtworksDownloadClient{userArtworks: func(_ context.Context, request sdk.UserArtworksRequest) (*sdk.IllustListResult, error) {
		requests = append(requests, request)
		switch request.Type {
		case sdk.IllustTypeIllust:
			if request.Cursor == "" {
				return &sdk.IllustListResult{Illusts: []sdk.Illust{{ID: 1, Type: "illust"}}, NextCursor: "illust-next"}, nil
			}
			return &sdk.IllustListResult{Illusts: []sdk.Illust{{ID: 2, Type: "illust"}}}, nil
		case sdk.IllustTypeManga:
			return &sdk.IllustListResult{Illusts: []sdk.Illust{{ID: 3, Type: "manga"}}}, nil
		case sdk.IllustTypeUgoira:
			return &sdk.IllustListResult{Illusts: []sdk.Illust{{ID: 4, Type: "ugoira"}}}, nil
		default:
			return nil, errors.New("unexpected artwork type")
		}
	}}
	var downloaded []int64
	service := application.DownloadService{NewManager: func(application.DownloadClient, string, string) (application.DownloadManager, error) {
		return &downloadManagerStub{download: func(_ context.Context, request application.DownloadRequest) ([]application.DownloadedArtwork, error) {
			id := request.IllustIDs[0]
			downloaded = append(downloaded, id)
			return []application.DownloadedArtwork{{IllustID: id}}, nil
		}}, nil
	}}

	report, err := service.DownloadTargets(context.Background(), client, []sdk.Reference{{Kind: sdk.ReferenceKindUser, ID: 7}}, application.DownloadRequest{})

	require.NoError(t, err)
	require.Empty(t, report.Failures)
	require.Equal(t, []int64{1, 2, 3, 4}, downloaded)
	require.Equal(t, []sdk.UserArtworksRequest{
		{UserID: 7, Type: sdk.IllustTypeIllust},
		{UserID: 7, Type: sdk.IllustTypeIllust, Cursor: "illust-next"},
		{UserID: 7, Type: sdk.IllustTypeManga},
		{UserID: 7, Type: sdk.IllustTypeUgoira},
	}, requests)
}

func TestDownloadSourcesDeduplicatesCanonicalArtworkAndReportsOriginalSourceIndex(t *testing.T) {
	client := &downloadAllClientStub{}
	service := application.DownloadService{}
	var progress []sdk.DownloadProgress
	_, err := service.DownloadSources(context.Background(), client, []string{"1", "https://www.pixiv.net/artworks/1", "2"}, sdk.DownloadOptions{
		Progress: func(event sdk.DownloadProgress) { progress = append(progress, event) },
	})

	require.NoError(t, err)
	require.Equal(t, []string{
		"https://www.pixiv.net/artworks/1",
		"https://www.pixiv.net/artworks/2",
	}, client.sources)
	require.Len(t, progress, 1)
	require.Equal(t, 2, progress[0].SourceIndex)
}

func TestDownloadSourcesDeduplicatesArtworkAndExpandedUserWorksByFirstOccurrence(t *testing.T) {
	client := &downloadAllUserClientStub{userArtworksDownloadClient: userArtworksDownloadClient{
		userArtworks: func(_ context.Context, request sdk.UserArtworksRequest) (*sdk.IllustListResult, error) {
			if request.Type == sdk.IllustTypeIllust {
				return &sdk.IllustListResult{Illusts: []sdk.Illust{{ID: 1, Type: "illust"}, {ID: 2, Type: "illust"}}}, nil
			}
			return &sdk.IllustListResult{}, nil
		},
	}}

	_, err := (application.DownloadService{}).DownloadSources(context.Background(), client, []string{"1", "https://www.pixiv.net/users/7/artworks"}, sdk.DownloadOptions{})

	require.NoError(t, err)
	require.Equal(t, []string{
		"https://www.pixiv.net/artworks/1",
		"https://www.pixiv.net/artworks/2",
	}, client.sources)
}

func TestDownloadSourcesExpandsPublicIllustrationSeriesAndDeduplicates(t *testing.T) {
	var requests []sdk.IllustSeriesRequest
	client := &downloadAllSeriesClientStub{series: func(_ context.Context, request sdk.IllustSeriesRequest) (*sdk.IllustListResult, error) {
		requests = append(requests, request)
		if request.Cursor == "" {
			return &sdk.IllustListResult{Illusts: []sdk.Illust{{ID: 1}, {ID: 2}}, NextCursor: "next"}, nil
		}
		return &sdk.IllustListResult{Illusts: []sdk.Illust{{ID: 2}, {ID: 3}}}, nil
	}}
	_, err := (application.DownloadService{}).DownloadSources(context.Background(), client, []string{"1", "https://www.pixiv.net/user/7/series/9"}, sdk.DownloadOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{
		"https://www.pixiv.net/artworks/1",
		"https://www.pixiv.net/artworks/2",
		"https://www.pixiv.net/artworks/3",
	}, client.sources)
	require.Equal(t, []sdk.IllustSeriesRequest{{SeriesID: 9, UserID: 7}, {SeriesID: 9, UserID: 7, Cursor: "next"}}, requests)
}

func TestDownloadServiceStopsImmediatelyWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	service := application.DownloadService{NewManager: func(application.DownloadClient, string, string) (application.DownloadManager, error) {
		return &downloadManagerStub{download: func(ctx context.Context, _ application.DownloadRequest) ([]application.DownloadedArtwork, error) {
			calls++
			return nil, ctx.Err()
		}}, nil
	}}

	report, err := service.DownloadTargets(ctx, downloadTargetClientStub{}, []sdk.Reference{
		{Kind: sdk.ReferenceKindArtwork, ID: 1},
		{Kind: sdk.ReferenceKindArtwork, ID: 2},
	}, application.DownloadRequest{})

	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls)
	require.Empty(t, report.Items)
	require.Empty(t, report.Failures)
}

func TestDownloadServiceRejectsMissingFactory(t *testing.T) {
	_, err := (application.DownloadService{}).Download(context.Background(), downloadClientStub{}, application.DownloadRequest{})

	require.EqualError(t, err, "download manager factory is not configured")
}

func TestDownloadServiceRejectsMissingOperationClient(t *testing.T) {
	factoryCalled := false
	service := application.DownloadService{NewManager: func(application.DownloadClient, string, string) (application.DownloadManager, error) {
		factoryCalled = true
		return &downloadManagerStub{download: func(context.Context, application.DownloadRequest) ([]application.DownloadedArtwork, error) {
			return nil, nil
		}}, nil
	}}

	_, err := service.Download(context.Background(), nil, application.DownloadRequest{})

	require.EqualError(t, err, "download operation client is not configured")
	require.False(t, factoryCalled)
}

func TestDownloadServiceRejectsTypedNilOperationClient(t *testing.T) {
	var client *typedNilDownloadClient
	factoryCalls := 0
	service := application.DownloadService{NewManager: func(application.DownloadClient, string, string) (application.DownloadManager, error) {
		factoryCalls++
		return &downloadManagerStub{download: func(context.Context, application.DownloadRequest) ([]application.DownloadedArtwork, error) {
			return nil, nil
		}}, nil
	}}

	_, err := service.Download(context.Background(), client, application.DownloadRequest{})

	require.EqualError(t, err, "download operation client is not configured")
	require.Zero(t, factoryCalls)
}

func TestDownloadServiceRejectsMissingManager(t *testing.T) {
	service := application.DownloadService{NewManager: func(application.DownloadClient, string, string) (application.DownloadManager, error) {
		return nil, nil
	}}

	_, err := service.Download(context.Background(), downloadClientStub{}, application.DownloadRequest{})

	require.EqualError(t, err, "download manager factory returned nil")
}

func TestDownloadServiceRejectsTypedNilManager(t *testing.T) {
	var manager *typedNilDownloadManager
	service := application.DownloadService{NewManager: func(application.DownloadClient, string, string) (application.DownloadManager, error) {
		return manager, nil
	}}

	_, err := service.Download(context.Background(), downloadClientStub{}, application.DownloadRequest{})

	require.EqualError(t, err, "download manager factory returned nil")
}

func TestDownloadServicePropagatesManagerFailure(t *testing.T) {
	want := errors.New("download failed")
	service := application.DownloadService{NewManager: func(application.DownloadClient, string, string) (application.DownloadManager, error) {
		return &downloadManagerStub{download: func(context.Context, application.DownloadRequest) ([]application.DownloadedArtwork, error) {
			return nil, want
		}}, nil
	}}

	_, err := service.Download(context.Background(), downloadClientStub{}, application.DownloadRequest{IllustIDs: []int64{42}})

	require.ErrorIs(t, err, want)
}

type downloadManagerStub struct {
	download func(context.Context, application.DownloadRequest) ([]application.DownloadedArtwork, error)
}

type typedNilDownloadManager struct{}

func (*typedNilDownloadManager) Download(context.Context, application.DownloadRequest) ([]application.DownloadedArtwork, error) {
	panic("typed-nil download manager must not be called")
}

func (m *downloadManagerStub) Download(ctx context.Context, request application.DownloadRequest) ([]application.DownloadedArtwork, error) {
	return m.download(ctx, request)
}

type downloadClientStub struct{}

type downloadTargetClientStub struct{ downloadClientStub }

type userArtworksDownloadClient struct {
	downloadClientStub
	userArtworks func(context.Context, sdk.UserArtworksRequest) (*sdk.IllustListResult, error)
}

type downloadAllClientStub struct {
	downloadTargetClientStub
	sources []string
}

func (c *downloadAllClientStub) DownloadAllWith(_ context.Context, sources []string, options sdk.DownloadOptions) (sdk.DownloadAllResult, error) {
	c.sources = append([]string(nil), sources...)
	if options.Progress != nil {
		options.Progress(sdk.DownloadProgress{SourceIndex: 1})
	}
	return sdk.DownloadAllResult{Items: make([]sdk.DownloadItemResult, len(sources))}, nil
}

type downloadAllUserClientStub struct {
	userArtworksDownloadClient
	sources []string
}

type downloadAllSeriesClientStub struct {
	downloadTargetClientStub
	series  func(context.Context, sdk.IllustSeriesRequest) (*sdk.IllustListResult, error)
	sources []string
}

func (c *downloadAllSeriesClientStub) IllustSeries(ctx context.Context, request sdk.IllustSeriesRequest) (*sdk.IllustListResult, error) {
	return c.series(ctx, request)
}

func (c *downloadAllSeriesClientStub) DownloadAllWith(_ context.Context, sources []string, _ sdk.DownloadOptions) (sdk.DownloadAllResult, error) {
	c.sources = append([]string(nil), sources...)
	return sdk.DownloadAllResult{Items: make([]sdk.DownloadItemResult, len(sources))}, nil
}

func (c *downloadAllUserClientStub) DownloadAllWith(_ context.Context, sources []string, _ sdk.DownloadOptions) (sdk.DownloadAllResult, error) {
	c.sources = append([]string(nil), sources...)
	return sdk.DownloadAllResult{Items: make([]sdk.DownloadItemResult, len(sources))}, nil
}

type typedNilDownloadClient struct{}

func (*typedNilDownloadClient) IllustDetail(context.Context, int64) (*sdk.IllustDetail, error) {
	return nil, nil
}

func (*typedNilDownloadClient) UgoiraMetadata(context.Context, int64) (*sdk.UgoiraMetadataResult, error) {
	return nil, nil
}

func (*typedNilDownloadClient) ParseResourceRef(string) (sdk.ResourceRef, error) {
	return sdk.ResourceRef{}, nil
}

func (*typedNilDownloadClient) DownloadResource(context.Context, sdk.ResourceRef, string) (sdk.ResourceDownloadResult, error) {
	return sdk.ResourceDownloadResult{}, nil
}

func (downloadClientStub) IllustDetail(context.Context, int64) (*sdk.IllustDetail, error) {
	return nil, nil
}

func (downloadClientStub) UgoiraMetadata(context.Context, int64) (*sdk.UgoiraMetadataResult, error) {
	return nil, nil
}

func (downloadClientStub) ParseResourceRef(string) (sdk.ResourceRef, error) {
	return sdk.ResourceRef{}, nil
}

func (downloadClientStub) DownloadResource(context.Context, sdk.ResourceRef, string) (sdk.ResourceDownloadResult, error) {
	return sdk.ResourceDownloadResult{}, nil
}

func (downloadTargetClientStub) UserArtworks(context.Context, sdk.UserArtworksRequest) (*sdk.IllustListResult, error) {
	return &sdk.IllustListResult{}, nil
}

func (c userArtworksDownloadClient) UserArtworks(ctx context.Context, request sdk.UserArtworksRequest) (*sdk.IllustListResult, error) {
	return c.userArtworks(ctx, request)
}
