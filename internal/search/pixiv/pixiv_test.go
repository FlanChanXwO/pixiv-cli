package pixiv_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	searchpixiv "github.com/FlanChanXwO/pixiv-cli/internal/search/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	product "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

type artworkClient struct {
	search func(context.Context, product.SearchArtworksRequest) (sdk.Page[product.Artwork], error)
}

func (c artworkClient) SearchArtworks(ctx context.Context, request product.SearchArtworksRequest) (sdk.Page[product.Artwork], error) {
	return c.search(ctx, request)
}

func oneClientOperation[C any](client C) searchpixiv.Operation[C] {
	return func(ctx context.Context, attempt func(context.Context, C) (bool, error)) error {
		_, err := attempt(ctx, client)
		return err
	}
}

func TestRunPooledPagedReadPreservesCommitBoundary(t *testing.T) {
	next := testCursor(t, "paged")
	var received []int64
	var cursors []sdk.Cursor

	err := searchpixiv.RunPooledPagedRead(context.Background(), oneClientOperation(struct{}{}), pagination.PagePlan{Limit: 1, Skip: 1}, nil,
		func(_ context.Context, _ struct{}, cursor sdk.Cursor) ([]product.Artwork, sdk.Cursor, error) {
			cursors = append(cursors, cursor)
			if cursor.IsZero() {
				return []product.Artwork{{ID: 1}}, next, nil
			}
			return []product.Artwork{{ID: 2}}, sdk.Cursor{}, nil
		}, func(items []product.Artwork) (bool, error) {
			for _, item := range items {
				received = append(received, item.ID)
			}
			return len(items) > 0, nil
		})

	require.NoError(t, err)
	require.Equal(t, []sdk.Cursor{{}, next}, cursors)
	require.Equal(t, []int64{2}, received)
}

func TestCollectPooledPagedReadKeepsLogicalMoreWithoutExposingCursor(t *testing.T) {
	next := testCursor(t, "collect")
	result, err := searchpixiv.CollectPooledPagedRead(context.Background(), oneClientOperation(struct{}{}), pagination.PagePlan{OneBatch: true}, func(_ context.Context, _ struct{}, cursor sdk.Cursor) ([]product.Artwork, sdk.Cursor, error) {
		if cursor.IsZero() {
			return []product.Artwork{{ID: 1}}, next, nil
		}
		return []product.Artwork{{ID: 2}}, sdk.Cursor{}, nil
	})

	require.NoError(t, err)
	require.Equal(t, []int64{1}, artworkIDs(result.Items))
	require.True(t, result.HasMore)
}

func TestSearchArtworksLocallyFiltersCandidatesAndReportsCompleteness(t *testing.T) {
	first := testCursor(t, "first")
	start := testCursor(t, "start")
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

	result, err := searchpixiv.SearchArtworks(context.Background(), oneClientOperation(client), searchpixiv.ArtworkSearchRequest{
		Query: product.SearchArtworksRequest{
			Word: "cat", Target: product.SearchTargetKeyword, Sort: product.SortModeDateDesc,
			BookmarkMin: intPointer(10), BookmarkMax: intPointer(20), Cursor: start,
		},
		Plan: pagination.PagePlan{Limit: 2}, Strategy: searchpixiv.BookmarkFilterStrategyAuto,
	})

	require.NoError(t, err)
	require.Equal(t, []int64{2, 3}, artworkIDs(result.Page.Items))
	require.True(t, result.Page.Next.IsZero())
	require.NotNil(t, result.Filter)
	require.Equal(t, searchpixiv.BookmarkMembershipUnknown, result.Filter.Membership)
	require.Equal(t, searchpixiv.BookmarkFilterStrategyLocal, result.Filter.Strategy)
	require.Equal(t, searchpixiv.BookmarkFilterCompletenessCompleteForSource, result.Filter.Completeness)
	require.Len(t, calls, 2)
	require.Equal(t, start, calls[0].Cursor)
	require.Equal(t, first, calls[1].Cursor)
	require.Nil(t, calls[0].BookmarkMin)
	require.Nil(t, calls[0].BookmarkMax)
}

