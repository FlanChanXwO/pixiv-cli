package fanbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

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

func TestChallengeRecovery(t *testing.T) {
	var solves atomic.Int32
	solver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if json.NewDecoder(r.Body).Decode(&payload) != nil {
			t.Error("invalid control request")
		}
		if r.URL.Path != "/v1" || payload["cmd"] != "request.get" || payload["url"] != "https://www.fanbox.cc/" || len(payload) != 2 || r.Header.Get("Cookie") != "" {
			t.Error("solver target or credentials escaped the anonymous boundary")
		}
		solves.Add(1)
		fmt.Fprint(w, `{"status":"ok","solution":{"userAgent":"solved-agent","cookies":[{"name":"cf_clearance","value":"clearance-fixture"},{"name":"FANBOXSESSID","value":"discard-fixture"}]}}`)
	}))
	defer solver.Close()
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Cookie") == "FANBOXSESSID=session-fixture" {
			w.Header().Set("Cf-Mitigated", "challenge")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Header.Get("Cookie") != "FANBOXSESSID=session-fixture; cf_clearance=clearance-fixture" || r.Header.Get("User-Agent") != "solved-agent" {
			t.Error("incorrect replay credentials or agent")
		}
		fmt.Fprint(w, `<meta name="metadata" content='{"context":{"user":{"userId":21,"name":"fixture"}}}'>`)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient, FlareSolverrURL: solver.URL})
	require.NoError(t, err)
	require.NoError(t, client.ValidateSession(t.Context()))
	require.NoError(t, client.ValidateSession(t.Context()))
	require.EqualValues(t, 1, solves.Load())
	require.EqualValues(t, 3, calls.Load())
	require.NoError(t, client.Close(t.Context()))
}

func TestSolverCancellation(t *testing.T) {
	started := make(chan struct{})
	solver := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
		}
		close(started)
		<-r.Context().Done()
	}))
	defer solver.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient, FlareSolverrURL: solver.URL})
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

func TestUnavailableSolver(t *testing.T) {
	solver := httptest.NewServer(http.NotFoundHandler())
	solver.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient, FlareSolverrURL: solver.URL})
	require.NoError(t, err)
	err = client.ValidateSession(t.Context())
	require.Equal(t, sdk.UpstreamUnavailable, sdk.ReasonOf(err))
	require.NotContains(t, fmt.Sprintf("%#v", err), "session-fixture")
	require.NoError(t, client.Close(t.Context()))
}
