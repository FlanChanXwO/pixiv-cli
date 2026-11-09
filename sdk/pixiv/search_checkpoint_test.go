package pixiv_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestSearchContinuationEchoesDefaults(t *testing.T) {
	for _, feed := range []feedEndpoint{
		{"search_artworks", api.PathSearchIllust, searchEndpoints[0].body, url.Values{"word": {"word"}, "search_target": {"partial_match_for_tags"}, "sort": {"date_desc"}, "search_ai_type": {"0"}}, url.Values{"offset": {"30"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
			page, err := c.SearchArtworks(ctx, "word", pixiv.ArtworkSearchOptions{Cursor: token})
			return page.Cursor, err
		}},
		{"search_novels", api.PathSearchNovel, searchEndpoints[1].body, url.Values{"word": {"word"}, "search_target": {"partial_match_for_tags"}, "sort": {"date_desc"}}, url.Values{"offset": {"30"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
			page, err := c.SearchNovels(ctx, "word", pixiv.NovelSearchOptions{Cursor: token})
			return page.Cursor, err
		}},
	} {
		t.Run(feed.name, func(t *testing.T) { testFeedContinuation(t, feed) })
	}
}

func TestArtworkSearchCheckpoint(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Empty(t, r.URL.Query().Get("consumed"))
		if calls <= 2 {
			require.Equal(t, "20", r.URL.Query().Get(api.ParamOffset))
			fmt.Fprint(w, searchArtworkBatch)
		} else {
			require.Equal(t, "50", r.URL.Query().Get(api.ParamOffset))
			fmt.Fprint(w, `{"illusts":[]}`)
		}
	}))
	defer server.Close()
	value := pixiv.NewCredentials("fixture-refresh")
	value.SetTokens("fixture-access", "fixture-refresh")
	value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
	store, err := pixiv.NewMemoryCredentials(value)
	require.NoError(t, err)
	client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
	require.NoError(t, err)
	options := pixiv.ArtworkSearchOptions{Offset: 20, AIMode: pixiv.SearchAIModeOnly, CursorContext: "local"}
	token, err := client.CheckpointSearchArtworks(t.Context(), "word", options, 1)
	require.NoError(t, err)
	options.Cursor, err = sdk.ParseCursor(token.String())
	require.NoError(t, err)
	remaining, err := client.SearchArtworks(t.Context(), "word", options)
	require.NoError(t, err)
	require.Len(t, remaining.Items, 1)
	require.EqualValues(t, 24, remaining.Items[0].ID)
	options.Cursor, err = client.CheckpointSearchArtworks(t.Context(), "word", options, 1)
	require.NoError(t, err)
	exhausted, err := client.SearchArtworks(t.Context(), "word", options)
	require.NoError(t, err)
	require.Empty(t, exhausted.Items)
	require.False(t, exhausted.Cursor.IsZero())
	_, err = exhausted.Next(t.Context())
	require.NoError(t, err)
	require.Equal(t, 3, calls)
}

func TestArtworkSearchCursorBinding(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, searchArtworkBatch) }))
	defer server.Close()
	value := pixiv.NewCredentials("fixture-refresh")
	value.SetTokens("fixture-access", "fixture-refresh")
	value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
	store, err := pixiv.NewMemoryCredentials(value)
	require.NoError(t, err)
	client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
	require.NoError(t, err)
	options := pixiv.ArtworkSearchOptions{AIMode: pixiv.SearchAIModeOnly, CursorContext: "local"}
	token, err := client.CheckpointSearchArtworks(t.Context(), "word", options, 1)
	require.NoError(t, err)
	for _, change := range []pixiv.ArtworkSearchOptions{{AIMode: pixiv.SearchAIModeAll, CursorContext: "local", Cursor: token}, {AIMode: pixiv.SearchAIModeOnly, CursorContext: "changed", Cursor: token}, {AIMode: pixiv.SearchAIModeOnly, CursorContext: "local", Offset: 10, Cursor: token}} {
		_, err = client.SearchArtworks(t.Context(), "word", change)
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	}
	options.Cursor = token
	_, err = client.SearchArtworks(t.Context(), "changed", options)
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	require.Zero(t, calls)
	options.Cursor, err = client.CheckpointSearchArtworks(t.Context(), "word", pixiv.ArtworkSearchOptions{AIMode: pixiv.SearchAIModeOnly, CursorContext: "local"}, 3)
	require.NoError(t, err)
	_, err = client.SearchArtworks(t.Context(), "word", options)
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	options.Cursor, err = client.CheckpointSearchArtworks(t.Context(), "word", options, int(^uint(0)>>1))
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = client.CheckpointSearchArtworks(t.Context(), "word", options, 0)
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

func TestSearchContinuationValidation(t *testing.T) {
	for _, next := range []string{
		`https://evil.example/v1/search/illust?offset=2`, `https://app-api.pixiv.net/v1/search/illust?offset=0`, `https://app-api.pixiv.net/v1/search/illust?offset=2&consumed=1`,
		`https://app-api.pixiv.net/v1/search/illust?offset=2&unexpected=1`,
	} {
		t.Run(next, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"illusts":[],"next_url":%q}`, next) }))
			defer server.Close()
			value := pixiv.NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			_, err = client.SearchArtworks(t.Context(), "word", pixiv.ArtworkSearchOptions{})
			require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
		})
	}
}
