package pixiv

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadResourceResumesVerifiedPartialWithIfRange(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("ETag", `"v1"`)
		if requests == 1 {
			w.Header().Set("Content-Length", "10")
			_, _ = io.WriteString(w, "1234")
			return
		}
		assert.Equal(t, "bytes=4-", r.Header.Get("Range"))
		assert.Equal(t, `"v1"`, r.Header.Get("If-Range"))
		w.Header().Set("Content-Range", "bytes 4-9/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "567890")
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	client, err := NewClient(NewClientOptions{
		HTTPClient:     server.Client(),
		ResourcePolicy: ResourcePolicy{Mirrors: []ResourceMirrorPolicy{{Host: parsed.Host, PathPrefixes: []string{"/resource/"}}}},
	})
	require.NoError(t, err)
	ref, err := client.ParseResourceRef(server.URL + "/resource/file.bin")
	require.NoError(t, err)
	destination := filepath.Join(t.TempDir(), "file.bin")

	_, err = client.DownloadResource(context.Background(), ref, destination)
	require.Error(t, err)
	_, statErr := os.Stat(destination)
	require.ErrorIs(t, statErr, os.ErrNotExist)

	result, err := client.DownloadResource(context.Background(), ref, destination)
	require.NoError(t, err)
	assert.Equal(t, ResourceDownloadCacheResumed, result.CacheState)
	body, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, "1234567890", string(body))
}
