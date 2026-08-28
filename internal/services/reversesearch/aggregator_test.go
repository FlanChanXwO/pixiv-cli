package reversesearch_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	"github.com/stretchr/testify/require"
)

type providerClientStub struct {
	preflight func(context.Context) error
	search    func(context.Context, *reversesearch.Snapshot) (reversesearch.ProviderResponse, error)
}

type ascii2DClientStub struct {
	preflight func(context.Context) error
	upload    func(context.Context, *reversesearch.Snapshot) (reversesearch.ASCII2DSession, error)
}

func (stub ascii2DClientStub) Preflight(ctx context.Context) error {
	if stub.preflight == nil {
		return nil
	}
	return stub.preflight(ctx)
}

func (stub ascii2DClientStub) Upload(ctx context.Context, snapshot *reversesearch.Snapshot) (reversesearch.ASCII2DSession, error) {
	return stub.upload(ctx, snapshot)
}

type ascii2DSessionFunc func(context.Context, reversesearch.Provider) (reversesearch.ProviderResponse, error)

func (search ascii2DSessionFunc) Search(ctx context.Context, provider reversesearch.Provider) (reversesearch.ProviderResponse, error) {
	return search(ctx, provider)
}

func (stub providerClientStub) Preflight(ctx context.Context) error {
	if stub.preflight == nil {
		return nil
	}
	return stub.preflight(ctx)
}

func (stub providerClientStub) Search(ctx context.Context, snapshot *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
	return stub.search(ctx, snapshot)
}

func TestAggregatorSingleSauceNAOSuccessProducesCanonicalEnvelope(t *testing.T) {
	quota := &reversesearch.Quota{ShortRemaining: 3, LongRemaining: 97, ShortLimit: 4, LongLimit: 100}
	sauce := providerClientStub{search: func(_ context.Context, snapshot *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
		require.NotNil(t, snapshot)
		return reversesearch.ProviderResponse{
			Provider: reversesearch.ProviderSauceNAO,
			Quota:    quota,
			Matches: []reversesearch.Match{{
				Rank: 1, Similarity: 91.23, IndexID: 5, IndexName: "Pixiv Images",
				Title: "Fixture title", Author: "Fixture author", ArtworkID: 123, UserID: 456,
				ExternalURLs: []string{"https://www.pixiv.net/artworks/123"},
			}},
		}, nil
	}}
	searcher := newAggregateFacade(t, reversesearch.AggregatorDependencies{SauceNAO: sauce})

	response, err := searcher.Search(context.Background(), reversesearch.Request{
		Source: fixtureImagePath(t), Provider: reversesearch.ProviderSauceNAO, PixivOnly: true,
	})
	require.NoError(t, err)
	require.Equal(t, []reversesearch.ProviderSummary{{
		Name: reversesearch.ProviderSauceNAO, Status: reversesearch.ProviderStatusSuccess,
		ResultCount: 1, Quota: quota,
	}}, response.Providers)
	require.Equal(t, []reversesearch.Result{{
		Pixiv:  &reversesearch.PixivRef{Type: reversesearch.PixivRefArtwork, ID: 123},
		Title:  "Fixture title",
		Author: "Fixture author",
		Evidence: []reversesearch.Evidence{{
			Provider: reversesearch.ProviderSauceNAO, Rank: 1, Similarity: 91.23,
			IndexID: 5, IndexName: "Pixiv Images", Title: "Fixture title", Author: "Fixture author",
			ExternalURLs: []string{"https://www.pixiv.net/artworks/123"},
		}},
	}}, response.Results)
	require.Empty(t, response.ProviderErrors)
	require.False(t, response.Partial)
	require.Equal(t, reversesearch.SourceKindFile, response.Input.Kind)
	require.NotEmpty(t, response.Input.SHA256)
}

func TestAggregatorSingleProviderFailureReturnsSafeEnvelopeAndError(t *testing.T) {
	privateCause := errors.New("private upstream body and credential")
	wantErr := reversesearch.NewError(reversesearch.CodeUpstreamHTTPStatus, "SauceNAO returned an unsuccessful HTTP status", privateCause)
	privateDiagnostic := errors.New("second private diagnostic")
	joinedErr := errors.Join(wantErr, privateDiagnostic)
	sauce := providerClientStub{search: func(context.Context, *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
		return reversesearch.ProviderResponse{}, joinedErr
	}}
	searcher := newAggregateFacade(t, reversesearch.AggregatorDependencies{SauceNAO: sauce})

	response, err := searcher.Search(context.Background(), reversesearch.Request{
		Source: fixtureImagePath(t), Provider: reversesearch.ProviderSauceNAO,
	})
	require.False(t, errors.Is(err, wantErr))
	require.False(t, errors.Is(err, privateCause))
	require.False(t, errors.Is(err, privateDiagnostic))
	var publicErr *reversesearch.Error
	require.True(t, errors.As(err, &publicErr))
	require.Equal(t, reversesearch.CodeUpstreamHTTPStatus, publicErr.Code())
	require.Nil(t, errors.Unwrap(publicErr))
	require.EqualError(t, err, "SauceNAO returned an unsuccessful HTTP status")
	require.Equal(t, []reversesearch.ProviderSummary{{
		Name: reversesearch.ProviderSauceNAO, Status: reversesearch.ProviderStatusError,
	}}, response.Providers)
	require.NotNil(t, response.Results)
	require.Empty(t, response.Results)
	require.Equal(t, []reversesearch.ProviderError{{
		Provider: reversesearch.ProviderSauceNAO,
		Code:     reversesearch.CodeUpstreamHTTPStatus,
		Message:  "SauceNAO returned an unsuccessful HTTP status",
	}}, response.ProviderErrors)
	require.False(t, response.Partial)
	require.NotEmpty(t, response.Input.SHA256)
	require.NotContains(t, response.ProviderErrors[0].Message, privateCause.Error())
	require.NotContains(t, response.ProviderErrors[0].Message, "second private diagnostic")
}

