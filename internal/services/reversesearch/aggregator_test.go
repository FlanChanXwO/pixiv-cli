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
	joinedErr := errors.Join(wantErr, errors.New("second private diagnostic"))
	sauce := providerClientStub{search: func(context.Context, *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
		return reversesearch.ProviderResponse{}, joinedErr
	}}
	searcher := newAggregateFacade(t, reversesearch.AggregatorDependencies{SauceNAO: sauce})

	response, err := searcher.Search(context.Background(), reversesearch.Request{
		Source: fixtureImagePath(t), Provider: reversesearch.ProviderSauceNAO,
	})
	require.ErrorIs(t, err, wantErr)
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

func TestAggregatorSingleASCII2DUsesOneUploadAndSelectedMode(t *testing.T) {
	for _, provider := range []reversesearch.Provider{
		reversesearch.ProviderASCII2DColor,
		reversesearch.ProviderASCII2DBOVW,
	} {
		t.Run(string(provider), func(t *testing.T) {
			uploads := 0
			ascii := ascii2DClientStub{upload: func(_ context.Context, snapshot *reversesearch.Snapshot) (reversesearch.ASCII2DSession, error) {
				uploads++
				require.NotNil(t, snapshot)
				return ascii2DSessionFunc(func(_ context.Context, mode reversesearch.Provider) (reversesearch.ProviderResponse, error) {
					require.Equal(t, provider, mode)
					return reversesearch.ProviderResponse{Provider: provider, Matches: []reversesearch.Match{{
						Rank: 1, IndexName: "external", Title: "External fixture",
						ExternalURLs: []string{"https://example.test/work/1"},
					}}}, nil
				}), nil
			}}
			searcher := newAggregateFacade(t, reversesearch.AggregatorDependencies{ASCII2D: ascii})

			response, err := searcher.Search(context.Background(), reversesearch.Request{
				Source: fixtureImagePath(t), Provider: provider, PixivOnly: false,
			})
			require.NoError(t, err)
			require.Equal(t, 1, uploads)
			require.Equal(t, []reversesearch.ProviderSummary{{
				Name: provider, Status: reversesearch.ProviderStatusSuccess, ResultCount: 1,
			}}, response.Providers)
			require.Len(t, response.Results, 1)
			require.Nil(t, response.Results[0].Pixiv)
			require.Equal(t, provider, response.Results[0].Evidence[0].Provider)
		})
	}
}

func TestAggregatorAllKeepsASCII2DSuccessWhenSauceNAOPreflightFails(t *testing.T) {
	missingKey := reversesearch.NewError(reversesearch.CodeMissingCredential, "SauceNAO API key is required", nil)
	sauceSearches := 0
	sauce := providerClientStub{
		preflight: func(context.Context) error { return missingKey },
		search: func(context.Context, *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
			sauceSearches++
			return reversesearch.ProviderResponse{}, nil
		},
	}
	uploads := 0
	ascii := ascii2DClientStub{upload: func(context.Context, *reversesearch.Snapshot) (reversesearch.ASCII2DSession, error) {
		uploads++
		return ascii2DSessionFunc(func(_ context.Context, provider reversesearch.Provider) (reversesearch.ProviderResponse, error) {
			id := int64(200)
			if provider == reversesearch.ProviderASCII2DColor {
				id = 100
			}
			return reversesearch.ProviderResponse{Provider: provider, Matches: []reversesearch.Match{{
				Rank: 1, IndexName: "pixiv", ArtworkID: id, Title: string(provider),
			}}}, nil
		}), nil
	}}
	searcher := newAggregateFacade(t, reversesearch.AggregatorDependencies{SauceNAO: sauce, ASCII2D: ascii})

	response, err := searcher.Search(context.Background(), reversesearch.Request{
		Source: fixtureImagePath(t), Provider: reversesearch.ProviderAll, PixivOnly: true,
	})
	require.NoError(t, err)
	require.Zero(t, sauceSearches)
	require.Equal(t, 1, uploads, "color 与 bovw 必须共享一次上传")
	require.Equal(t, []reversesearch.ProviderSummary{
		{Name: reversesearch.ProviderSauceNAO, Status: reversesearch.ProviderStatusError},
		{Name: reversesearch.ProviderASCII2DColor, Status: reversesearch.ProviderStatusSuccess, ResultCount: 1},
		{Name: reversesearch.ProviderASCII2DBOVW, Status: reversesearch.ProviderStatusSuccess, ResultCount: 1},
	}, response.Providers)
	require.Equal(t, []reversesearch.ProviderError{{
		Provider: reversesearch.ProviderSauceNAO, Code: reversesearch.CodeMissingCredential,
		Message: "SauceNAO API key is required",
	}}, response.ProviderErrors)
	require.True(t, response.Partial)
	require.Equal(t, []int64{100, 200}, []int64{response.Results[0].Pixiv.ID, response.Results[1].Pixiv.ID})
	require.Equal(t, []reversesearch.Provider{
		reversesearch.ProviderASCII2DColor, reversesearch.ProviderASCII2DBOVW,
	}, []reversesearch.Provider{
		response.Results[0].Evidence[0].Provider, response.Results[1].Evidence[0].Provider,
	})
}

