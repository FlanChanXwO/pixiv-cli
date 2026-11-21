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
	"sync"
	"testing"
	"time"

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

func TestPacingSharesCooldownAndCancellation(t *testing.T) {
	var mutex sync.Mutex
	var starts []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		starts = append(starts, time.Now())
		mutex.Unlock()
		if r.URL.Path == "/limited" {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
		}
	}))
	defer server.Close()
	client, _ := api.NewHTTPClient(server.Client(), 30*time.Millisecond)
	response, err := client.Get(server.URL + "/limited")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/canceled", nil)
	require.NoError(t, err)
	_, err = client.Do(request)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			response, err := client.Get(server.URL)
			if err != nil {
				t.Error(err)
				return
			}
			response.Body.Close()
		})
	}
	workers.Wait()
	require.Len(t, starts, 3)
	require.GreaterOrEqual(t, starts[1].Sub(starts[0]), time.Second)
	require.GreaterOrEqual(t, starts[2].Sub(starts[1]), 25*time.Millisecond)
}

func TestReadRateLimitRetriesOnce(t *testing.T) {
	for _, after := range []string{"0", "invalid"} {
		calls := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.Header().Set("Retry-After", after)
			w.WriteHeader(429)
		}))
		client := server.Client()
		transport := client.Transport.(*http.Transport)
		transport.TLSClientConfig.ServerName = server.Listener.Addr().(*net.TCPAddr).IP.String()
		transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		}
		paced, _ := api.NewHTTPClient(client, time.Millisecond)
		var value any
		err := api.New(api.Config{HTTPClient: paced, AccessToken: "fixture"}).GetJSON(t.Context(), api.PathSearchIllust, nil, &value)
		require.Error(t, err)
		var failure api.Failure
		require.ErrorAs(t, err, &failure)
		require.Equal(t, 429, failure.StatusCode)
		expected := 1
		if after == "0" {
			expected = 2
		}
		require.Equal(t, expected, calls)
		server.Close()
	}
}
