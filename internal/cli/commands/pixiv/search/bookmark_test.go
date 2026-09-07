package search

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
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

func (c artworkClient) CheckpointSearchArtworks(request product.SearchArtworksRequest, consumed int) (sdk.Cursor, error) {
	return sdk.NewCursor("test", "checkpoint", 1, "hash", []byte("batch remainder"))
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
	require.False(t, result.Page.Next.IsZero())
	require.NotEqual(t, next, result.Page.Next)
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

// continuationHTTP 只替代上游 HTTP，测试使用真实 SDK 和搜索适配路径。
type continuationHTTP func(*http.Request) (*http.Response, error)

func (f continuationHTTP) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSearchContinuationDoesNotLoseRemainder(t *testing.T) {
	for _, filtered := range []bool{false, true} {
		t.Run(fmt.Sprint(filtered), func(t *testing.T) {
			client, err := product.NewWith("token", product.Options{HTTPClient: &http.Client{Transport: continuationHTTP(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"illusts":[{"id":1,"total_bookmarks":12,"type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7}},{"id":2,"total_bookmarks":12,"type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7}},{"id":3,"total_bookmarks":12,"type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7}}]}`))}, nil
			})}})
			require.NoError(t, err)
			execute := func(ctx context.Context, fn func(context.Context, *product.Client) (bool, error)) error {
				_, err := fn(ctx, client)
				return err
			}
			request := artworkSearchRequest{Query: product.SearchArtworksRequest{Word: "test"}, Plan: pagination.PagePlan{Limit: 2}}
			if filtered {
				min := 10
				request.Query.BookmarkMin = &min
			}
			first, err := searchArtworks(context.Background(), execute, request)
			require.NoError(t, err)
			require.False(t, first.Page.Next.IsZero(), "last batch remainder needs a continuation")
			request.Query.Cursor = first.Page.Next
			if filtered {
				changed := request
				min := 11
				changed.Query.BookmarkMin = &min
				_, err := searchArtworks(context.Background(), execute, changed)
				require.Equal(t, sdk.InvalidCursor, sdk.ReasonOf(err))
			}
			second, err := searchArtworks(context.Background(), execute, request)
			require.NoError(t, err)
			var ids []int64
			for _, item := range append(first.Page.Items, second.Page.Items...) {
				ids = append(ids, item.ID)
			}
			require.Equal(t, []int64{1, 2, 3}, ids)
			require.True(t, second.Page.Next.IsZero())
		})
	}
}

func TestSearchContinuationReplayDiscardsFailedAttempt(t *testing.T) {
	makeClient := func(failing bool) *product.Client {
		client, err := product.NewWith("token", product.Options{HTTPClient: &http.Client{Transport: continuationHTTP(func(r *http.Request) (*http.Response, error) {
			if failing && r.URL.Query().Get("offset") != "" {
				return nil, fmt.Errorf("fixture read failure")
			}
			ids := []int{2, 3}
			next := ""
			if failing {
				ids = []int{1}
				next = `,"next_url":"https://app-api.pixiv.net/v1/search/illust?offset=30"`
			}
			var records []string
			for _, id := range ids {
				records = append(records, fmt.Sprintf(`{"id":%d,"total_bookmarks":12,"type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7}}`, id))
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"illusts":[` + strings.Join(records, ",") + `]` + next + `}`))}, nil
		})}})
		require.NoError(t, err)
		return client
	}
	first, second := makeClient(true), makeClient(false)
	execute := func(ctx context.Context, fn func(context.Context, *product.Client) (bool, error)) error {
		committed, err := fn(ctx, first)
		require.Error(t, err)
		require.False(t, committed)
		_, err = fn(ctx, second)
		return err
	}
	min := 10
	result, err := searchArtworks(context.Background(), execute, artworkSearchRequest{Query: product.SearchArtworksRequest{Word: "test", BookmarkMin: &min}, Plan: pagination.PagePlan{Limit: 2}})
	require.NoError(t, err)
	require.Len(t, result.Page.Items, 2)
	require.EqualValues(t, 2, result.Page.Items[0].ID)
	require.EqualValues(t, 3, result.Page.Items[1].ID)
	require.True(t, result.Page.Next.IsZero())
}