func TestAggregatorAllUsesProviderRankOrderAndMergesCanonicalEvidence(t *testing.T) {
	sauce := providerClientStub{search: func(context.Context, *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
		return reversesearch.ProviderResponse{Provider: reversesearch.ProviderSauceNAO, Matches: []reversesearch.Match{
			{Rank: 2, Similarity: 99, IndexName: "sauce-second", ArtworkID: 42, Title: "second evidence"},
			{Rank: 1, Similarity: 20, IndexName: "sauce-first", Title: "first evidence", ExternalURLs: []string{"https://www.pixiv.net/artworks/42"}},
		}}, nil
	}}
	ascii := ascii2DClientStub{upload: func(context.Context, *reversesearch.Snapshot) (reversesearch.ASCII2DSession, error) {
		return ascii2DSessionFunc(func(_ context.Context, provider reversesearch.Provider) (reversesearch.ProviderResponse, error) {
			switch provider {
			case reversesearch.ProviderASCII2DColor:
				return reversesearch.ProviderResponse{Provider: provider, Matches: []reversesearch.Match{{
					Rank: 1, Similarity: 0, IndexName: "ascii-color", Title: "color evidence",
					ExternalURLs: []string{"https://www.pixiv.net/artworks/42"},
				}}}, nil
			case reversesearch.ProviderASCII2DBOVW:
				return reversesearch.ProviderResponse{Provider: provider, Matches: []reversesearch.Match{{
					Rank: 1, IndexName: "ascii-bovw", Title: "user evidence",
					ExternalURLs: []string{"https://www.pixiv.net/users/7"},
				}}}, nil
			default:
				return reversesearch.ProviderResponse{}, errors.New("unexpected provider")
			}
		}), nil
	}}
	searcher := newAggregateFacade(t, reversesearch.AggregatorDependencies{SauceNAO: sauce, ASCII2D: ascii})

	response, err := searcher.Search(context.Background(), reversesearch.Request{
		Source: fixtureImagePath(t), Provider: reversesearch.ProviderAll, PixivOnly: true,
	})
	require.NoError(t, err)
	require.False(t, response.Partial)
	require.Equal(t, []reversesearch.Provider{
		reversesearch.ProviderSauceNAO, reversesearch.ProviderASCII2DColor, reversesearch.ProviderASCII2DBOVW,
	}, []reversesearch.Provider{response.Providers[0].Name, response.Providers[1].Name, response.Providers[2].Name})
	require.Len(t, response.Results, 2)
	require.Equal(t, &reversesearch.PixivRef{Type: reversesearch.PixivRefArtwork, ID: 42}, response.Results[0].Pixiv)
	require.Equal(t, "first evidence", response.Results[0].Title)
	require.Equal(t, []reversesearch.Provider{
		reversesearch.ProviderSauceNAO, reversesearch.ProviderSauceNAO, reversesearch.ProviderASCII2DColor,
	}, []reversesearch.Provider{
		response.Results[0].Evidence[0].Provider,
		response.Results[0].Evidence[1].Provider,
		response.Results[0].Evidence[2].Provider,
	})
	require.Equal(t, []int{1, 2, 1}, []int{
		response.Results[0].Evidence[0].Rank,
		response.Results[0].Evidence[1].Rank,
		response.Results[0].Evidence[2].Rank,
	})
	require.Equal(t, []float64{20, 99, 0}, []float64{
		response.Results[0].Evidence[0].Similarity,
		response.Results[0].Evidence[1].Similarity,
		response.Results[0].Evidence[2].Similarity,
	}, "跨 provider similarity 只保留证据，不参与重新排序")
	require.Equal(t, &reversesearch.PixivRef{Type: reversesearch.PixivRefUser, ID: 7}, response.Results[1].Pixiv)
}

