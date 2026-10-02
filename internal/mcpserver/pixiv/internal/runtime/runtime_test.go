package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	app := NewApp(SDKPorts{Execute: func(ctx context.Context, _ Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
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

func TestToolPinsAccountAcrossSDKOperations(t *testing.T) {
	var selected int64 = 42
	var resolutions int
	var used []int64
	app := NewApp(SDKPorts{
		ResolveAccount: func(ctx context.Context, account Account) (Account, error) {
			resolutions++
			account.UserID = selected
			return account, nil
		},
		Execute: func(ctx context.Context, account Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			used = append(used, account.UserID)
			_, err := attempt(ctx, nil)
			return err
		},
		OpenLease: func(ctx context.Context, account Account) (*lifecycle.Lease[*pixiv.Client], error) {
			used = append(used, account.UserID)
			return lifecycle.NewLease(&pixiv.Client{}, func() error { return nil }), nil
		},
	}, Account{})
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	AddTool(app, server, &mcp.Tool{Name: "snapshot"}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		_, err := Read(app, ctx, func(context.Context, *pixiv.Client) (int, error) { return 1, nil })
		if err != nil {
			return nil, struct{}{}, err
		}
		selected = 73
		if err := Write(app, ctx, func(context.Context, *pixiv.Client) error { return nil }); err != nil {
			return nil, struct{}{}, err
		}
		if err := app.Execute()(ctx, func(context.Context, *pixiv.Client) (bool, error) { return false, nil }); err != nil {
			return nil, struct{}{}, err
		}
		lease, err := app.OpenClient(ctx)
		if err != nil {
			return nil, struct{}{}, err
		}
		return nil, struct{}{}, lease.Close()
	})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()
	for range 2 {
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "snapshot", Arguments: map[string]any{}})
		require.NoError(t, err)
		require.False(t, result.IsError)
	}
	require.Equal(t, []int64{42, 42, 42, 42, 73, 73, 73, 73}, used)
	require.Equal(t, 2, resolutions)
}

func TestAccountSelectionFailureNeverOpensOrExecutesSDK(t *testing.T) {
	selectionErr := errors.New("selection_required")
	app := NewApp(SDKPorts{
		ResolveAccount: func(context.Context, Account) (Account, error) { return Account{}, selectionErr },
		Execute: func(context.Context, Account, func(context.Context, *pixiv.Client) (bool, error)) error {
			t.Error("executed without account")
			return nil
		},
		Open: func(Account) (*pixiv.Client, error) { t.Error("opened without account"); return &pixiv.Client{}, nil },
	}, Account{UserID: 99})
	_, err := Read(app, t.Context(), func(context.Context, *pixiv.Client) (int, error) { t.Error("read invoked"); return 0, nil })
	require.ErrorIs(t, err, selectionErr)
	require.ErrorIs(t, Write(app, t.Context(), func(context.Context, *pixiv.Client) error { t.Error("write invoked"); return nil }), selectionErr)
	require.ErrorIs(t, app.Execute()(t.Context(), func(context.Context, *pixiv.Client) (bool, error) { t.Error("attempt invoked"); return false, nil }), selectionErr)
	lease, err := app.OpenClient(t.Context())
	require.Nil(t, lease)
	require.ErrorIs(t, err, selectionErr)
}
