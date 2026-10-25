package fanbox_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestCreatorsReportUpstreamFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantError  bool
	}{
		{"empty list", `{"body":[]}`, http.StatusOK, false},
		{"missing list", `{}`, http.StatusOK, true},
		{"wrong list type", `{"body":{"plans":true}}`, http.StatusOK, true},
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
			client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-canary"}, fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			page, err := client.Creators(t.Context(), fanbox.CreatorsRequest{Kind: fanbox.CreatorListSupporting})
			require.Equal(t, tc.wantError, err != nil)
			require.Empty(t, page.Items)
		})
	}
}

func TestCreatorsPropagateCancellation(t *testing.T) {
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
	client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-canary"}, fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := client.Creators(ctx, fanbox.CreatorsRequest{Kind: fanbox.CreatorListSupporting})
		result <- err
	}()
	<-started
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
}

func TestSessionCredentialsStayPrivate(t *testing.T) {
	credentials := fanbox.SessionCredentials{FANBOXSESSID: "session-canary"}
	var output bytes.Buffer
	slog.New(slog.NewJSONHandler(&output, nil)).Info("credentials", slog.Any("credentials", credentials))
	for _, format := range []string{"%v", "%+v", "%#v"} {
		fmt.Fprintf(&output, format, credentials)
	}
	require.Contains(t, output.String(), "credentials")
	require.NotContains(t, output.String(), credentials.FANBOXSESSID)
}
