package reversesearch_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	"github.com/stretchr/testify/require"
)

type searcherFunc func(context.Context, reversesearch.Request) (reversesearch.Response, error)

func (f searcherFunc) Search(ctx context.Context, request reversesearch.Request) (reversesearch.Response, error) {
	return f(ctx, request)
}

type payloadSearcherFunc func(context.Context, reversesearch.PayloadRequest) (reversesearch.Response, error)

func (payloadSearcherFunc) Preflight(context.Context, reversesearch.PayloadQuery) error { return nil }

func (f payloadSearcherFunc) SearchPayload(ctx context.Context, request reversesearch.PayloadRequest) (reversesearch.Response, error) {
	return f(ctx, request)
}

type preflightPayloadSearcher struct {
	preflight func(context.Context, reversesearch.PayloadQuery) error
	search    payloadSearcherFunc
}

func (s preflightPayloadSearcher) Preflight(ctx context.Context, query reversesearch.PayloadQuery) error {
	return s.preflight(ctx, query)
}

func (s preflightPayloadSearcher) SearchPayload(ctx context.Context, request reversesearch.PayloadRequest) (reversesearch.Response, error) {
	return s.search(ctx, request)
}

func TestFacadePreflightFailureDoesNotReadSource(t *testing.T) {
	wantErr := reversesearch.NewError(reversesearch.CodeMissingCredential, "SauceNAO API key is required", nil)
	searchCalls := 0
	facade := reversesearch.NewFacade(reversesearch.Dependencies{
		Sources: reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: t.TempDir()}),
		Payloads: preflightPayloadSearcher{
			preflight: func(_ context.Context, query reversesearch.PayloadQuery) error {
				require.Equal(t, reversesearch.ProviderSauceNAO, query.Provider)
				require.True(t, query.PixivOnly)
				return wantErr
			},
			search: func(context.Context, reversesearch.PayloadRequest) (reversesearch.Response, error) {
				searchCalls++
				return reversesearch.Response{}, nil
			},
		},
	})

	_, err := facade.Search(context.Background(), reversesearch.Request{
		Source: "/sensitive/source-that-must-not-be-opened", Provider: reversesearch.ProviderSauceNAO, PixivOnly: true,
	})
	require.ErrorIs(t, err, wantErr)
	require.Zero(t, searchCalls)
}

func TestFacadeLoadsOnePayloadSnapshotDelegatesAndCleansIt(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.bin")
	require.NoError(t, os.WriteFile(sourcePath, []byte("image"), 0o600))
	snapshotDir := t.TempDir()
	delegations := 0
	facade := reversesearch.NewFacade(reversesearch.Dependencies{
		Sources: reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: snapshotDir}),
		Payloads: payloadSearcherFunc(func(_ context.Context, request reversesearch.PayloadRequest) (reversesearch.Response, error) {
			delegations++
			require.Equal(t, reversesearch.ProviderASCII2DColor, request.Provider)
			require.False(t, request.PixivOnly)
			for range 2 {
				reader, err := request.Snapshot.Open()
				require.NoError(t, err)
				body, err := io.ReadAll(reader)
				require.NoError(t, err)
				require.NoError(t, reader.Close())
				require.Equal(t, []byte("image"), body)
			}
			return reversesearch.Response{}, nil
		}),
	})
	var searcher reversesearch.Searcher = facade

	response, err := searcher.Search(context.Background(), reversesearch.Request{
		Source: sourcePath, Provider: reversesearch.ProviderASCII2DColor, PixivOnly: false,
	})
	require.NoError(t, err)
	require.Equal(t, 1, delegations)
	require.Equal(t, reversesearch.SourceKindFile, response.Input.Kind)
	require.NotEmpty(t, response.Input.SHA256)
	files, err := os.ReadDir(snapshotDir)
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestFacadePreservesSafeInputAndCleansSnapshotWhenPayloadSearchFails(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.bin")
	require.NoError(t, os.WriteFile(sourcePath, []byte("image"), 0o600))
	snapshotDir := t.TempDir()
	wantErr := reversesearch.NewError(reversesearch.CodeProviderFailed, "provider query failed", errors.New("private cause"))
	facade := reversesearch.NewFacade(reversesearch.Dependencies{
		Sources: reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: snapshotDir}),
		Payloads: payloadSearcherFunc(func(_ context.Context, _ reversesearch.PayloadRequest) (reversesearch.Response, error) {
			return reversesearch.Response{}, wantErr
		}),
	})

	response, err := facade.Search(context.Background(), reversesearch.Request{Source: sourcePath, Provider: reversesearch.ProviderSauceNAO})
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, reversesearch.SourceKindFile, response.Input.Kind)
	require.NotEmpty(t, response.Input.SHA256)
	files, readErr := os.ReadDir(snapshotDir)
	require.NoError(t, readErr)
	require.Empty(t, files)
}

func TestClassifiedErrorKeepsCodeAndCauseWithoutRenderingSensitiveCause(t *testing.T) {
	cause := fmt.Errorf("open /sensitive/source.png: %w", context.Canceled)
	err := reversesearch.NewError(reversesearch.CodeSourceReadFailed, "could not read image source", cause)

	require.EqualError(t, err, "could not read image source")
	require.Equal(t, reversesearch.CodeSourceReadFailed, reversesearch.CodeOf(err))
	require.ErrorIs(t, err, context.Canceled)
	require.NotContains(t, err.Error(), "/sensitive/source.png")
	require.Equal(t, reversesearch.CodeUnknown, reversesearch.CodeOf(errors.New("plain error")))
}

func TestDomainContractUsesStableProviderValues(t *testing.T) {
	providers := []reversesearch.Provider{
		reversesearch.ProviderSauceNAO,
		reversesearch.ProviderASCII2DColor,
		reversesearch.ProviderASCII2DBOVW,
		reversesearch.ProviderAll,
	}
	require.Equal(t, []reversesearch.Provider{"saucenao", "ascii2d-color", "ascii2d-bovw", "all"}, providers)

	var searcher reversesearch.Searcher = searcherFunc(func(_ context.Context, request reversesearch.Request) (reversesearch.Response, error) {
		return reversesearch.Response{Input: reversesearch.Input{Kind: reversesearch.SourceKindFile, SHA256: "digest"}}, nil
	})
	response, err := searcher.Search(context.Background(), reversesearch.Request{
		Source:    "opaque-source",
		Provider:  reversesearch.ProviderSauceNAO,
		PixivOnly: true,
	})
	require.NoError(t, err)
	require.Equal(t, reversesearch.SourceKindFile, response.Input.Kind)
}