func TestAggregatorAllRunsProviderBranchesAndASCII2DModesConcurrently(t *testing.T) {
	branches := newOverlapBarrier(2)
	modes := newOverlapBarrier(2)
	sauce := providerClientStub{search: func(ctx context.Context, _ *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
		if err := branches.Wait(ctx); err != nil {
			return reversesearch.ProviderResponse{}, err
		}
		return reversesearch.ProviderResponse{Provider: reversesearch.ProviderSauceNAO, Matches: []reversesearch.Match{}}, nil
	}}
	ascii := ascii2DClientStub{upload: func(ctx context.Context, _ *reversesearch.Snapshot) (reversesearch.ASCII2DSession, error) {
		if err := branches.Wait(ctx); err != nil {
			return nil, err
		}
		return ascii2DSessionFunc(func(ctx context.Context, provider reversesearch.Provider) (reversesearch.ProviderResponse, error) {
			if err := modes.Wait(ctx); err != nil {
				return reversesearch.ProviderResponse{}, err
			}
			return reversesearch.ProviderResponse{Provider: provider, Matches: []reversesearch.Match{}}, nil
		}), nil
	}}
	searcher := newAggregateFacade(t, reversesearch.AggregatorDependencies{SauceNAO: sauce, ASCII2D: ascii})
	// 仅作为测试死锁看门狗：两个边界均为纯内存调用，生产路径没有对应超时。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := searcher.Search(ctx, reversesearch.Request{
		Source: fixtureImagePath(t), Provider: reversesearch.ProviderAll,
	})
	require.NoError(t, err)
	require.False(t, response.Partial)
	require.Equal(t, 2, branches.Arrivals())
	require.Equal(t, 2, modes.Arrivals())
}

func TestAggregatorAllFailureReturnsOrderedSafeErrorsAndAggregateError(t *testing.T) {
	const privateDetail = "private source credential and upstream body"
	sauce := providerClientStub{search: func(context.Context, *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
		return reversesearch.ProviderResponse{}, errors.New(privateDetail)
	}}
	ascii := ascii2DClientStub{upload: func(context.Context, *reversesearch.Snapshot) (reversesearch.ASCII2DSession, error) {
		return nil, errors.New(privateDetail)
	}}
	searcher := newAggregateFacade(t, reversesearch.AggregatorDependencies{SauceNAO: sauce, ASCII2D: ascii})

	response, err := searcher.Search(context.Background(), reversesearch.Request{
		Source: fixtureImagePath(t), Provider: reversesearch.ProviderAll,
	})
	require.Equal(t, reversesearch.CodeAllProvidersFailed, reversesearch.CodeOf(err))
	require.EqualError(t, err, "all reverse search providers failed")
	require.Equal(t, []reversesearch.Provider{
		reversesearch.ProviderSauceNAO, reversesearch.ProviderASCII2DColor, reversesearch.ProviderASCII2DBOVW,
	}, []reversesearch.Provider{
		response.ProviderErrors[0].Provider, response.ProviderErrors[1].Provider, response.ProviderErrors[2].Provider,
	})
	for _, providerError := range response.ProviderErrors {
		require.Equal(t, reversesearch.CodeProviderFailed, providerError.Code)
		require.Equal(t, "reverse search provider failed", providerError.Message)
		require.NotContains(t, providerError.Message, privateDetail)
	}
	require.NotNil(t, response.Results)
	require.Empty(t, response.Results)
	require.False(t, response.Partial)
	require.NotContains(t, err.Error(), privateDetail)
}

func TestAggregatorCancellationAbortsWholeAllRequestWithoutPartial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sauce := providerClientStub{search: func(context.Context, *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
		return reversesearch.ProviderResponse{Provider: reversesearch.ProviderSauceNAO, Matches: []reversesearch.Match{{Rank: 1, ArtworkID: 1}}}, nil
	}}
	ascii := ascii2DClientStub{upload: func(context.Context, *reversesearch.Snapshot) (reversesearch.ASCII2DSession, error) {
		cancel()
		return nil, context.Canceled
	}}
	searcher := newAggregateFacade(t, reversesearch.AggregatorDependencies{SauceNAO: sauce, ASCII2D: ascii})

	response, err := searcher.Search(ctx, reversesearch.Request{
		Source: fixtureImagePath(t), Provider: reversesearch.ProviderAll,
	})
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, response.Partial)
	require.Empty(t, response.Providers)
	require.Empty(t, response.Results)
	require.Empty(t, response.ProviderErrors)
	require.NotEmpty(t, response.Input.SHA256)
}

