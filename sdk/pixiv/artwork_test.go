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

const artworkResponse = `{"illust":{"id":23,"title":"fixture","type":"illust","create_date":"2026-01-01T09:00:00+09:00","user":{"id":11},"illust_ai_type":2,"ai_type":1,"page_count":1,"width":640,"height":480,"meta_single_page":{"original_image_url":"https://i.pximg.net/fixture.jpg"}}}`

type artworkEndpointCase struct {
	name, body string
	status     int
	cancel     bool
	reason     sdk.Reason
}

func TestArtworkFailures(t *testing.T) {
	for _, tc := range []artworkEndpointCase{
		{"upstream", `{}`, 403, false, sdk.Forbidden},
		{"malformed", `{"illust":null}`, 200, false, sdk.MalformedUpstreamResponse},
		{"wrong_id", `{"illust":{"id":24,"create_date":"2026-01-01T00:00:00Z"}}`, 200, false, sdk.MalformedUpstreamResponse},
		{"missing_pages", `{"illust":{"id":23,"create_date":"2026-01-01T00:00:00Z","page_count":2}}`, 200, false, sdk.MalformedUpstreamResponse},
		{"partial_pages", `{"illust":{"id":23,"create_date":"2026-01-01T00:00:00Z","page_count":2,"meta_pages":[{"image_urls":{"original":"https://i.pximg.net/0.jpg"}}]}}`, 200, false, sdk.MalformedUpstreamResponse},
		{"preview_only", `{"illust":{"id":23,"create_date":"2026-01-01T00:00:00Z","page_count":1,"meta_pages":[{"image_urls":{"large":"https://i.pximg.net/preview.jpg"}}]}}`, 200, false, sdk.MalformedUpstreamResponse},
		{"cancel", `{}`, 200, true, sdk.UpstreamUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			credentials := pixiv.NewCredentials("fixture-refresh")
			credentials.SetTokens("fixture-access", "fixture-refresh")
			credentials.UserID, credentials.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(credentials)
			require.NoError(t, err)
			client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			_, err = client.Artwork(23).Get(ctx)
			require.Error(t, err)
			if tc.cancel {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.Equal(t, tc.reason, sdk.ReasonOf(err))
			}
		})
	}
}

func TestArtworkObjects(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		serveArtwork(t, w, r)
	}))
	defer server.Close()
	credentials := pixiv.NewCredentials("fixture-refresh")
	credentials.SetTokens("fixture-access", "fixture-refresh")
	credentials.UserID, credentials.ExpiresAt = 11, time.Now().Add(time.Hour)
	store, err := pixiv.NewMemoryCredentials(credentials)
	require.NoError(t, err)
	client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
	require.NoError(t, err)
	ref := client.Artwork(23)
	require.Zero(t, calls)
	art, err := ref.Get(t.Context())
	require.NoError(t, err)
	require.Equal(t, "fixture", art.Title)
	require.EqualValues(t, 11, art.Author.ID)
	_, err = art.Author.Artworks(t.Context(), pixiv.UserArtworksOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, art.AIType)
	require.Len(t, art.Pages, 1)
	require.Same(t, art, art.Pages[0].Artwork)
	data, err := json.Marshal(art)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, art))
	require.Equal(t, "fixture", art.Title)
	page := pixiv.ArtworkPage{Artwork: &pixiv.Artwork{ArtworkRef: client.Artwork(23)}}
	require.NoError(t, json.Unmarshal([]byte(`{}`), &page))
	require.Nil(t, page.Artwork)
	_, err = art.Get(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = art.Pages[0].Original.Open(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

func TestUgoiraMetadata(t *testing.T) {
	for _, tc := range []artworkEndpointCase{
		{"normal", `{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/fixture.zip","original":"https://i.pximg.net/original.zip"},"frames":[{"file":"000.jpg","delay":80}]}}`, 200, false, ""},
		{"upstream", `{}`, 403, false, sdk.Forbidden},
		{"malformed", `{"ugoira_metadata":{}}`, 200, false, sdk.MalformedUpstreamResponse},
		{"cancel", `{}`, 200, true, sdk.UpstreamUnavailable},
		{"unsafe_frame", `{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/fixture.zip"},"frames":[{"file":"../000.jpg","delay":80}]}}`, 200, false, sdk.MalformedUpstreamResponse},
		{"negative_delay", `{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/fixture.zip"},"frames":[{"file":"000.jpg","delay":-80}]}}`, 200, false, sdk.MalformedUpstreamResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			credentials := pixiv.NewCredentials("fixture-refresh")
			credentials.SetTokens("fixture-access", "fixture-refresh")
			credentials.UserID, credentials.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(credentials)
			require.NoError(t, err)
			client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			value, err := client.Artwork(23).UgoiraMetadata(ctx)
			if tc.cancel {
				require.ErrorIs(t, err, context.Canceled)
			} else if tc.reason != "" {
				require.Equal(t, tc.reason, sdk.ReasonOf(err))
			} else {
				require.NoError(t, err)
				assertUgoira(t, value)
			}
		})
	}
}

func TestRelatedArtworkContinuation(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, api.PathIllustRelated, r.URL.Path)
		require.Equal(t, "23", r.URL.Query().Get(api.ParamIllustID))
		if calls == 1 {
			fmt.Fprint(w, `{"illusts":[],"next_url":"https://app-api.pixiv.net/v2/illust/related?illust_id=23&seed_illust_ids[0]=24&viewed[0]=25"}`)
		} else {
			require.Equal(t, []string{"24"}, r.URL.Query()[api.ParamSeedIllustIDs])
			require.Equal(t, []string{"25"}, r.URL.Query()[api.ParamViewed])
			fmt.Fprint(w, `{"illusts":[]}`)
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
	page, err := client.Artwork(23).Related(t.Context(), pixiv.RelatedArtworksOptions{})
	require.NoError(t, err)
	_, err = client.Artwork(99).Related(t.Context(), pixiv.RelatedArtworksOptions{Cursor: page.Cursor})
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	data, err := json.Marshal(page)
	require.NoError(t, err)
	var decoded sdk.Page[*pixiv.Artwork]
	require.NoError(t, json.Unmarshal(data, &decoded))
	_, err = decoded.Next(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = page.Next(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func serveArtwork(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	if r.URL.Path == api.PathUserIllusts {
		require.Equal(t, "11", r.URL.Query().Get(api.ParamUserID))
		fmt.Fprint(w, `{"illusts":[]}`)
		return
	}
	require.Equal(t, api.PathIllustDetail, r.URL.Path)
	require.Equal(t, "23", r.URL.Query().Get(api.ParamIllustID))
	fmt.Fprint(w, artworkResponse)
}

func assertUgoira(t *testing.T, value pixiv.UgoiraMetadata) {
	t.Helper()
	require.Len(t, value.Archives, 2)
	require.Equal(t, pixiv.UgoiraQualityMedium, value.Archives[0].Quality)
	require.Equal(t, "https://i.pximg.net/fixture.zip", value.Archives[0].URL)
	require.Equal(t, pixiv.UgoiraQualityOriginal, value.Archives[1].Quality)
	require.Equal(t, "https://i.pximg.net/original.zip", value.Archives[1].URL)
	require.Equal(t, 80, value.Frames[0].DelayMilliseconds)
	archive := value.Archives[0]
	require.NoError(t, json.Unmarshal([]byte(`{"url":"https://i.pximg.net/f.zip","quality":"original"}`), &archive))
	require.Equal(t, pixiv.UgoiraQualityOriginal, archive.Quality)
	_, err := archive.Open(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}
