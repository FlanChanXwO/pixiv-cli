package api_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv/internal/api"
	"github.com/stretchr/testify/require"
)

func TestRequestLogsKeepSecretsPrivate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-canary" || r.URL.Query().Get("word") != "query-canary" || r.Header.Get("Cookie") != "session=cookie-canary" {
			t.Error("request did not carry the credential and query canaries")
		}
		fmt.Fprint(w, `{"payload":"body-canary"}`)
	}))
	defer server.Close()
	httpClient := server.Client()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	jar.SetCookies(&url.URL{Scheme: "https", Host: "app-api.pixiv.net"}, []*http.Cookie{{Name: "session", Value: "cookie-canary"}})
	httpClient.Jar = jar
	transport := httpClient.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = server.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	client := api.New(api.Config{HTTPClient: httpClient, AccessToken: "access-canary"})
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)
	query := url.Values{api.ParamWord: {"query-canary"}, "url": {"https://i.pximg.net/image.png?signature=signature-canary"}}
	for _, failed := range []bool{false, true} {
		output.Reset()
		if failed {
			server.Close()
		}
		var response map[string]any
		err = client.GetJSON(t.Context(), api.PathSearchIllust, query, &response)
		require.Equal(t, failed, err != nil)
		require.Contains(t, output.String(), api.PathSearchIllust)
		require.NotRegexp(t, `(?:access|query|signature|body|cookie)-canary|https://i\.pximg\.net`, output.String())
	}
}
