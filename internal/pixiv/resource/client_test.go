package resource

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientsPreserveHeadersStatusAndCopy(t *testing.T) {
	tests := []struct {
		name               string
		newClient          func(*http.Client) *Client
		referer, userAgent string
		accept             string
	}{
		{"app", NewApp, AppReferer, AppUserAgent, ""},
		{"web", func(h *http.Client) *Client { return NewWeb(h, "https://www.pixiv.net/") }, "https://www.pixiv.net/", WebUserAgent, "application/json,text/plain,*/*"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.referer, r.Header.Get("Referer"))
				assert.Equal(t, tc.userAgent, r.Header.Get("User-Agent"))
				assert.Equal(t, tc.accept, r.Header.Get("Accept"))
				fmt.Fprint(w, "image")
			}))
			defer server.Close()
			var dst bytes.Buffer
			require.NoError(t, tc.newClient(server.Client()).Download(context.Background(), server.URL, &dst))
			assert.Equal(t, "image", dst.String())
		})
	}
}

func TestAppDownloadExposesNonSuccessStatusWithoutBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "blocked", http.StatusForbidden) }))
	defer server.Close()
	err := NewApp(server.Client()).Download(context.Background(), server.URL, &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
	assert.NotContains(t, err.Error(), "blocked")
}

func TestWebDownloadPreservesNonSuccessBodyPolicy(t *testing.T) {
	t.Run("non-empty body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "blocked", http.StatusForbidden)
		}))
		defer server.Close()
		err := NewWeb(server.Client(), server.URL).Download(context.Background(), server.URL, &bytes.Buffer{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "403")
		assert.Contains(t, err.Error(), "blocked")
	})

	t.Run("empty body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer server.Close()
		err := NewWeb(server.Client(), server.URL).Download(context.Background(), server.URL, &bytes.Buffer{})
		require.Error(t, err)
		assert.Equal(t, "download failed: 403 Forbidden", err.Error())
	})
}

func TestWebDownloadReportsErrorBodyReadFailureAndClosesBody(t *testing.T) {
	readErr := errors.New("read failed")
	body := &trackingBody{Reader: errorReader{err: readErr}}
	client := NewWeb(&http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Status: "403 Forbidden", Body: body}, nil
	})}, "https://www.pixiv.net")

	err := client.Download(context.Background(), "https://i.pximg.net/a.jpg", io.Discard)

	require.ErrorIs(t, err, readErr)
	assert.Contains(t, err.Error(), "download failed: 403 Forbidden: read error body")
	assert.True(t, body.closed)
}

func TestDownloadClosesResponseBody(t *testing.T) {
	body := &trackingBody{Reader: strings.NewReader("image")}
	client := NewApp(&http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: body}, nil
	})})
	require.NoError(t, client.Download(context.Background(), "https://i.pximg.net/a.jpg", io.Discard))
	assert.True(t, body.closed)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackingBody struct {
	io.Reader
	closed bool
}

func (b *trackingBody) Close() error { b.closed = true; return nil }

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