func TestAggregatorSinglePreflightRejectsBeforeReadingSource(t *testing.T) {
	missingKey := reversesearch.NewError(reversesearch.CodeMissingCredential, "SauceNAO API key is required", nil)
	searches := 0
	sauce := providerClientStub{
		preflight: func(context.Context) error { return missingKey },
		search: func(context.Context, *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
			searches++
			return reversesearch.ProviderResponse{}, nil
		},
	}
	searcher := newAggregateFacade(t, reversesearch.AggregatorDependencies{SauceNAO: sauce})

	_, err := searcher.Search(context.Background(), reversesearch.Request{
		Source: "/private/source-that-must-not-be-read", Provider: reversesearch.ProviderSauceNAO,
	})
	require.ErrorIs(t, err, missingKey)
	require.Zero(t, searches)

	_, err = searcher.Search(context.Background(), reversesearch.Request{
		Source: "/private/source-that-must-not-be-read", Provider: reversesearch.Provider("unknown"),
	})
	require.Equal(t, reversesearch.CodeInvalidRequest, reversesearch.CodeOf(err))
	require.EqualError(t, err, "reverse search provider is invalid")
}

func TestAggregatorAllOneModeFailureIsPartialAndKeepsOtherResults(t *testing.T) {
	sauce := providerClientStub{search: func(context.Context, *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
		return reversesearch.ProviderResponse{Provider: reversesearch.ProviderSauceNAO, Matches: []reversesearch.Match{{Rank: 1, ArtworkID: 1}}}, nil
	}}
	bovwFailure := reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d returned a malformed result page", nil)
	ascii := ascii2DClientStub{upload: func(context.Context, *reversesearch.Snapshot) (reversesearch.ASCII2DSession, error) {
		return ascii2DSessionFunc(func(_ context.Context, provider reversesearch.Provider) (reversesearch.ProviderResponse, error) {
			if provider == reversesearch.ProviderASCII2DBOVW {
				return reversesearch.ProviderResponse{}, bovwFailure
			}
			return reversesearch.ProviderResponse{Provider: provider, Matches: []reversesearch.Match{{Rank: 1, ArtworkID: 2}}}, nil
		}), nil
	}}
	searcher := newAggregateFacade(t, reversesearch.AggregatorDependencies{SauceNAO: sauce, ASCII2D: ascii})

	response, err := searcher.Search(context.Background(), reversesearch.Request{
		Source: fixtureImagePath(t), Provider: reversesearch.ProviderAll, PixivOnly: true,
	})
	require.NoError(t, err)
	require.True(t, response.Partial)
	require.Equal(t, []int64{1, 2}, []int64{response.Results[0].Pixiv.ID, response.Results[1].Pixiv.ID})
	require.Equal(t, []reversesearch.ProviderSummary{
		{Name: reversesearch.ProviderSauceNAO, Status: reversesearch.ProviderStatusSuccess, ResultCount: 1},
		{Name: reversesearch.ProviderASCII2DColor, Status: reversesearch.ProviderStatusSuccess, ResultCount: 1},
		{Name: reversesearch.ProviderASCII2DBOVW, Status: reversesearch.ProviderStatusError},
	}, response.Providers)
	require.Equal(t, []reversesearch.ProviderError{{
		Provider: reversesearch.ProviderASCII2DBOVW,
		Code:     reversesearch.CodeMalformedUpstreamResponse,
		Message:  "ascii2d returned a malformed result page",
	}}, response.ProviderErrors)
}

type overlapBarrier struct {
	want     int
	arrivals chan struct{}
	release  chan struct{}
	once     sync.Once
}

func newOverlapBarrier(want int) *overlapBarrier {
	return &overlapBarrier{want: want, arrivals: make(chan struct{}, want), release: make(chan struct{})}
}

func (barrier *overlapBarrier) Wait(ctx context.Context) error {
	barrier.arrivals <- struct{}{}
	if len(barrier.arrivals) == barrier.want {
		barrier.once.Do(func() { close(barrier.release) })
	}
	select {
	case <-barrier.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (barrier *overlapBarrier) Arrivals() int { return len(barrier.arrivals) }

func newAggregateFacade(t *testing.T, dependencies reversesearch.AggregatorDependencies) *reversesearch.Facade {
	t.Helper()
	return reversesearch.NewFacade(reversesearch.Dependencies{
		Sources:  reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: t.TempDir()}),
		Payloads: reversesearch.NewAggregator(dependencies),
	})
}

func fixtureImagePath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture-image")
	require.NoError(t, os.WriteFile(path, []byte("fixture-image"), 0o600))
	return path
}
