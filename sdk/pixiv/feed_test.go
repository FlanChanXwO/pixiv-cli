package pixiv_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

type feedEndpoint struct {
	name, path, body string
	query, position  url.Values
	call             func(context.Context, *pixiv.Client, sdk.Cursor) (sdk.Cursor, error)
}

var artworkFeeds = []feedEndpoint{
	{"artwork_ranking", api.PathIllustRanking, `{"illusts":[` + searchArtworkFirst + `]}`, url.Values{"mode": {"day"}}, url.Values{"offset": {"30"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
		p, e := c.ArtworkRanking(ctx, pixiv.ArtworkRankingOptions{Cursor: token})
		return p.Cursor, e
	}},
	{"artwork_recommended", api.PathIllustRecommended, `{"illusts":[` + searchArtworkFirst + `]}`, url.Values{}, url.Values{"offset": {"0"}, "min_bookmark_id_for_recent_illust": {"12"}, "max_bookmark_id_for_recommend": {"13"}, "include_ranking_illusts": {"true"}, "include_privacy_policy": {"false"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
		p, e := c.RecommendedArtworks(ctx, pixiv.RecommendedArtworksOptions{Cursor: token})
		return p.Cursor, e
	}},
	{"artwork_follow", api.PathIllustFollow, `{"illusts":[` + searchArtworkFirst + `]}`, url.Values{"restrict": {"public"}}, url.Values{"offset": {"30"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
		p, e := c.FollowingArtworks(ctx, pixiv.FollowingOptions{Cursor: token})
		return p.Cursor, e
	}},
	{"artwork_latest", api.PathIllustNew, `{"illusts":[` + searchArtworkFirst + `]}`, url.Values{"content_type": {"illust"}, "filter": {"for_android"}}, url.Values{"max_illust_id": {"23"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
		p, e := c.LatestArtworks(ctx, pixiv.LatestArtworksOptions{Cursor: token})
		return p.Cursor, e
	}},
}

var novelFeeds = []feedEndpoint{
	{"novel_ranking", api.PathNovelRanking, `{"novels":[` + novelMetadata + `]}`, url.Values{"mode": {"day"}, "filter": {"for_android"}}, url.Values{"offset": {"30"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
		p, e := c.NovelRanking(ctx, pixiv.NovelRankingOptions{Cursor: token})
		return p.Cursor, e
	}},
	{"novel_recommended", api.PathNovelRecommended, `{"novels":[` + novelMetadata + `]}`, url.Values{}, url.Values{"offset": {"0"}, "already_recommended": {"23,24"}, "max_bookmark_id_for_recommend": {"13"}, "include_ranking_novels": {"true"}, "include_privacy_policy": {"false"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
		p, e := c.RecommendedNovels(ctx, pixiv.RecommendedNovelsOptions{Cursor: token})
		return p.Cursor, e
	}},
	{"novel_follow", api.PathNovelFollow, `{"novels":[` + novelMetadata + `]}`, url.Values{"restrict": {"public"}}, url.Values{"offset": {"30"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
		p, e := c.FollowingNovels(ctx, pixiv.FollowingOptions{Cursor: token})
		return p.Cursor, e
	}},
	{"novel_latest", api.PathNovelNew, `{"novels":[` + novelMetadata + `]}`, url.Values{"filter": {"for_android"}}, url.Values{"max_novel_id": {"23"}}, func(ctx context.Context, c *pixiv.Client, token sdk.Cursor) (sdk.Cursor, error) {
		p, e := c.LatestNovels(ctx, pixiv.LatestNovelsOptions{Cursor: token})
		return p.Cursor, e
	}},
}

func TestFeedEndpointContracts(t *testing.T) {
	for _, feeds := range [][]feedEndpoint{artworkFeeds, novelFeeds} {
		endpoints := make([]endpointContract, len(feeds))
		for i, feed := range feeds {
			endpoints[i] = endpointContract{feed.name, feed.path, feed.body, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
				_, err := feed.call(ctx, c, sdk.Cursor{})
				return err
			}}
		}
		testEndpointContracts(t, endpoints)
	}
}

func TestFeedInvalidArguments(t *testing.T) {
	c := &pixiv.Client{}
	cases := []struct {
		name string
		call func() error
	}{
		{"artwork mode", func() error {
			_, e := c.ArtworkRanking(t.Context(), pixiv.ArtworkRankingOptions{Mode: "invalid"})
			return e
		}},
		{"novel mode", func() error {
			_, e := c.NovelRanking(t.Context(), pixiv.NovelRankingOptions{Mode: "invalid"})
			return e
		}},
		{"date", func() error {
			_, e := c.ArtworkRanking(t.Context(), pixiv.ArtworkRankingOptions{Date: "2026-02-30"})
			return e
		}},
		{"artwork scope", func() error {
			_, e := c.FollowingArtworks(t.Context(), pixiv.FollowingOptions{Restrict: "invalid"})
			return e
		}},
		{"novel scope", func() error {
			_, e := c.FollowingNovels(t.Context(), pixiv.FollowingOptions{Restrict: "invalid"})
			return e
		}},
		{"category", func() error {
			_, e := c.LatestArtworks(t.Context(), pixiv.LatestArtworksOptions{Category: pixiv.ArtworkCategoryUgoira})
			return e
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) { require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(test.call())) })
	}
}
