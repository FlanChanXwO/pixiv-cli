package pixiv

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/stretchr/testify/require"
)

func TestImageFailures(t *testing.T) {
	for _, mode := range []string{"normal", "upstream", "malformed", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "upstream" {
					w.WriteHeader(403)
				}
				if mode == "malformed" {
					w.Header().Set("Content-Length", "100")
				}
				fmt.Fprint(w, "bytes")
			}))
			defer server.Close()
			value := NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := New(store, Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			image, err := client.image("https://i.pximg.net/fixture.jpg", 0, 0)
			require.NoError(t, err)
			dest := filepath.Join(t.TempDir(), "image.jpg")
			require.NoError(t, os.WriteFile(dest, []byte("existing"), 0600))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			err = image.Save(ctx, dest)
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
	value := NewCredentials("fixture-refresh")
	value.SetTokens("fixture-access", "fixture-refresh")
	value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
	store, err := NewMemoryCredentials(value)
	require.NoError(t, err)
	client, err := New(store, Options{HTTPClient: &http.Client{Transport: cancelMedia{cancel}}})
	require.NoError(t, err)
	image, err := client.image("https://i.pximg.net/fixture.jpg", 0, 0)
	require.NoError(t, err)
	dest := filepath.Join(t.TempDir(), "image.jpg")
	require.NoError(t, os.WriteFile(dest, []byte("existing"), 0600))
	assertImageResult(t, "cancel", image.Save(ctx, dest), dest)
}

type cancelMedia struct{ cancel context.CancelFunc }

func (c cancelMedia) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: c}, nil
}

func (c cancelMedia) Read([]byte) (int, error) {
	c.cancel()
	return 0, io.EOF
}

func (cancelMedia) Close() error { return nil }
