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

func TestFeedContinuations(t *testing.T) {
	for _, feeds := range [][]feedEndpoint{artworkFeeds, novelFeeds} {
		for _, feed := range feeds {
			t.Run(feed.name, func(t *testing.T) { testFeedContinuation(t, feed) })
		}
	}
}

func testFeedContinuation(t *testing.T, feed feedEndpoint) {
	t.Helper()
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		expected := feed.query
		if calls == 2 {
			expected = feedContinuationQuery(feed)
		}
		require.Equal(t, feed.path, r.URL.Path)
		require.Equal(t, expected, r.URL.Query())
		if calls == 2 {
			fmt.Fprint(w, feed.body)
			return
		}
		next := feedContinuationQuery(feed)
		if feed.path == api.PathIllustRanking && next.Get("date") == "" {
			next.Set("date", "2026-11-01")
		}
		if feed.path == api.PathSearchIllust || feed.path == api.PathSearchNovel {
			next.Set("duration", "")
		}
		if feed.path == api.PathIllustRecommended {
			next.Set("viewed[0]", "23")
		}
		raw, _ := json.Marshal("https://app-api.pixiv.net" + feed.path + "?" + next.Encode())
		fmt.Fprint(w, strings.TrimSuffix(feed.body, "}")+`,"next_url":`+string(raw)+`}`)
	}))
	defer server.Close()
	value := pixiv.NewCredentials("fixture-refresh")
	value.SetTokens("fixture-access", "fixture-refresh")
	value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
	store, err := pixiv.NewMemoryCredentials(value)
	require.NoError(t, err)
	c, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
	require.NoError(t, err)
	token, err := feed.call(t.Context(), c, sdk.Cursor{})
	require.NoError(t, err)
	require.False(t, token.IsZero())
	assertFeedCursorBinding(t, c, feed.name, token)
	next, err := feed.call(t.Context(), c, token)
	require.NoError(t, err)
	require.True(t, next.IsZero())
	require.Equal(t, 2, calls)
}

func feedContinuationQuery(feed feedEndpoint) url.Values {
	query := url.Values{}
	for k, v := range feed.query {
		query[k] = v
	}
	for k, v := range feed.position {
		query[k] = v
	}
	return query
}

func assertFeedCursorBinding(t *testing.T, c *pixiv.Client, name string, token sdk.Cursor) {
	t.Helper()
	var err error
	switch name {
	case "artwork_ranking":
		_, err = c.ArtworkRanking(t.Context(), pixiv.ArtworkRankingOptions{Mode: pixiv.RankingModeWeek, Cursor: token})
	case "week_date":
		_, err = c.ArtworkRanking(t.Context(), pixiv.ArtworkRankingOptions{Mode: pixiv.RankingModeWeek, Date: "2026-11-02", Cursor: token})
	case "novel_ranking":
		_, err = c.NovelRanking(t.Context(), pixiv.NovelRankingOptions{Mode: pixiv.RankingModeWeek, Cursor: token})
	case "artwork_follow":
		_, err = c.FollowingArtworks(t.Context(), pixiv.FollowingOptions{Restrict: pixiv.RestrictPrivate, Cursor: token})
	case "novel_follow":
		_, err = c.FollowingNovels(t.Context(), pixiv.FollowingOptions{Restrict: pixiv.RestrictPrivate, Cursor: token})
	case "artwork_latest":
		_, err = c.LatestArtworks(t.Context(), pixiv.LatestArtworksOptions{Category: pixiv.ArtworkCategoryManga, Cursor: token})
	default:
		_, err = c.ArtworkRanking(t.Context(), pixiv.ArtworkRankingOptions{Cursor: token})
	}
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

func TestFeedMalformedContinuation(t *testing.T) {
	cases := []struct {
		feed     feedEndpoint
		position string
	}{
		{artworkFeeds[0], "offset=0"},
		{artworkFeeds[1], "include_ranking_illusts=1"},
		{artworkFeeds[1], "min_bookmark_id_for_recent_illust=0"},
		{artworkFeeds[1], "offset=0&unknown=1"},
		{artworkFeeds[3], "offset=1&max_illust_id=2"},
		{novelFeeds[1], "already_recommended="},
		{novelFeeds[1], "max_bookmark_id_for_recommend=-1"},
		{novelFeeds[3], "offset=1"},
	}
	for _, test := range cases {
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

func TestFeedOptionsAndLatestOffset(t *testing.T) {
	cases := []feedEndpoint{
		{"week_date", api.PathIllustRanking, artworkFeeds[0].body, url.Values{"mode": {"week"}, "date": {"2026-11-01"}}, url.Values{"offset": {"30"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
			p, e := c.ArtworkRanking(ctx, pixiv.ArtworkRankingOptions{Mode: pixiv.RankingModeWeek, Date: "2026-11-01", Cursor: token})
			return p.Cursor, e
		}},
		{"private_artworks", api.PathIllustFollow, artworkFeeds[2].body, url.Values{"restrict": {"private"}}, url.Values{"offset": {"30"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
			p, e := c.FollowingArtworks(ctx, pixiv.FollowingOptions{Restrict: pixiv.RestrictPrivate, Cursor: token})
			return p.Cursor, e
		}},
		{"private_novels", api.PathNovelFollow, novelFeeds[2].body, url.Values{"restrict": {"private"}}, url.Values{"offset": {"30"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
			p, e := c.FollowingNovels(ctx, pixiv.FollowingOptions{Restrict: pixiv.RestrictPrivate, Cursor: token})
			return p.Cursor, e
		}},
		{"manga_offset", api.PathIllustNew, artworkFeeds[3].body, url.Values{"content_type": {"manga"}, "filter": {"for_android"}}, url.Values{"offset": {"30"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
			p, e := c.LatestArtworks(ctx, pixiv.LatestArtworksOptions{Category: pixiv.ArtworkCategoryManga, Cursor: token})
			return p.Cursor, e
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) { testFeedContinuation(t, test) })
	}
}
