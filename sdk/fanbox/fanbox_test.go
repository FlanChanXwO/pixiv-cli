package fanbox_test

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

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestSessionIdentity(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "FANBOXSESSID=session-fixture" || r.Header.Get("Referer") != "https://www.fanbox.cc/" {
			t.Error("incorrect session headers")
		}
		fmt.Fprint(w, `<meta name="metadata" content='{"context":{"user":{"userId":"21","name":"fixture","creatorId":"artist","creatorStatus":"active"}}}'>`)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	user, err := client.CurrentUser(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(21), user.UserID)
	require.Equal(t, "fixture", user.DisplayName)
	require.Equal(t, "artist", user.CreatorID)
	require.True(t, user.IsCreator)
	require.NoError(t, client.ValidateSession(t.Context()))
}

func TestSessionFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		reason     sdk.Reason
	}{
		{"expired", "", http.StatusUnauthorized, sdk.CredentialsExpired},
		{"forbidden", `{}`, http.StatusForbidden, sdk.Forbidden},
		{"metadata missing", `<html></html>`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"metadata invalid", `<meta name="metadata" content='secret-fixture'>`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"user missing", `<meta name="metadata" content='{"context":{}}'>`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"invalid id", `<meta name="metadata" content='{"context":{"user":{"userId":0,"name":"fixture"}}}'>`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"empty name", `<meta name="metadata" content='{"context":{"user":{"userId":21,"name":" "}}}'>`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"upstream", "", http.StatusServiceUnavailable, sdk.UpstreamUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			err = client.ValidateSession(t.Context())
			require.Equal(t, tc.reason, sdk.ReasonOf(err))
			require.NotContains(t, fmt.Sprint(err), "secret-fixture")
		})
	}
}

func routeToServer(client *http.Client, server *httptest.Server) {
	transport := client.Transport.(*http.Transport)
	transport.Proxy = nil
	transport.TLSClientConfig.ServerName = server.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
}

func TestConnectionInputPrivacy(t *testing.T) {
	for _, tc := range []struct {
		session string
		options fanbox.Options
	}{
		{"", fanbox.Options{}},
		{"secret-fixture\r\n", fanbox.Options{}},
		{"secret-fixture; extra=1", fanbox.Options{}},
		{"secret-fixture", fanbox.Options{UserAgent: "agent\r\nsecret-fixture"}},
		{"secret-fixture", fanbox.Options{ProxyURL: "http://secret-fixture@proxy.invalid"}},
		{"secret-fixture", fanbox.Options{FlareSolverrURL: "http://secret-fixture@solver.invalid"}},
		{"secret-fixture", fanbox.Options{FlareSolverrProxyURL: "socks5://secret-fixture@proxy.invalid"}},
		{"secret-fixture", fanbox.Options{HTTPClient: &http.Client{}}},
	} {
		_, err := fanbox.New(tc.session, tc.options)
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
		require.NotContains(t, fmt.Sprint(err), "secret-fixture")
	}
}

func TestSessionCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- client.ValidateSession(ctx) }()
	<-started
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	require.NoError(t, client.Close(t.Context()))
}

func TestRedirectAndDiagnosticPrivacy(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Location", "/second?secret=signature-fixture")
			w.WriteHeader(http.StatusFound)
			return
		}
		if r.Header.Get("Cookie") != "" {
			t.Error("redirect forwarded credentials or caller jar")
		}
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	httpClient.Jar = jar
	target := &url.URL{Scheme: "https", Host: "www.fanbox.cc", Path: "/"}
	httpClient.Jar.SetCookies(target, []*http.Cookie{{Name: "extra", Value: "jar-fixture"}})
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)
	err = client.ValidateSession(t.Context())
	require.Equal(t, sdk.RateLimited, sdk.ReasonOf(err))
	var failure *sdk.Error
	require.ErrorAs(t, err, &failure)
	require.Equal(t, http.StatusTooManyRequests, failure.HTTPStatus)
	require.EqualValues(t, 7e9, *failure.RetryAfter)
	fmt.Fprintf(&output, "%v %+v %#v", client, client, client)
	require.Contains(t, output.String(), "second")
	for _, secret := range []string{"session-fixture", "signature-fixture", "jar-fixture"} {
		require.NotContains(t, output.String(), secret)
	}
	require.NoError(t, client.Close(t.Context()))
}
