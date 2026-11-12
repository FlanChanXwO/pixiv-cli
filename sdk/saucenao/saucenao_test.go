package saucenao_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/saucenao"
	"github.com/stretchr/testify/require"
)

const successJSON = `{"header":{"status":0,"short_remaining":"0","long_remaining":70,"short_limit":"6","long_limit":100},"results":[{"header":{"similarity":"94.5","index_id":5,"index_name":"Pixiv"},"data":{"pixiv_id":"123","member_id":45,"title":"fixture","member_name":"member","author_name":"other","ext_urls":["https://www.pixiv.net/artworks/123"]}},{"header":{"similarity":50,"index_id":"5","index_name":"Pixiv"},"data":{"ext_urls":["https://www.pixiv.net/artworks/456"],"author_name":"author"}}]}`

func routeClient(server *httptest.Server) *http.Client {
	client := server.Client()
	transport := client.Transport.(*http.Transport)
	transport.Proxy = nil
	transport.TLSClientConfig.ServerName = server.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	return client
}

func TestSearch(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/search.php" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Error(err)
			return
		}
		defer r.MultipartForm.RemoveAll()
		if r.FormValue("api_key") != "key-fixture" || r.FormValue("output_type") != "2" || r.FormValue("db") != "999" {
			t.Error("unexpected search fields")
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil || string(data) != "image-fixture" {
			t.Error("unexpected upload")
		}
		fmt.Fprint(w, successJSON)
	}))
	defer server.Close()
	result, err := saucenao.Search(t.Context(), saucenao.Options{Image: []byte("image-fixture"), APIKey: "key-fixture", HTTPClient: routeClient(server)})
	require.NoError(t, err)
	require.Equal(t, &saucenao.Quota{ShortRemaining: 0, LongRemaining: 70, ShortLimit: 6, LongLimit: 100}, result.Quota)
	require.Equal(t, []saucenao.Match{
		{Rank: 1, Similarity: 94.5, IndexID: 5, IndexName: "Pixiv", Title: "fixture", Author: "member", ArtworkID: 123, UserID: 45, ExternalURLs: []string{"https://www.pixiv.net/artworks/123"}},
		{Rank: 2, Similarity: 50, IndexID: 5, IndexName: "Pixiv", Author: "author", ArtworkID: 456, ExternalURLs: []string{"https://www.pixiv.net/artworks/456"}},
	}, result.Matches)
}

func TestSearchFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		cancel bool
		reason sdk.Reason
	}{
		{"quota", 200, `{"header":{"status":-1,"short_remaining":0,"long_remaining":0,"short_limit":6,"long_limit":100}}`, false, sdk.RateLimited},
		{"http limit", 429, "", false, sdk.RateLimited},
		{"server", 503, "", false, sdk.UpstreamUnavailable},
		{"format", 200, `{"header":{"status":0},"results":[]}`, false, sdk.MalformedUpstreamResponse},
		{"trailing", 200, successJSON + ` {}`, false, sdk.MalformedUpstreamResponse},
		{"rejected", 200, `{"header":{"status":-1}}`, false, sdk.UpstreamError},
		{"cancel", 200, "", true, sdk.UpstreamUnavailable},
		{"redirect", 307, "", false, sdk.ResourceForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) { searchFailure(t, tc.status, tc.body, tc.cancel, tc.reason) })
	}
}

func searchFailure(t *testing.T, status int, body string, canceled bool, reason sdk.Reason) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
		}
		if canceled {
			cancel()
			<-r.Context().Done()
			return
		}
		if status == http.StatusTemporaryRedirect {
			w.Header().Set("Location", "https://other.invalid/search")
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	result, err := saucenao.Search(ctx, saucenao.Options{Image: []byte("image-fixture"), APIKey: "key-fixture", HTTPClient: routeClient(server)})
	require.Equal(t, reason, sdk.ReasonOf(err))
	var failure *sdk.Error
	require.ErrorAs(t, err, &failure)
	if canceled {
		require.ErrorIs(t, err, context.Canceled)
	}
	if status >= 400 {
		require.Equal(t, status, failure.HTTPStatus)
	}
	if body != "" && reason == sdk.RateLimited {
		require.NotNil(t, result.Quota)
		require.Zero(t, result.Quota.ShortRemaining)
		require.Zero(t, result.Quota.LongRemaining)
	}
	require.NotContains(t, fmt.Sprintf("%#v", err), "key-fixture")
}
