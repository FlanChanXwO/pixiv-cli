package fanbox_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func readPostPage(ctx context.Context, client *fanbox.Client, path string, token sdk.Cursor) (sdk.Page[*fanbox.Post], error) {
	switch path {
	case "/post.listCreator":
		return client.Creator(" artist ").Posts(ctx, fanbox.PostsRequest{Cursor: token})
	case "/post.listTagged":
		return client.Creator("artist").Posts(ctx, fanbox.PostsRequest{Tag: " art ", Cursor: token})
	case "/post.listHome":
		return client.Home(ctx, fanbox.FeedRequest{Cursor: token})
	default:
		return client.Supporting(ctx, fanbox.FeedRequest{Cursor: token})
	}
}

func servePostList(t *testing.T, path string, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	if r.URL.Path == "/" {
		fmt.Fprint(w, `<meta name="metadata" content='{"context":{"user":{"userId":21,"name":"fixture"}}}'>`)
		return
	}
	if r.URL.Path != path {
		t.Error("incorrect post list route")
	}
	query := r.URL.Query()
	if query.Get("offset") == "1" {
		fmt.Fprint(w, `{"body":{"posts":[]}}`)
		return
	}
	if path == "/post.listCreator" || path == "/post.listTagged" {
		if query.Get("creatorId") != "artist" {
			t.Error("missing creator")
		}
	}
	if path == "/post.listTagged" {
		if query.Get("tag") != "art" || query.Has("limit") {
			t.Error("incorrect tag query")
		}
	} else if query.Get("limit") != "10" {
		t.Error("missing limit")
	}
	key := "posts"
	if path == "/post.listHome" || path == "/post.listSupporting" {
		key = "items"
	}
	fmt.Fprintf(w, `{"body":{"%s":[{"id":"42","creatorId":"artist","title":"fixture","publishedDatetime":"2026-11-10T00:00:00Z","body":{"text":"list body"}}],"pageUrls":["https://api.fanbox.cc%s?offset=1"],"nextUrl":"https://foreign.invalid/"}}`, key, path)
}

func TestPostLists(t *testing.T) {
	for _, path := range []string{"/post.listCreator", "/post.listTagged", "/post.listHome", "/post.listSupporting"} {
		t.Run(path, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { servePostList(t, path, w, r) }))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			page, err := readPostPage(t.Context(), client, path, sdk.Cursor{})
			require.NoError(t, err)
			require.Len(t, page.Items, 1)
			require.Equal(t, "42", page.Items[0].ID)
			require.Equal(t, "list body", page.Items[0].Body.Text)
			token, err := sdk.ParseCursor(page.Cursor.String())
			require.NoError(t, err)
			resumed, err := readPostPage(t.Context(), client, path, token)
			require.NoError(t, err)
			require.NotNil(t, resumed.Items)
			require.Empty(t, resumed.Items)
			next, err := page.Next(t.Context())
			require.NoError(t, err)
			require.True(t, next.Cursor.IsZero())
			require.Empty(t, next.Items)
		})
	}
}

func TestPostListFailures(t *testing.T) {
	for _, path := range []string{"/post.listCreator", "/post.listTagged", "/post.listHome", "/post.listSupporting"} {
		for _, tc := range []struct {
			name, body string
			status     int
			reason     sdk.Reason
		}{
			{"missing", `{}`, 200, sdk.MalformedUpstreamResponse},
			{"null", `{"body":{"posts":null,"items":null}}`, 200, sdk.MalformedUpstreamResponse},
			{"wrong list", `{"body":{"posts":{}}}`, 200, sdk.MalformedUpstreamResponse},
			{"missing id", `{"body":{"posts":[{}]}}`, 200, sdk.MalformedUpstreamResponse},
			{"wrong continuation", `{"body":{"posts":[],"nextUrl":"https://api.fanbox.cc/post.info?offset=1"}}`, 200, sdk.MalformedUpstreamResponse},
			{"changed selector", `{"body":{"posts":[],"nextUrl":"https://api.fanbox.cc` + path + `?creatorId=other&offset=1"}}`, 200, sdk.MalformedUpstreamResponse},
			{"expired", `{}`, 401, sdk.CredentialsExpired},
			{"forbidden", `{}`, 403, sdk.Forbidden},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/" {
						fmt.Fprint(w, `<meta name="metadata" content='{"context":{"user":{"userId":21,"name":"fixture"}}}'>`)
						return
					}
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
				}))
				defer server.Close()
				httpClient := server.Client()
				routeToServer(httpClient, server)
				client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
				require.NoError(t, err)
				_, err = readPostPage(t.Context(), client, path, sdk.Cursor{})
				require.Equal(t, tc.reason, sdk.ReasonOf(err))
			})
		}
	}
}
