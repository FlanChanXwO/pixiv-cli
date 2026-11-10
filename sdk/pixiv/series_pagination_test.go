package pixiv_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

var seriesFeeds = []feedEndpoint{
	{"artwork_series", api.PathIllustSeries, artworkSeriesBody, url.Values{"illust_series_id": {"7"}}, url.Values{"last_order": {"2"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
		p, e := c.ArtworkSeries(7).Artworks(ctx, pixiv.ArtworkSeriesOptions{Cursor: token})
		return p.Cursor, e
	}},
	{"novel_series", api.PathNovelSeries, novelSeriesBody, url.Values{"series_id": {"7"}}, url.Values{"last_order": {"2"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
		s, e := c.NovelSeries(7).Get(ctx, pixiv.NovelSeriesOptions{Cursor: token})
		if e != nil {
			return sdk.Cursor{}, e
		}
		return s.Novels.Cursor, nil
	}},
}

func TestSeriesContinuations(t *testing.T) {
	for _, feed := range seriesFeeds {
		t.Run(feed.name, func(t *testing.T) { testFeedContinuation(t, feed) })
	}
	offset := seriesFeeds[0]
	offset.position = url.Values{"offset": {"30"}}
	t.Run("artwork_offset", func(t *testing.T) { testFeedContinuation(t, offset) })
}

func TestSeriesCursorMismatchAndLiveNext(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == api.PathIllustSeries {
			require.Equal(t, "7", r.URL.Query().Get("illust_series_id"))
			fmt.Fprint(w, strings.TrimSuffix(artworkSeriesBody, "}")+`,"next_url":"https://app-api.pixiv.net/v1/illust/series?illust_series_id=7&last_order=2"}`)
		} else {
			require.Equal(t, "7", r.URL.Query().Get("series_id"))
			if calls == 3 {
				require.Equal(t, "2", r.URL.Query().Get("last_order"))
			}
			fmt.Fprint(w, strings.TrimSuffix(novelSeriesBody, "}")+`,"next_url":"https://app-api.pixiv.net/v2/novel/series?series_id=7&last_order=2"}`)
		}
	}))
	defer server.Close()
	value := pixiv.NewCredentials("fixture-refresh")
	value.SetTokens("fixture-access", "fixture-refresh")
	value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
	store, err := pixiv.NewMemoryCredentials(value)
	require.NoError(t, err)
	c, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
	require.NoError(t, err)
	arts, err := c.ArtworkSeries(7).Artworks(t.Context(), pixiv.ArtworkSeriesOptions{})
	require.NoError(t, err)
	_, err = c.ArtworkSeries(8).Artworks(t.Context(), pixiv.ArtworkSeriesOptions{Cursor: arts.Cursor})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	series, err := c.NovelSeries(7).Get(t.Context(), pixiv.NovelSeriesOptions{})
	require.NoError(t, err)
	_, err = c.NovelSeries(8).Get(t.Context(), pixiv.NovelSeriesOptions{Cursor: series.Novels.Cursor})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	series.ID = 9
	page, err := series.Novels.Next(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 23, page.Items[0].ID)
	require.Equal(t, 3, calls)

}

func TestSeriesMalformedContinuation(t *testing.T) {
	for _, test := range []struct {
		feed     feedEndpoint
		position string
	}{
		{seriesFeeds[0], "last_order=0"}, {seriesFeeds[0], "offset=1&last_order=2"},
		{seriesFeeds[1], "offset=2"}, {seriesFeeds[1], "last_order=-1"},
	} {
		t.Run(test.position, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := json.Marshal("https://app-api.pixiv.net" + test.feed.path + "?" + test.position)
				fmt.Fprint(w, strings.TrimSuffix(test.feed.body, "}")+`,"next_url":`+string(raw)+`}`)
			}))
			defer server.Close()
			value := pixiv.NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(value)
			require.NoError(t, err)
			c, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			_, err = test.feed.call(t.Context(), c, sdk.Cursor{})
			require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
		})
	}
}

func TestNovelSeriesDecodeClearsBinding(t *testing.T) {
	c := &pixiv.Client{}
	series := &pixiv.NovelSeries{NovelSeriesRef: c.NovelSeries(7)}
	series.Author = &pixiv.User{UserRef: c.User(11)}
	require.NoError(t, json.Unmarshal([]byte(`{"id":7,"title":"decoded"}`), series))
	require.Equal(t, "decoded", series.Title)
	require.Nil(t, series.Author)
	_, err := series.Get(t.Context(), pixiv.NovelSeriesOptions{})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}
