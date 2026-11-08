package pixiv_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestImageFailures(t *testing.T) {
	for _, mode := range []string{"normal", "upstream", "malformed", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == api.PathIllustDetail {
					fmt.Fprint(w, artworkResponse)
					return
				}
				if mode == "upstream" {
					w.WriteHeader(403)
				}
				if mode == "malformed" {
					w.Header().Set("Content-Length", "100")
				}
				fmt.Fprint(w, "bytes")
			}))
			defer server.Close()
			value := pixiv.NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			art, err := client.Artwork(23).Get(t.Context())
			require.NoError(t, err)
			dest := filepath.Join(t.TempDir(), "image.jpg")
			require.NoError(t, os.WriteFile(dest, []byte("existing"), 0600))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			err = art.Pages[0].Original.Save(ctx, dest)
			assertImageResult(t, mode, err, dest)
		})
	}
}

func assertImageResult(t *testing.T, mode string, err error, dest string) {
	t.Helper()

	switch mode {
	case "normal":
		require.NoError(t, err)
	case "upstream":
		require.Equal(t, sdk.Forbidden, sdk.ReasonOf(err))
	case "malformed":
		require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
	case "cancel":
		require.ErrorIs(t, err, context.Canceled)
	}
	data, readErr := os.ReadFile(dest)
	require.NoError(t, readErr)
	if mode == "normal" {
		require.Equal(t, "bytes", string(data))
	} else {
		require.Equal(t, "existing", string(data))
	}
}

func TestImageSaveCancellationBeforeReplace(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	value := pixiv.NewCredentials("fixture-refresh")
	value.SetTokens("fixture-access", "fixture-refresh")
	value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
	store, err := pixiv.NewMemoryCredentials(value)
	require.NoError(t, err)
	client, err := pixiv.New(store, pixiv.Options{HTTPClient: &http.Client{Transport: cancelMedia{cancel}}})
	require.NoError(t, err)
	art, err := client.Artwork(23).Get(t.Context())
	require.NoError(t, err)
	dest := filepath.Join(t.TempDir(), "image.jpg")
	require.NoError(t, os.WriteFile(dest, []byte("existing"), 0600))
	assertImageResult(t, "cancel", art.Pages[0].Original.Save(ctx, dest), dest)
}

type cancelMedia struct{ cancel context.CancelFunc }

func (c cancelMedia) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == api.PathIllustDetail {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(artworkResponse))}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: c}, nil
}

func (c cancelMedia) Read([]byte) (int, error) {
	c.cancel()
	return 0, io.EOF
}

func (cancelMedia) Close() error { return nil }

func fixtureHTTP(server *httptest.Server) *http.Client {
	client := server.Client()
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = server.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	return client
}