func TestSearchArtworksBestEffortKeepsCandidateBoundsAndReportsPartialLimit(t *testing.T) {
	next := testCursor(t, "partial")
	var request product.SearchArtworksRequest
	client := artworkClient{search: func(_ context.Context, value product.SearchArtworksRequest) (sdk.Page[product.Artwork], error) {
		request = value
		return sdk.Page[product.Artwork]{Items: []product.Artwork{{ID: 1, TotalBookmarks: 11}, {ID: 2, TotalBookmarks: 12}}, Next: next}, nil
	}}

	result, err := searchpixiv.SearchArtworks(context.Background(), oneClientOperation(client), searchpixiv.ArtworkSearchRequest{
		Query: product.SearchArtworksRequest{BookmarkMin: intPointer(10), BookmarkMax: intPointer(20)},
		Plan:  pagination.PagePlan{Limit: 1}, Strategy: searchpixiv.BookmarkFilterStrategyBestEffort,
	})

	require.NoError(t, err)
	require.Equal(t, 10, *request.BookmarkMin)
	require.Equal(t, 20, *request.BookmarkMax)
	require.Equal(t, next, result.Page.Next)
	require.Equal(t, searchpixiv.BookmarkFilterStrategyBestEffort, result.Filter.Strategy)
	require.Equal(t, searchpixiv.BookmarkFilterCompletenessPartial, result.Filter.Completeness)
}

func TestSearchArtworksGatesUnavailableServerStrategiesBeforeOperation(t *testing.T) {
	called := false
	operation := func(context.Context, func(context.Context, artworkClient) (bool, error)) error {
		called = true
		return nil
	}
	request := searchpixiv.ArtworkSearchRequest{
		Query:    product.SearchArtworksRequest{BookmarkMin: intPointer(10)},
		Strategy: searchpixiv.BookmarkFilterStrategyServer,
	}

	_, err := searchpixiv.SearchArtworks(context.Background(), operation, request)
	require.Equal(t, sdk.UpstreamUnavailable, sdk.ReasonOf(err))
	require.False(t, called)

	request.Membership = searchpixiv.BookmarkMembershipNonPremium
	_, err = searchpixiv.SearchArtworks(context.Background(), operation, request)
	require.Equal(t, sdk.Forbidden, sdk.ReasonOf(err))
	require.False(t, called)
}

func TestSearchArtworksPreservesUpstreamErrorAndRejectsNegativeBookmarkCount(t *testing.T) {
	upstream := errors.New("upstream sentinel")
	client := artworkClient{search: func(context.Context, product.SearchArtworksRequest) (sdk.Page[product.Artwork], error) {
		return sdk.Page[product.Artwork]{}, upstream
	}}
	_, err := searchpixiv.SearchArtworks(context.Background(), oneClientOperation(client), searchpixiv.ArtworkSearchRequest{Query: product.SearchArtworksRequest{BookmarkMin: intPointer(1)}})
	require.ErrorIs(t, err, upstream)

	invalid := artworkClient{search: func(context.Context, product.SearchArtworksRequest) (sdk.Page[product.Artwork], error) {
		return sdk.Page[product.Artwork]{Items: []product.Artwork{{ID: 7, TotalBookmarks: -1}}}, nil
	}}
	_, err = searchpixiv.SearchArtworks(context.Background(), oneClientOperation(invalid), searchpixiv.ArtworkSearchRequest{Query: product.SearchArtworksRequest{BookmarkMin: intPointer(1)}})
	require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
}

func TestSearchArtworksReportsUnconfiguredOperation(t *testing.T) {
	_, err := searchpixiv.SearchArtworks[*pixiv.Client](context.Background(), nil, searchpixiv.ArtworkSearchRequest{
		Query: product.SearchArtworksRequest{BookmarkMin: intPointer(1)},
	})
	require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
	require.NotContains(t, err.Error(), "refresh_token")
}

func TestSearchWorkflowPreservesMissingPooledOperationError(t *testing.T) {
	_, err := searchpixiv.SearchArtworks(context.Background(), searchpixiv.Operation[artworkClient](nil), searchpixiv.ArtworkSearchRequest{
		Query: product.SearchArtworksRequest{BookmarkMin: intPointer(1)},
	})
	require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
	require.Contains(t, err.Error(), "sdk pooled operation is not configured")

	err = searchpixiv.RunPooledPagedRead(context.Background(), searchpixiv.Operation[struct{}](nil), pagination.PagePlan{}, nil,
		func(context.Context, struct{}, sdk.Cursor) ([]product.Artwork, sdk.Cursor, error) {
			return nil, sdk.Cursor{}, nil
		}, func([]product.Artwork) (bool, error) {
			return false, nil
		})
	require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
	require.Contains(t, err.Error(), "sdk pooled operation is not configured")
}

func testCursor(t *testing.T, payload string) sdk.Cursor {
	t.Helper()
	cursor, err := sdk.NewCursor("pixiv", "SearchArtworks", 1, "search-workflow", []byte(payload))
	require.NoError(t, err)
	return cursor
}

func intPointer(value int) *int { return &value }

func artworkIDs(items []product.Artwork) []int64 {
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}
