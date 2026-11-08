package pixiv

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/stretchr/testify/require"
)

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
		{"cancel", `{}`, 200, true, sdk.UpstreamUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			credentials := NewCredentials("fixture-refresh")
			credentials.SetTokens("fixture-access", "fixture-refresh")
			credentials.UserID, credentials.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := NewMemoryCredentials(credentials)
			require.NoError(t, err)
			client, err := New(store, Options{HTTPClient: fixtureHTTP(server)})
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
		require.Equal(t, "23", r.URL.Query().Get(api.ParamIllustID))
		fmt.Fprint(w, `{"illust":{"id":23,"title":"fixture","type":"illust","create_date":"2026-01-01T09:00:00+09:00","user":{"id":11},"page_count":1,"width":640,"height":480,"meta_single_page":{"original_image_url":"https://i.pximg.net/fixture.jpg"}}}`)
	}))
	defer server.Close()
	credentials := NewCredentials("fixture-refresh")
	credentials.SetTokens("fixture-access", "fixture-refresh")
	credentials.UserID, credentials.ExpiresAt = 11, time.Now().Add(time.Hour)
	store, err := NewMemoryCredentials(credentials)
	require.NoError(t, err)
	client, err := New(store, Options{HTTPClient: fixtureHTTP(server)})
	require.NoError(t, err)
	ref := client.Artwork(23)
	require.Zero(t, calls)
	art, err := ref.Get(t.Context())
	require.NoError(t, err)
	require.Equal(t, "fixture", art.Title)
	require.EqualValues(t, 11, art.Author.ID)
	require.Same(t, client, art.Author.client)
	require.Len(t, art.Pages, 1)
	require.Same(t, art, art.Pages[0].Artwork)
	data, err := json.Marshal(art)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, art))
	require.Equal(t, "fixture", art.Title)
	_, err = art.Get(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	_, err = art.Pages[0].Original.Open(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

func TestUgoiraMetadata(t *testing.T) {
	for _, tc := range []artworkEndpointCase{
		{"normal", `{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/fixture.zip"},"frames":[{"file":"000.jpg","delay":80}]}}`, 200, false, ""},
		{"upstream", `{}`, 403, false, sdk.Forbidden},
		{"malformed", `{"ugoira_metadata":{}}`, 200, false, sdk.MalformedUpstreamResponse},
		{"cancel", `{}`, 200, true, sdk.UpstreamUnavailable},
		{"unsafe_frame", `{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/fixture.zip"},"frames":[{"file":"../000.jpg","delay":80}]}}`, 200, false, sdk.MalformedUpstreamResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			credentials := NewCredentials("fixture-refresh")
			credentials.SetTokens("fixture-access", "fixture-refresh")
			credentials.UserID, credentials.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := NewMemoryCredentials(credentials)
			require.NoError(t, err)
			client, err := New(store, Options{HTTPClient: fixtureHTTP(server)})
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
				require.Len(t, value.Archives, 1)
				require.Equal(t, 80, value.Frames[0].DelayMilliseconds)
			}
		})
	}
}

func TestImageSave(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "image bytes") }))
	defer server.Close()
	credentials := NewCredentials("fixture-refresh")
	credentials.SetTokens("fixture-access", "fixture-refresh")
	credentials.UserID, credentials.ExpiresAt = 11, time.Now().Add(time.Hour)
	store, err := NewMemoryCredentials(credentials)
	require.NoError(t, err)
	client, err := New(store, Options{HTTPClient: fixtureHTTP(server)})
	require.NoError(t, err)
	image, err := client.image("https://i.pximg.net/fixture.jpg", 640, 480)
	require.NoError(t, err)
	dest := filepath.Join(t.TempDir(), "image.jpg")
	require.NoError(t, image.Save(t.Context(), dest))
	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, "image bytes", string(data))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, image.Save(ctx, dest), context.Canceled)
	data, err = os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, "image bytes", string(data))
}