func TestAggregatorPixivOnlyUsesExplicitOrStrictPixivIdentityWithoutGuessing(t *testing.T) {
	matches := []reversesearch.Match{
		{Rank: 1, ArtworkID: 1, UserID: 99, Title: "explicit artwork wins"},
		{Rank: 2, UserID: 2, Title: "explicit user"},
		{Rank: 3, ExternalURLs: []string{"HTTPS://WWW.PIXIV.NET/artworks/3"}, Title: "case-insensitive origin"},
		{Rank: 4, ExternalURLs: []string{"https://www.pixiv.net/users/4"}, Title: "strict user URL"},
		{Rank: 5, ExternalURLs: []string{"https://www.pixiv.net/users/5", "https://www.pixiv.net/artworks/6"}, Title: "artwork URL wins"},
		{Rank: 6, UserID: 77, ExternalURLs: []string{"https://www.pixiv.net/artworks/12"}, Title: "artwork URL beats author ID"},
		{Rank: 7, ExternalURLs: []string{"https://www.pixiv.net.evil/artworks/7"}, Title: "lookalike host"},
		{Rank: 8, ExternalURLs: []string{"https://www.pixiv.net/artworks/7/extra"}, Title: "extra path"},
		{Rank: 9, ExternalURLs: []string{"https://www.pixiv.net/artworks/8?private=value"}, Title: "query"},
		{Rank: 10, ExternalURLs: []string{"http://www.pixiv.net/artworks/9"}, Title: "insecure scheme"},
		{Rank: 11, ExternalURLs: []string{"https://user@www.pixiv.net/artworks/10"}, Title: "userinfo"},
		{Rank: 12, Title: "Pixiv artwork 11", Author: "user 11"},
	}
	sauce := providerClientStub{search: func(context.Context, *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
		return reversesearch.ProviderResponse{Provider: reversesearch.ProviderSauceNAO, Matches: matches}, nil
	}}
	searcher := newAggregateFacade(t, reversesearch.AggregatorDependencies{SauceNAO: sauce})

	filtered, err := searcher.Search(context.Background(), reversesearch.Request{
		Source: fixtureImagePath(t), Provider: reversesearch.ProviderSauceNAO, PixivOnly: true,
	})
	require.NoError(t, err)
	require.Equal(t, []*reversesearch.PixivRef{
		{Type: reversesearch.PixivRefArtwork, ID: 1},
		{Type: reversesearch.PixivRefUser, ID: 2},
		{Type: reversesearch.PixivRefArtwork, ID: 3},
		{Type: reversesearch.PixivRefUser, ID: 4},
		{Type: reversesearch.PixivRefArtwork, ID: 6},
		{Type: reversesearch.PixivRefArtwork, ID: 12},
	}, resultRefs(filtered.Results))
	require.Equal(t, len(matches), filtered.Providers[0].ResultCount, "provider count 不受 pixiv-only 展示过滤影响")

	unfiltered, err := searcher.Search(context.Background(), reversesearch.Request{
		Source: fixtureImagePath(t), Provider: reversesearch.ProviderSauceNAO, PixivOnly: false,
	})
	require.NoError(t, err)
	require.Len(t, unfiltered.Results, len(matches))
	require.Nil(t, unfiltered.Results[len(unfiltered.Results)-1].Pixiv)
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

func resultRefs(results []reversesearch.Result) []*reversesearch.PixivRef {
	refs := make([]*reversesearch.PixivRef, 0, len(results))
	for _, result := range results {
		refs = append(refs, result.Pixiv)
	}
	return refs
}

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
