package pixiv_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

const artworkSeriesBody = `{"illust_series_detail":{"user":{"id":11}},"illusts":[` + searchArtworkFirst + `]}`
const novelSeriesDetail = `{"id":7,"title":"series","caption":"caption","user":{"id":11},"is_concluded":true}`
const novelSeriesBody = `{"novel_series_detail":` + novelSeriesDetail + `,"novels":[` + novelMetadata + `]}`

func TestSeriesEndpointContracts(t *testing.T) {
	testEndpointContracts(t, []endpointContract{
		{"artwork_series", api.PathIllustSeries, artworkSeriesBody, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
			page, err := c.ArtworkSeries(7).Artworks(ctx, pixiv.ArtworkSeriesOptions{})
			if err == nil {
				require.Len(t, page.Items, 1)
				require.EqualValues(t, 23, page.Items[0].ID)
				require.EqualValues(t, 11, page.Items[0].Author.ID)
			}
			return err
		}},
		{"novel_series", api.PathNovelSeries, novelSeriesBody, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
			series, err := c.NovelSeries(7).Get(ctx, pixiv.NovelSeriesOptions{})
			if err == nil {
				require.EqualValues(t, 7, series.ID)
				require.Equal(t, "series", series.Title)
				require.Equal(t, "caption", series.Caption)
				require.True(t, series.IsConcluded)
				require.EqualValues(t, 11, series.Author.ID)
				require.Len(t, series.Novels.Items, 1)
				require.EqualValues(t, 23, series.Novels.Items[0].ID)
			}
			return err
		}},
	})
}

func TestSeriesRejectsUnboundReferences(t *testing.T) {
	c := &pixiv.Client{}
	liveArt, liveNovel := c.ArtworkSeries(7), c.NovelSeries(7)
	require.NoError(t, json.Unmarshal([]byte(`{"id":7}`), liveArt))
	require.NoError(t, json.Unmarshal([]byte(`{"id":7}`), liveNovel))
	_, err := liveArt.Artworks(t.Context(), pixiv.ArtworkSeriesOptions{})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = liveNovel.Get(t.Context(), pixiv.NovelSeriesOptions{})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))

	var art *pixiv.ArtworkSeriesRef
	var novel *pixiv.NovelSeriesRef
	_, err = art.Artworks(t.Context(), pixiv.ArtworkSeriesOptions{})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = novel.Get(t.Context(), pixiv.NovelSeriesOptions{})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	for _, id := range []int64{0, -1} {
		_, err = c.ArtworkSeries(id).Artworks(t.Context(), pixiv.ArtworkSeriesOptions{})
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
		_, err = c.NovelSeries(id).Get(t.Context(), pixiv.NovelSeriesOptions{})
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	}
}

func TestSeriesMalformedMetadata(t *testing.T) {
	tests := []struct{ path, body string }{
		{api.PathIllustSeries, `{"illusts":[]}`},
		{api.PathIllustSeries, `{"illust_series_detail":{"user":{"id":0}},"illusts":[]}`},
		{api.PathIllustSeries, `{"illust_series_detail":{"user":{"id":11}},"illusts":[` + strings.Replace(searchArtworkFirst, `"id":11`, `"id":0`, 1) + `]}`},
		{api.PathNovelSeries, strings.Replace(novelSeriesBody, `"id":7`, `"id":8`, 1)},
		{api.PathNovelSeries, strings.Replace(novelSeriesBody, `"id":11`, `"id":0`, 1)},
		{api.PathNovelSeries, `{"novel_series_detail":` + novelSeriesDetail + `}`},
	}
	for _, test := range tests {
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
			if test.path == api.PathIllustSeries {
				_, err = c.ArtworkSeries(7).Artworks(t.Context(), pixiv.ArtworkSeriesOptions{})
			} else {
				_, err = c.NovelSeries(7).Get(t.Context(), pixiv.NovelSeriesOptions{})
			}
			require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
		})
	}
}

func TestArtworkSeriesAssociation(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			require.Equal(t, api.PathIllustDetail, r.URL.Path)
			fmt.Fprint(w, `{"illust":`+strings.TrimSuffix(searchArtworkFirst, "}")+`,"series":{"id":7,"title":"linked"}}}`)
		} else {
			require.Equal(t, api.PathIllustSeries, r.URL.Path)
			require.Equal(t, "7", r.URL.Query().Get("illust_series_id"))
			fmt.Fprint(w, artworkSeriesBody)
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
	art, err := c.Artwork(23).Get(t.Context())
	require.NoError(t, err)
	require.Equal(t, "linked", art.Series.Title)
	canonical, err := art.Series.CanonicalURL()
	require.NoError(t, err)
	require.Equal(t, "https://www.pixiv.net/user/11/series/7", canonical)
	page, err := art.Series.Artworks(t.Context(), pixiv.ArtworkSeriesOptions{})
	require.NoError(t, err)
	require.EqualValues(t, 23, page.Items[0].ID)
	ref, err := c.ParseURL(canonical)
	require.NoError(t, err)
	_, err = ref.(*pixiv.ArtworkSeriesRef).Artworks(t.Context(), pixiv.ArtworkSeriesOptions{})
	require.NoError(t, err)
	require.Equal(t, 3, calls)
	require.NoError(t, json.Unmarshal([]byte(`{"id":7,"title":"decoded"}`), art.Series))
	require.Equal(t, "decoded", art.Series.Title)
	_, err = art.Series.Artworks(t.Context(), pixiv.ArtworkSeriesOptions{})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}
