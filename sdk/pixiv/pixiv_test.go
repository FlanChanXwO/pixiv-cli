package pixiv_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestSearchReportsUpstreamFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantError  bool
	}{
		{"empty list", `{"illusts":[],"next_url":null}`, http.StatusOK, false},
		{"missing list", `{}`, http.StatusOK, true},
		{"wrong list type", `{"illusts":true}`, http.StatusOK, true},
		{"invalid JSON", `{`, http.StatusOK, true},
		{"upstream failure", `{}`, http.StatusServiceUnavailable, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer api.Close()
			httpClient := api.Client()
			transport := httpClient.Transport.(*http.Transport)
			transport.TLSClientConfig.ServerName = api.Listener.Addr().(*net.TCPAddr).IP.String()
			transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, api.Listener.Addr().String())
			}
			client, err := pixiv.NewWith("access-canary", pixiv.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			page, err := client.SearchArtworks(t.Context(), pixiv.SearchArtworksRequest{Word: "cat"})
			require.Equal(t, tc.wantError, err != nil)
			require.Empty(t, page.Items)
			if tc.wantError {
				require.NotEmpty(t, sdk.ReasonOf(err))
			}
		})
	}
}

func TestSearchPropagatesCancellation(t *testing.T) {
	started := make(chan struct{})
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer api.Close()
	httpClient := api.Client()
	transport := httpClient.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = api.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, api.Listener.Addr().String())
	}
	client, err := pixiv.NewWith("access-canary", pixiv.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := client.SearchArtworks(ctx, pixiv.SearchArtworksRequest{Word: "cat"})
		result <- err
	}()
	<-started
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
}

func TestOpenCredentialsStayPrivate(t *testing.T) {
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":"access-canary","refresh_token":"refresh-canary","expires_in":3600,"user":{"id":"42","name":"artist"}}`)
	}))
	defer api.Close()
	httpClient := api.Client()
	transport := httpClient.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = api.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, api.Listener.Addr().String())
	}
	client, credentials, err := pixiv.OpenWith(t.Context(), "initial-refresh", pixiv.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	require.EqualValues(t, 42, client.UserID())
	require.Equal(t, "access-canary", credentials.AccessToken())
	require.Equal(t, "refresh-canary", credentials.RefreshToken())
	var output bytes.Buffer
	slog.New(slog.NewJSONHandler(&output, nil)).Info("credentials", slog.Any("credentials", credentials))
	for _, format := range []string{"%v", "%+v", "%#v"} {
		fmt.Fprintf(&output, format, credentials)
	}
	require.Contains(t, output.String(), "artist")
	require.NotContains(t, output.String(), "access-canary")
	require.NotContains(t, output.String(), "refresh-canary")
}
