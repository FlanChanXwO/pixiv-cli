package pixiv_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

var novelBookmarkEndpoints = []endpointContract{
	{"novel_bookmark", api.PathNovelBookmarkDetail, `{"bookmark_detail":{"is_bookmarked":true,"restrict":"private","tags":[{"name":"kept","is_registered":true},{"name":"ignored"}]}}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		v, e := c.Novel(23).Bookmark(ctx)
		if e == nil {
			require.Equal(t, pixiv.RestrictPrivate, v.Restrict)
			require.Equal(t, []string{"kept"}, v.Tags)
		}
		return e
	}},
	{"novel_bookmarks", api.PathUserNovelBookmarks, `{"novels":[` + novelMetadata + `]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		v, e := c.User(11).NovelBookmarks(ctx, pixiv.NovelBookmarksOptions{Tag: "kept"})
		if e == nil {
			require.Len(t, v.Items, 1)
			require.Equal(t, "fixture", v.Items[0].Title)
		}
		return e
	}},
	{"novel_tags", api.PathUserNovelBookmarkTags, `{"bookmark_tags":[{"name":"kept","count":2}]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		v, e := c.User(11).NovelBookmarkTags(ctx, pixiv.NovelBookmarkTagsOptions{})
		if e == nil {
			require.Equal(t, []pixiv.BookmarkTag{{Name: "kept", Count: 2}}, v.Items)
			require.True(t, v.Cursor.IsZero())
		}
		return e
	}},
}

func TestNovelBookmarkReadContracts(t *testing.T) { testEndpointContracts(t, novelBookmarkEndpoints) }

func TestNovelBookmarkWrites(t *testing.T) {
	for _, mode := range []string{"normal", "upstream", "malformed", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodPost, r.Method)
				require.NoError(t, r.ParseForm())
				require.Equal(t, "23", r.Form.Get(api.ParamNovelID))
				require.Equal(t, []string{api.PathNovelBookmarkAdd, api.PathNovelBookmarkDelete}[calls-1], r.URL.Path)
				if calls == 1 {
					require.Equal(t, "public", r.Form.Get(api.ParamRestrict))
					require.Equal(t, []string{"a", "b"}, r.Form[api.ParamTags])
				}
				if mode == "upstream" {
					w.WriteHeader(403)
				}
				if mode == "malformed" {
					w.Header().Set("Content-Length", "100")
				}
				fmt.Fprint(w, `{}`)
			}))
			defer server.Close()
			value := pixiv.NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			assertFollowResult(t, mode, client.Novel(23).AddBookmark(ctx, pixiv.BookmarkOptions{Tags: []string{"a", "b"}}), client.Novel(23).RemoveBookmark(ctx), calls)
		})
	}
}

func TestNovelBookmarkContinuation(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, api.PathUserNovelBookmarks, r.URL.Path)
		require.Equal(t, "11", r.URL.Query().Get(api.ParamUserID))
		require.Equal(t, "kept", r.URL.Query().Get(api.ParamTag))
		require.Equal(t, "private", r.URL.Query().Get(api.ParamRestrict))
		if calls == 1 {
			fmt.Fprint(w, `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?max_bookmark_id=7"}`)
		} else {
			require.Equal(t, "7", r.URL.Query().Get(api.ParamMaxBookmarkID))
			fmt.Fprint(w, `{"novels":[]}`)
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
	options := pixiv.NovelBookmarksOptions{Restrict: pixiv.RestrictPrivate, Tag: "kept"}
	page, err := client.User(11).NovelBookmarks(t.Context(), options)
	require.NoError(t, err)
	options.Cursor = page.Cursor
	options.Tag = "changed"
	_, err = client.User(11).NovelBookmarks(t.Context(), options)
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = client.User(11).NovelBookmarkTags(t.Context(), pixiv.NovelBookmarkTagsOptions{Cursor: page.Cursor})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	require.Equal(t, 1, calls)
	_, err = page.Next(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func TestNovelEmptyBookmark(t *testing.T) {
	for _, body := range []string{`{}`, `{"bookmark_detail":{"is_bookmarked":false}}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			value := pixiv.NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			result, err := client.Novel(23).Bookmark(t.Context())
			require.NoError(t, err)
			require.Equal(t, pixiv.NovelBookmarkDetail{Tags: []string{}}, result)
		})
	}
}
