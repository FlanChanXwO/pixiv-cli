package fanbox_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestCreatorsReportUpstreamFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantReason sdk.Reason
	}{
		{"empty list", `{"body":[]}`, http.StatusOK, ""},
		{"missing list", `{}`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"null list", `{"body":null}`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"wrong list type", `{"body":{"plans":true}}`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"invalid JSON", `{`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"trailing JSON", `{"body":[]} {}`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"upstream failure", `{}`, http.StatusServiceUnavailable, sdk.UpstreamError},
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
			require.Equal(t, tc.wantReason, sdk.ReasonOf(err))
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
	for _, value := range []string{"session-canary; extra=injected-canary", "session-canary\r\n"} {
		_, err := fanbox.Open(fanbox.SessionCredentials{FANBOXSESSID: value})
		require.Error(t, err)
		require.NotContains(t, fmt.Sprint(err), "session-canary")
	}
}

func TestResponsesReportMalformedData(t *testing.T) {
	for _, tc := range []struct {
		operation, body string
	}{
		{"CreatorTags", `{"body":{}}`},
		{"CurrentUser", `<meta name="metadata" content='{"context":{"user":{"name":"artist"}}}'>`},
		{"CreatorPosts", `{"body":{}}`},
		{"CreatorPosts", `{"body":{"posts":[],"nextUrl":" "}}`},
		{"CreatorPosts", `{"body":{"posts":[{"id":"p1","publishedDatetime":"response-token-canary"}]}}`},
		{"Creator", `{"body":{"creatorId":"artist","user":{}}}`},
	} {
		api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, tc.body)
		}))
		httpClient := api.Client()
		transport := httpClient.Transport.(*http.Transport)
		transport.TLSClientConfig.ServerName = api.Listener.Addr().(*net.TCPAddr).IP.String()
		transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, api.Listener.Addr().String())
		}
		client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-canary"}, fanbox.Options{HTTPClient: httpClient})
		require.NoError(t, err)
		switch tc.operation {
		case "CreatorPosts":
			_, err = client.CreatorPosts(t.Context(), fanbox.CreatorPostsRequest{CreatorID: "artist"})
		case "Creator":
			_, err = client.Creator(t.Context(), fanbox.CreatorRequest{CreatorID: "artist"})
		case "CurrentUser":
			_, err = client.CurrentUser(t.Context(), fanbox.CurrentUserRequest{})
		case "CreatorTags":
			_, err = client.CreatorTags(t.Context(), fanbox.CreatorTagsRequest{CreatorID: "artist"})
		}
		api.Close()
		var failure *sdk.Error
		require.ErrorAs(t, err, &failure)
		require.Equal(t, tc.operation, failure.Operation)
		require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
		require.NotContains(t, fmt.Sprint(err), "response-token-canary")
	}
}

func TestSaveResourcePropagatesCancellation(t *testing.T) {
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "downloads.fanbox.cc" {
			fmt.Fprint(w, "file bytes")
			return
		}
		fmt.Fprint(w, `{"body":{"post":{"id":"p1","publishedDatetime":"2026-01-01T00:00:00Z","body":{"files":[{"id":"f1","url":"https://downloads.fanbox.cc/file.bin"}]}}}}`)
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
	post, err := client.Post(t.Context(), fanbox.PostRequest{PostID: "p1"})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "file.bin")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, err = client.SaveResource(ctx, post.Body.Assets[0].Resource.Ref, sdk.SaveOptions{Path: path, Progress: func(sdk.SaveProgress) { cancel() }})
	require.ErrorIs(t, err, context.Canceled)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "original", string(data))
}
