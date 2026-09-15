package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

type runtimeFixtureTransport func(*http.Request) (*http.Response, error)

func (f runtimeFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestCollectWithFromResetsLocalFilterOnSafeReplay(t *testing.T) {
	request := pixiv.SearchArtworksRequest{Word: "test", CursorContext: "mcp-local-filter"}
	seed := newRuntimeFixtureClient(t, func(request *http.Request) (string, error) {
		require.Empty(t, request.URL.Query().Get("offset"))
		return runtimeArtworkPage([]int{100}, "https://app-api.pixiv.net/v1/search/illust?offset=30"), nil
	})
	seedPage, err := seed.SearchArtworks(context.Background(), request)
	require.NoError(t, err)
	require.False(t, seedPage.Next.IsZero())

	firstAttempt := newRuntimeFixtureClient(t, func(request *http.Request) (string, error) {
		switch request.URL.Query().Get("offset") {
		case "30":
			return runtimeArtworkPage([]int{200}, "https://app-api.pixiv.net/v1/search/illust?offset=60"), nil
		case "60":
			return "", fmt.Errorf("fixture read failure")
		default:
			return "", fmt.Errorf("unexpected first-attempt offset %q", request.URL.Query().Get("offset"))
		}
	})
	secondAttempt := newRuntimeFixtureClient(t, func(request *http.Request) (string, error) {
		if got := request.URL.Query().Get("offset"); got != "30" {
			return "", fmt.Errorf("unexpected replay offset %q", got)
		}
		return runtimeArtworkPage([]int{200, 200, 201}, ""), nil
	})

	minViews := 10
	ctx, err := filters.WithIllustFilter(context.Background(), &filters.IllustFilter{MinViews: &minViews})
	require.NoError(t, err)
	app := NewApp(nil, nil, SDKPorts{Execute: func(ctx context.Context, _ Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		committed, err := attempt(ctx, firstAttempt)
		require.False(t, committed)
		require.Error(t, err)
		committed, err = attempt(ctx, secondAttempt)
		require.False(t, committed)
		require.NoError(t, err)
		return nil
	}}, Account{UserID: 42})

	items, hasMore, err := collectWithFrom(ctx, app, ListPlan{Limit: 0}, seedPage.Next, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		continued := request
		continued.Cursor = cursor
		page, err := client.SearchArtworks(ctx, continued)
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return page.Items, page.Next, nil
	})
	require.NoError(t, err)
	require.False(t, hasMore)
	require.Len(t, items, 1)
	require.EqualValues(t, 200, items[0].ID)
}

func newRuntimeFixtureClient(t *testing.T, handler func(*http.Request) (string, error)) *pixiv.Client {
	t.Helper()
	client, _, err := pixiv.OpenWith(context.Background(), "fixture-refresh-token", pixiv.Options{HTTPClient: &http.Client{Transport: runtimeFixtureTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "oauth.secure.pixiv.net" {
			return runtimeFixtureResponse(`{"access_token":"fixture-access-token","refresh_token":"fixture-refresh-token","expires_in":3600,"user":{"id":42,"name":"fixture"}}`), nil
		}
		body, err := handler(request)
		if err != nil {
			return nil, err
		}
		return runtimeFixtureResponse(body), nil
	})}})
	require.NoError(t, err)
	return client
}

func runtimeFixtureResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func runtimeArtworkPage(ids []int, nextURL string) string {
	items := make([]string, 0, len(ids))
	for _, id := range ids {
		totalViews := 42
		if id == 201 {
			totalViews = 1
		}
		items = append(items, fmt.Sprintf(`{"id":%d,"title":"fixture","type":"illust","total_view":%d,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"},"tags":[]}`, id, totalViews))
	}
	next := ""
	if nextURL != "" {
		next = `,"next_url":"` + nextURL + `"`
	}
	return `{"illusts":[` + strings.Join(items, ",") + `]` + next + `}`
}
