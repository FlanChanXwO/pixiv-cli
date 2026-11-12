package pixiv_test

import (
	"context"
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

const searchArtworkFirst = `{"id":23,"title":"first","type":"illust","create_date":"2026-01-01T00:00:00Z","user":{"id":11},"illust_ai_type":2,"page_count":1,"meta_single_page":{"original_image_url":"https://i.pximg.net/first.jpg"}}`
const searchArtworkSecond = `{"id":24,"title":"second","type":"illust","create_date":"2026-01-02T00:00:00Z","user":{"id":11},"illust_ai_type":2,"page_count":1,"meta_single_page":{"original_image_url":"https://i.pximg.net/second.jpg"}}`
const searchArtworkNonAI = `{"id":25,"title":"non-AI","type":"illust","create_date":"2026-01-03T00:00:00Z","user":{"id":11},"illust_ai_type":1,"page_count":1,"meta_single_page":{"original_image_url":"https://i.pximg.net/non-ai.jpg"}}`
const searchArtworkBatch = `{"illusts":[` + searchArtworkFirst + `,` + searchArtworkNonAI + `,` + searchArtworkSecond + `],"next_url":"https://app-api.pixiv.net/v1/search/illust?offset=50"}`

var searchEndpoints = []endpointContract{
	{"search_artworks", api.PathSearchIllust, `{"illusts":[` + searchArtworkFirst + `]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		p, e := c.SearchArtworks(ctx, "word", pixiv.ArtworkSearchOptions{})
		if e == nil {
			require.Len(t, p.Items, 1)
			require.EqualValues(t, 23, p.Items[0].ID)
		}
		return e
	}},
	{"search_novels", api.PathSearchNovel, `{"novels":[` + novelMetadata + `]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		p, e := c.SearchNovels(ctx, "word", pixiv.NovelSearchOptions{})
		if e == nil {
			require.Len(t, p.Items, 1)
			require.Equal(t, 17, p.Items[0].TextLength)
		}
		return e
	}},
	{"search_users", api.PathSearchUser, `{"user_previews":[{"user":{"id":11,"name":"fixture"}}]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
		p, e := c.SearchUsers(ctx, "word", pixiv.UserSearchOptions{})
		if e == nil {
			require.Len(t, p.Items, 1)
			require.Equal(t, "fixture", p.Items[0].Name)
		}
		return e
	}},
}

func TestSearchEndpointContracts(t *testing.T) { testEndpointContracts(t, searchEndpoints) }

func TestArtworkSearchFilters(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, api.PathSearchIllust, r.URL.Path)
		require.Equal(t, url.Values{
			api.ParamWord: {" word "}, api.ParamSearchTarget: {"exact_match_for_tags"}, api.ParamSort: {"popular_desc"}, api.ParamDuration: {"within_last_week"},
			api.ParamStartDate: {"2026-01-01"}, api.ParamEndDate: {"2026-01-31"}, api.ParamContentType: {"illust_and_ugoira"}, api.ParamSearchAIType: {"0"}, api.ParamRatioPattern: {"square"},
			api.ParamWidthMin: {"1000"}, api.ParamWidthMax: {"2999"}, api.ParamHeightMin: {"1000"}, api.ParamHeightMax: {"2999"}, api.ParamTool: {"pen"}, api.ParamBookmarkNumMin: {"0"}, api.ParamBookmarkNumMax: {"20"}, api.ParamOffset: {"20"},
		}, r.URL.Query())
		fmt.Fprint(w, searchArtworkBatch)
	}))
	defer server.Close()
	value := pixiv.NewCredentials("fixture-refresh")
	value.SetTokens("fixture-access", "fixture-refresh")
	value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
	store, err := pixiv.NewMemoryCredentials(value)
	require.NoError(t, err)
	client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
	require.NoError(t, err)
	minimum, maximum := 0, 20
	page, err := client.SearchArtworks(t.Context(), " word ", pixiv.ArtworkSearchOptions{
		Target: pixiv.SearchTargetExactMatchForTags, Sort: pixiv.SortModePopularDesc, Duration: pixiv.DurationLastWeek, StartDate: "2026-01-01", EndDate: "2026-01-31", Category: pixiv.ArtworkCategoryIllustAndUgoira,
		AIMode: pixiv.SearchAIModeOnly, AspectRatio: pixiv.SearchAspectRatioSquare, Resolution: pixiv.SearchResolutionMedium, Tool: "pen", BookmarkMin: &minimum, BookmarkMax: &maximum, Offset: 20, CursorContext: "local-only",
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.EqualValues(t, 23, page.Items[0].ID)
	require.EqualValues(t, 24, page.Items[1].ID)
}

func TestSearchArgumentValidation(t *testing.T) {
	c := &pixiv.Client{}
	for _, call := range []func() error{
		func() error { _, e := c.SearchArtworks(t.Context(), " ", pixiv.ArtworkSearchOptions{}); return e },
		func() error { _, e := c.SearchUsers(t.Context(), "", pixiv.UserSearchOptions{}); return e },
		func() error { _, e := c.SearchNovels(t.Context(), "", pixiv.NovelSearchOptions{}); return e },
		func() error {
			_, e := c.SearchNovels(t.Context(), "word", pixiv.NovelSearchOptions{Target: "bad"})
			return e
		},
		func() error {
			_, e := c.SearchNovels(t.Context(), "word", pixiv.NovelSearchOptions{Sort: "bad"})
			return e
		},
		func() error {
			_, e := c.SearchNovels(t.Context(), "word", pixiv.NovelSearchOptions{Duration: "bad"})
			return e
		},
	} {
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(call()))
	}
}

func TestArtworkSearchArgumentValidation(t *testing.T) {
	c := &pixiv.Client{}
	negative, minimum, maximum := -1, 3, 2
	for _, options := range []pixiv.ArtworkSearchOptions{
		{Offset: -1}, {StartDate: "2026-02-30"}, {StartDate: "2026-01-02", EndDate: "2026-01-01"}, {BookmarkMin: &negative}, {BookmarkMax: &negative}, {BookmarkMin: &minimum, BookmarkMax: &maximum}, {Category: "bad"}, {AIMode: "bad"},
	} {
		_, err := c.SearchArtworks(t.Context(), "word", options)
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	}
	for _, options := range []pixiv.ArtworkSearchOptions{{Target: "bad"}, {Sort: "bad"}, {Duration: "bad"}, {AspectRatio: "bad"}, {Resolution: "bad"}, {EndDate: "2026-1-1"}} {
		_, err := c.SearchArtworks(t.Context(), "word", options)
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	}
}

func TestNovelSearchRequiresMetadata(t *testing.T) {
	for _, field := range []string{`,"x_restrict":0`, `,"text_length":17`, `,"is_original":true`} {
		t.Run(field, func(t *testing.T) {
			body := `{"novels":[` + strings.Replace(novelMetadata, field, "", 1) + `]}`
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			value := pixiv.NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			_, err = client.SearchNovels(t.Context(), "word", pixiv.NovelSearchOptions{})
			require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
		})
	}
}
