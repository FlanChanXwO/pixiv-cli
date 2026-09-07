package search_illust

import (
	"context"
	"fmt"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	product "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

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
