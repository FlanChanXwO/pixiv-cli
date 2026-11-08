package pixiv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/stretchr/testify/require"
)

var bookmarkEndpoints = []endpointContract{
	{"detail", api.PathIllustBookmarkDetail, `{"bookmark_detail":{"is_bookmarked":true,"restrict":"private","tags":[{"name":"kept","is_registered":true},{"name":"dropped"}]}}`, func(t *testing.T, ctx context.Context, c *Client) error {
		v, e := c.Artwork(23).Bookmark(ctx)
		if e == nil {
			require.Equal(t, RestrictPrivate, v.Restrict)
			require.Equal(t, []string{"kept"}, v.Tags)
		}
		return e
	}},
	{"artworks", api.PathUserBookmarksIllust, `{"illusts":[]}`, func(t *testing.T, ctx context.Context, c *Client) error {
		_, e := c.User(11).ArtworkBookmarks(ctx, ArtworkBookmarksOptions{Tag: "kept"})
		return e
	}},
	{"tags", api.PathUserBookmarkTags, `{"bookmark_tags":[{"name":"kept","count":2}]}`, func(t *testing.T, ctx context.Context, c *Client) error {
		v, e := c.User(11).ArtworkBookmarkTags(ctx, ArtworkBookmarkTagsOptions{})
		if e == nil {
			require.Equal(t, []BookmarkTag{{Name: "kept", Count: 2}}, v.Items)
		}
		return e
	}},
}

func TestBookmarkReadContracts(t *testing.T) { testEndpointContracts(t, bookmarkEndpoints) }

func TestEmptyBookmarkStates(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
	}{{`{}`, 404}, {`{}`, 200}, {`{"bookmark_detail":{"is_bookmarked":false,"tags":[{"name":"ignored","is_registered":true}]}}`, 200}} {
		t.Run(fmt.Sprint(tc.status, tc.body), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			value := NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := New(store, Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			detail, err := client.Artwork(23).Bookmark(t.Context())
			require.NoError(t, err)
			require.Equal(t, ArtworkBookmarkDetail{Tags: []string{}}, detail)
		})
	}
}

func TestBookmarkWrites(t *testing.T) {
	for _, mode := range []string{"normal", "upstream", "malformed", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.NoError(t, r.ParseForm())
				require.Equal(t, "23", r.Form.Get(api.ParamIllustID))
				if r.URL.Path == api.PathBookmarkAdd {
					require.Equal(t, "public", r.Form.Get(api.ParamRestrict))
					require.Equal(t, []string{"a", "b"}, r.Form[api.ParamTags])
				}
				if mode == "upstream" {
					w.WriteHeader(403)
				}
				if mode == "malformed" {
					w.Header().Set("Content-Length", "100")
					fmt.Fprint(w, "broken")
				} else {
					fmt.Fprint(w, `{}`)
				}
			}))
			defer server.Close()
			value := NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := New(store, Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			assertFollowResult(t, mode, client.Artwork(23).AddBookmark(ctx, BookmarkOptions{Tags: []string{"a", "b"}}), client.Artwork(23).RemoveBookmark(ctx), calls)
		})
	}
}

func TestParsedReferences(t *testing.T) {
	var art Artwork
	require.NoError(t, json.Unmarshal([]byte(`{"id":23}`), &art))
	_, err := art.Bookmark(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(art.AddBookmark(t.Context(), BookmarkOptions{})))

	var bookmarks UserBookmarksRef
	require.NoError(t, json.Unmarshal([]byte(`{"id":11}`), &bookmarks))
	_, err = bookmarks.Get(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	client := &Client{}
	for _, tc := range []struct{ raw, want string }{
		{"https://www.pixiv.net/en/artworks/23?tracking=1", "https://www.pixiv.net/artworks/23"},
		{"https://pixiv.net/users/11/artworks", "https://www.pixiv.net/users/11"},
		{"https://pixiv.net/zh-CN/users/11/bookmarks/artworks", "https://www.pixiv.net/users/11/bookmarks/artworks"},
	} {
		ref, err := client.ParseURL(tc.raw)
		require.NoError(t, err)
		canonical, err := ref.CanonicalURL()
		require.NoError(t, err)
		require.Equal(t, tc.want, canonical)
		switch r := ref.(type) {
		case *ArtworkRef:
			require.Same(t, client, r.client)
		case *UserRef:
			require.Same(t, client, r.client)
		case *UserBookmarksRef:
			require.Same(t, client, r.client)
		default:
			t.Fatalf("unexpected reference %T", ref)
		}
	}
	for _, raw := range []string{"23", "http://pixiv.net/artworks/23", "https://pixiv.net.evil/artworks/23", "https://x@pixiv.net/artworks/23", "https://pixiv.net:443/artworks/23", "https://pixiv.net/artworks/0", "https://pixiv.net/novel/show.php?id=23", "https://pixiv.net/user/11/series/23"} {
		_, err := client.ParseURL(raw)
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	}
}

func TestBookmarkContinuation(t *testing.T) {
	for _, path := range []string{api.PathUserBookmarksIllust, api.PathUserBookmarkTags} {
		t.Run(path, func(t *testing.T) {
			calls := 0
			key := api.ParamOffset
			if path == api.PathUserBookmarksIllust {
				key = api.ParamMaxBookmarkID
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, path, r.URL.Path)
				require.Equal(t, "", r.URL.Query().Get(api.ParamRestrict))
				require.Equal(t, "11", r.URL.Query().Get(api.ParamUserID))
				bodyKey := "bookmark_tags"
				if path == api.PathUserBookmarksIllust {
					bodyKey = "illusts"
					require.Equal(t, "kept", r.URL.Query().Get(api.ParamTag))
				}
				if calls == 1 {
					fmt.Fprintf(w, `{"%s":[],"next_url":"https://app-api.pixiv.net%s?%s=30&restrict=&user_id=11"}`, bodyKey, path, key)
				} else {
					require.Equal(t, "30", r.URL.Query().Get(key))
					fmt.Fprintf(w, `{"%s":[]}`, bodyKey)
				}
			}))
			defer server.Close()
			value := NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := New(store, Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			assertBookmarkNext(t, client, path)
			require.Equal(t, 2, calls)
		})
	}
}

func assertBookmarkNext(t *testing.T, client *Client, path string) {
	t.Helper()

	if path == api.PathUserBookmarksIllust {
		page, err := client.User(11).ArtworkBookmarks(t.Context(), ArtworkBookmarksOptions{Tag: "kept"})
		require.NoError(t, err)
		_, err = client.User(11).ArtworkBookmarks(t.Context(), ArtworkBookmarksOptions{Tag: "changed", Cursor: page.Cursor})
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
		_, err = page.Next(t.Context())
		require.NoError(t, err)
	} else {
		page, err := client.User(11).ArtworkBookmarkTags(t.Context(), ArtworkBookmarkTagsOptions{})
		require.NoError(t, err)
		_, err = page.Next(t.Context())
		require.NoError(t, err)
	}
}
