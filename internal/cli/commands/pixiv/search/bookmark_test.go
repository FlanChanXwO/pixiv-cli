package search

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/traversal"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	product "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

type artworkClient struct {
	search func(context.Context, product.SearchArtworksRequest) (sdk.Page[product.Artwork], error)
}

func (c artworkClient) SearchArtworks(ctx context.Context, request product.SearchArtworksRequest) (sdk.Page[product.Artwork], error) {
	return c.search(ctx, request)
}

func oneClientOperation[C any](client C) traversal.Execute[C] {
	return func(ctx context.Context, attempt func(context.Context, C) (bool, error)) error {
		_, err := attempt(ctx, client)
		return err
	}
}

func TestSearchArtworksLocallyFiltersCandidatesAndReportsCompleteness(t *testing.T) {
	first := testSearchCursor(t, "first")
	start := testSearchCursor(t, "start")
	var calls []product.SearchArtworksRequest
	client := artworkClient{search: func(_ context.Context, request product.SearchArtworksRequest) (sdk.Page[product.Artwork], error) {
		calls = append(calls, request)
		switch len(calls) {
		case 1:
			return sdk.Page[product.Artwork]{Items: []product.Artwork{{ID: 1, TotalBookmarks: 5}, {ID: 2, TotalBookmarks: 12}}, Next: first}, nil
		case 2:
			return sdk.Page[product.Artwork]{Items: []product.Artwork{{ID: 3, TotalBookmarks: 20}, {ID: 4, TotalBookmarks: 21}}}, nil
		default:
			t.Fatalf("unexpected search call %d", len(calls))
			return sdk.Page[product.Artwork]{}, nil
		}
	}}

	result, err := searchArtworks(context.Background(), oneClientOperation(client), artworkSearchRequest{
		Query: product.SearchArtworksRequest{
			Word: "cat", Target: product.SearchTargetKeyword, Sort: product.SortModeDateDesc,
			BookmarkMin: intSearchPointer(10), BookmarkMax: intSearchPointer(20), Cursor: start,
		},
		Plan: pagination.PagePlan{Limit: 2}, Strategy: bookmarkFilterStrategyAuto,
	})

	require.NoError(t, err)
	require.Equal(t, []int64{2, 3}, searchArtworkIDs(result.Page.Items))
	require.True(t, result.Page.Next.IsZero())
	require.NotNil(t, result.Filter)
	require.Equal(t, bookmarkMembershipUnknown, result.Filter.Membership)
	require.Equal(t, bookmarkFilterStrategyLocal, result.Filter.Strategy)
	require.Equal(t, bookmarkFilterCompletenessCompleteForSource, result.Filter.Completeness)
	require.Len(t, calls, 2)
	require.Equal(t, start, calls[0].Cursor)
	require.Equal(t, first, calls[1].Cursor)
	require.Nil(t, calls[0].BookmarkMin)
	require.Nil(t, calls[0].BookmarkMax)
}

func TestSearchArtworksBestEffortKeepsCandidateBoundsAndReportsPartialLimit(t *testing.T) {
	next := testSearchCursor(t, "partial")
	var request product.SearchArtworksRequest
	client := artworkClient{search: func(_ context.Context, value product.SearchArtworksRequest) (sdk.Page[product.Artwork], error) {
		request = value
		return sdk.Page[product.Artwork]{Items: []product.Artwork{{ID: 1, TotalBookmarks: 11}, {ID: 2, TotalBookmarks: 12}}, Next: next}, nil
	}}

	result, err := searchArtworks(context.Background(), oneClientOperation(client), artworkSearchRequest{
		Query: product.SearchArtworksRequest{BookmarkMin: intSearchPointer(10), BookmarkMax: intSearchPointer(20)},
		Plan:  pagination.PagePlan{Limit: 1}, Strategy: bookmarkFilterStrategyBestEffort,
	})

	require.NoError(t, err)
	require.Equal(t, 10, *request.BookmarkMin)
	require.Equal(t, 20, *request.BookmarkMax)
	require.Equal(t, next, result.Page.Next)
	require.Equal(t, bookmarkFilterStrategyBestEffort, result.Filter.Strategy)
	require.Equal(t, bookmarkFilterCompletenessPartial, result.Filter.Completeness)
}

func TestSearchArtworksGatesUnavailableServerStrategiesBeforeOperation(t *testing.T) {
	called := false
	operation := func(context.Context, func(context.Context, artworkClient) (bool, error)) error {
		called = true
		return nil
	}
	request := artworkSearchRequest{
		Query:    product.SearchArtworksRequest{BookmarkMin: intSearchPointer(10)},
		Strategy: bookmarkFilterStrategyServer,
	}

	_, err := searchArtworks(context.Background(), operation, request)
	require.Equal(t, sdk.UpstreamUnavailable, sdk.ReasonOf(err))
	require.False(t, called)

	request.Membership = bookmarkMembershipNonPremium
	_, err = searchArtworks(context.Background(), operation, request)
	require.Equal(t, sdk.Forbidden, sdk.ReasonOf(err))
	require.False(t, called)
}

func TestSearchArtworksPreservesUpstreamErrorAndRejectsNegativeBookmarkCount(t *testing.T) {
	upstream := errors.New("upstream sentinel")
	client := artworkClient{search: func(context.Context, product.SearchArtworksRequest) (sdk.Page[product.Artwork], error) {
		return sdk.Page[product.Artwork]{}, upstream
	}}
	_, err := searchArtworks(context.Background(), oneClientOperation(client), artworkSearchRequest{Query: product.SearchArtworksRequest{BookmarkMin: intSearchPointer(1)}})
	require.ErrorIs(t, err, upstream)

	invalid := artworkClient{search: func(context.Context, product.SearchArtworksRequest) (sdk.Page[product.Artwork], error) {
		return sdk.Page[product.Artwork]{Items: []product.Artwork{{ID: 7, TotalBookmarks: -1}}}, nil
	}}
	_, err = searchArtworks(context.Background(), oneClientOperation(invalid), artworkSearchRequest{Query: product.SearchArtworksRequest{BookmarkMin: intSearchPointer(1)}})
	require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
}

func TestSearchArtworksReportsUnconfiguredOperation(t *testing.T) {
	_, err := searchArtworks[*product.Client](context.Background(), nil, artworkSearchRequest{
		Query: product.SearchArtworksRequest{BookmarkMin: intSearchPointer(1)},
	})
	require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
	require.NotContains(t, err.Error(), "refresh_token")
}

func TestSearchWorkflowPreservesMissingExecutorError(t *testing.T) {
	_, err := searchArtworks(context.Background(), func(context.Context, func(context.Context, artworkClient) (bool, error)) error {
		return nil
	}, artworkSearchRequest{
		Query: product.SearchArtworksRequest{BookmarkMin: intSearchPointer(1)},
	})
	_ = err

	_, err = searchArtworks[artworkClient](context.Background(), nil, artworkSearchRequest{
		Query: product.SearchArtworksRequest{BookmarkMin: intSearchPointer(1)},
	})
	require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
	require.Contains(t, err.Error(), "sdk pooled operation is not configured")
}

func testSearchCursor(t *testing.T, payload string) sdk.Cursor {
	t.Helper()
	cursor, err := sdk.NewCursor("pixiv", "SearchArtworks", 1, "search-workflow", []byte(payload))
	require.NoError(t, err)
	return cursor
}

func intSearchPointer(value int) *int { return &value }

func searchArtworkIDs(items []product.Artwork) []int64 {
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}
