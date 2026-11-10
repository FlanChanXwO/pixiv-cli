package pixiv_test

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
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

const novelMetadata = `{"id":23,"title":"fixture","caption":"caption","user":{"id":11,"name":"author"},"create_date":"2026-11-01T10:00:00+09:00","tags":[{"name":"tag"}],"image_urls":{"large":"https://i.pximg.net/cover.jpg"},"text_length":17,"is_original":true,"x_restrict":0,"total_bookmarks":2,"total_view":3}`

var novelEndpoints = []endpointContract{
	{"novel", api.PathNovelDetail, `{"novel":` + novelMetadata + `,"series_next":{"id":24,"title":"next"}}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		n, err := c.Novel(23).Get(ctx)
		if err == nil {
			require.Equal(t, "fixture", n.Title)
			require.EqualValues(t, 11, n.Author.ID)
			require.Equal(t, "next", n.SeriesNext.Title)
			require.Equal(t, 17, n.TextLength)
		}
		return err
	}},
	{"novels", api.PathUserNovels, `{"novels":[` + novelMetadata + `]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		p, err := c.User(11).Novels(ctx, pixiv.UserNovelsOptions{})
		if err == nil {
			require.Len(t, p.Items, 1)
			require.Equal(t, "fixture", p.Items[0].Title)
			require.Equal(t, time.UTC, p.Items[0].PublishedAt.Location())
		}
		return err
	}},
}

func TestNovelEndpointContracts(t *testing.T) { testEndpointContracts(t, novelEndpoints) }

func TestNovelContinuationAndBinding(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, api.PathUserNovels, r.URL.Path)
		require.Equal(t, "11", r.URL.Query().Get(api.ParamUserID))
		require.Equal(t, "for_android", r.URL.Query().Get(api.ParamFilter))
		if calls == 1 {
			fmt.Fprint(w, `{"novels":[`+novelMetadata+`],"next_url":"https://app-api.pixiv.net/v1/user/novels?user_id=11&filter=for_android&offset=2"}`)
		} else {
			require.Equal(t, "2", r.URL.Query().Get(api.ParamOffset))
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
	page, err := client.User(11).Novels(t.Context(), pixiv.UserNovelsOptions{})
	require.NoError(t, err)
	token, err := sdk.ParseCursor(page.Cursor.String())
	require.NoError(t, err)
	_, err = client.User(12).Novels(t.Context(), pixiv.UserNovelsOptions{Cursor: token})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	require.Equal(t, 1, calls)
	_, err = page.Next(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func TestNovelDecodeClearsBinding(t *testing.T) {
	client := &pixiv.Client{}
	novel := pixiv.Novel{NovelRef: client.Novel(23)}
	require.NoError(t, json.Unmarshal([]byte(`{"id":23,"title":"fixture","author":{"id":11},"cover":{"url":"https://i.pximg.net/cover.jpg"},"series_next":{"id":24,"title":"next"}}`), &novel))
	require.Equal(t, "fixture", novel.Title)
	_, err := novel.Get(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = novel.Author.Profile(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = novel.Cover.Open(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = novel.SeriesNext.Get(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

func TestNovelMalformedResponses(t *testing.T) {
	for _, body := range []string{
		`{"novel":null}`, `{"novel":{"id":24,"user":{"id":11}}}`, `{"novel":{"id":23,"user":{"id":0}}}`,
		`{"novel":{"id":23,"user":{"id":11},"create_date":"invalid"}}`,
		`{"novel":` + novelMetadata + `,"series_next":{"id":0}}`,
	} {
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
			_, err = client.Novel(23).Get(t.Context())
			require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
		})
	}
}

func TestNovelContentUnavailable(t *testing.T) {
	client := &pixiv.Client{}
	_, err := client.Novel(23).Content(t.Context())
	require.Equal(t, sdk.ContentUnavailable, sdk.ReasonOf(err))
	_, err = client.Novel(0).Content(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	var ref pixiv.NovelRef
	require.NoError(t, json.Unmarshal([]byte(`{"id":23}`), &ref))
	_, err = ref.Content(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}
