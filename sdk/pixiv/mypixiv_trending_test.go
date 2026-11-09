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

var mypixivFeeds = []feedEndpoint{
	{"mypixiv_artworks", api.PathIllustMyPixiv, artworkFeeds[0].body, url.Values{}, url.Values{"offset": {"30"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
		p, e := c.MyPixivArtworks(ctx, pixiv.MyPixivOptions{Cursor: token})
		return p.Cursor, e
	}},
	{"mypixiv_novels", api.PathNovelMyPixiv, novelFeeds[0].body, url.Values{}, url.Values{"offset": {"30"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
		p, e := c.MyPixivNovels(ctx, pixiv.MyPixivOptions{Cursor: token})
		return p.Cursor, e
	}},
	{"mypixiv_users", api.PathUserMyPixiv, `{"user_previews":[{"user":{"id":12}}]}`, url.Values{"user_id": {"11"}, "filter": {"for_android"}}, url.Values{"offset": {"30"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
		p, e := c.MyPixivUsers(ctx, pixiv.MyPixivOptions{Cursor: token})
		return p.Cursor, e
	}},
}

func TestMyPixivEndpointContracts(t *testing.T) {
	endpoints := make([]endpointContract, len(mypixivFeeds))
	for i, feed := range mypixivFeeds {
		endpoints[i] = endpointContract{feed.name, feed.path, feed.body, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
			_, err := feed.call(ctx, c, sdk.Cursor{})
			return err
		}}
	}
	testEndpointContracts(t, endpoints)
}

func TestMyPixivContinuations(t *testing.T) {
	for _, feed := range mypixivFeeds {
		t.Run(feed.name, func(t *testing.T) { testFeedContinuation(t, feed) })
	}
}

func TestTrendingEndpointContract(t *testing.T) {
	testEndpointContracts(t, []endpointContract{{"trending", api.PathTrendingTagsIllust, `{"trend_tags":[{"tag":"tag","translated_name":"translation","illust":` + searchArtworkFirst + `}]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		tags, err := c.TrendingArtworkTags(ctx)
		if err == nil {
			require.Len(t, tags, 1)
			require.Equal(t, pixiv.Tag{Name: "tag", TranslatedName: "translation"}, tags[0].Tag)
			require.EqualValues(t, 23, tags[0].Artwork.ID)
			require.NoError(t, json.Unmarshal([]byte(`{}`), &tags[0]))
			require.Nil(t, tags[0].Artwork)
			require.Empty(t, tags[0].Tag.Name)
		}
		return err
	}}})
}

func TestMyPixivAndTrendingMalformedItems(t *testing.T) {
	for _, test := range []struct{ path, body string }{
		{api.PathTrendingTagsIllust, `{"trend_tags":[{"tag":"tag"}]}`},
		{api.PathTrendingTagsIllust, `{"trend_tags":[{"tag":"tag","illust":null}]}`},
		{api.PathTrendingTagsIllust, `{"trend_tags":[{"tag":"","illust":` + searchArtworkFirst + `}]}`},
		{api.PathTrendingTagsIllust, `{"trend_tags":[{"tag":"tag","illust":{"id":23,"create_date":"broken"}}]}`},
		{api.PathIllustMyPixiv, strings.Replace(mypixivFeeds[0].body, `"id":11`, `"id":0`, 1)},
	} {
		t.Run(test.body, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, test.path, r.URL.Path)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			value := pixiv.NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(value)
			require.NoError(t, err)
			c, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			if test.path == api.PathIllustMyPixiv {
				_, err = c.MyPixivArtworks(t.Context(), pixiv.MyPixivOptions{})
			} else {
				_, err = c.TrendingArtworkTags(t.Context())
			}
			require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
		})
	}
}

func TestTrendingPreservesUnidentifiedAuthor(t *testing.T) {
	for _, user := range []string{``, `,"user":{"id":0}`} {
		artwork := strings.Replace(searchArtworkFirst, `,"user":{"id":11}`, user, 1)
		testEndpointContracts(t, []endpointContract{{"unidentified_author", api.PathTrendingTagsIllust, `{"trend_tags":[{"tag":"tag","illust":` + artwork + `}]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
			tags, err := c.TrendingArtworkTags(ctx)
			if err == nil {
				require.Zero(t, tags[0].Artwork.Author.ID)
				_, profileErr := tags[0].Artwork.Author.Profile(ctx)
				require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(profileErr))
			}
			return err
		}}})
	}
}
