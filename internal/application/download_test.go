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
	manager := &downloadManagerStub{download: func(gotContext context.Context, gotIDs []int64) ([]application.DownloadedArtwork, error) {
		require.Same(t, ctx, gotContext)
		require.Equal(t, []int64{42, 84}, gotIDs)
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

func TestDownloadServiceRejectsMissingFactory(t *testing.T) {
	_, err := (application.DownloadService{}).Download(context.Background(), downloadClientStub{}, application.DownloadRequest{})

	require.EqualError(t, err, "download manager factory is not configured")
}

func TestDownloadServiceRejectsMissingOperationClient(t *testing.T) {
	factoryCalled := false
	service := application.DownloadService{NewManager: func(application.DownloadClient, string, string) (application.DownloadManager, error) {
		factoryCalled = true
		return &downloadManagerStub{download: func(context.Context, []int64) ([]application.DownloadedArtwork, error) {
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
		return &downloadManagerStub{download: func(context.Context, []int64) ([]application.DownloadedArtwork, error) {
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
		return &downloadManagerStub{download: func(context.Context, []int64) ([]application.DownloadedArtwork, error) {
			return nil, want
		}}, nil
	}}

	_, err := service.Download(context.Background(), downloadClientStub{}, application.DownloadRequest{IllustIDs: []int64{42}})

	require.ErrorIs(t, err, want)
}

type downloadManagerStub struct {
	download func(context.Context, []int64) ([]application.DownloadedArtwork, error)
}

type typedNilDownloadManager struct{}

func (*typedNilDownloadManager) Download(context.Context, []int64) ([]application.DownloadedArtwork, error) {
	panic("typed-nil download manager must not be called")
}

func (m *downloadManagerStub) Download(ctx context.Context, ids []int64) ([]application.DownloadedArtwork, error) {
	return m.download(ctx, ids)
}

type downloadClientStub struct{}

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

func (*typedNilDownloadClient) Download(context.Context, sdk.ResourceRef, string) error {
	return nil
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

func (downloadClientStub) Download(context.Context, sdk.ResourceRef, string) error {
	return nil
}
