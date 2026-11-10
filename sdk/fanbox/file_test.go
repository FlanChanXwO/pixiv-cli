package fanbox_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

type closeErrorTransport struct{ base http.RoundTripper }

func (r closeErrorTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := r.base.RoundTrip(request)
	if err == nil && request.URL.Hostname() == "downloads.fanbox.cc" {
		response.Body = closeErrorBody{response.Body}
	}
	return response, err
}

type closeErrorBody struct{ io.ReadCloser }

func (b closeErrorBody) Close() error {
	return errors.Join(b.ReadCloser.Close(), errors.New("secret-fixture"))
}

func TestFileSaveCloseFailure(t *testing.T) {
	for _, failClose := range []bool{false, true} {
		t.Run(fmt.Sprint(failClose), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/post.info" {
					fmt.Fprint(w, `{"body":{"post":{"id":"42","publishedDatetime":"2026-11-10T00:00:00Z","body":{"files":[{"id":"f","url":"https://downloads.fanbox.cc/file"}]}}}}`)
					return
				}
				fmt.Fprint(w, "new bytes")
			}))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			if failClose {
				httpClient.Transport = closeErrorTransport{httpClient.Transport}
			}
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			post, err := client.Post("42").Get(t.Context())
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "file")
			require.NoError(t, os.WriteFile(path, []byte("old bytes"), 0o600))
			err = post.Body.Files[0].Save(t.Context(), path)
			want := "new bytes"
			if failClose {
				want = "old bytes"
				require.Equal(t, sdk.UpstreamUnavailable, sdk.ReasonOf(err))
				require.NotContains(t, err.Error(), "secret-fixture")
			} else {
				require.NoError(t, err)
			}
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, want, string(data))
		})
	}
}

func TestFileSaveCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/post.info" {
			fmt.Fprint(w, `{"body":{"post":{"id":"42","publishedDatetime":"2026-11-10T00:00:00Z","body":{"files":[{"id":"f","url":"https://downloads.fanbox.cc/file"}]}}}}`)
			return
		}
		w.Header().Set("Content-Length", "100")
		fmt.Fprint(w, "partial")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	post, err := client.Post("42").Get(t.Context())
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(path, []byte("old bytes"), 0o600))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- post.Body.Files[0].Save(ctx, path) }()
	<-started
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "old bytes", string(data))
}
