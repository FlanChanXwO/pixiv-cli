package pixiv

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type preserveReplacementSourceError struct {
	err error
}

func (e preserveReplacementSourceError) Error() string            { return e.err.Error() }
func (e preserveReplacementSourceError) Unwrap() error            { return e.err }
func (preserveReplacementSourceError) PreserveReplacementSource() {}

func TestDownloadPreservesNewSourceWhenReplacementRecoveryIsUnresolved(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("new image"))
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	client, err := NewClient(NewClientOptions{
		HTTPClient:     server.Client(),
		ResourcePolicy: ResourcePolicy{Mirrors: []ResourceMirrorPolicy{{Host: parsed.Host, PathPrefixes: []string{"/resource/"}}}},
	})
	require.NoError(t, err)
	ref, err := client.ParseResourceRef(server.URL + "/resource/image.jpg")
	require.NoError(t, err)

	destination := filepath.Join(t.TempDir(), "image.jpg")
	require.NoError(t, os.WriteFile(destination, []byte("old image"), 0o600))
	var source string
	replaceCause := errors.New("replacement recovery unresolved")
	err = client.downloadWithReplacer(context.Background(), ref, destination, func(sourcePath, _ string) error {
		source = sourcePath
		return preserveReplacementSourceError{err: replaceCause}
	})
	require.Error(t, err)

	oldBody, readErr := os.ReadFile(destination)
	require.NoError(t, readErr)
	assert.Equal(t, "old image", string(oldBody))
	newBody, readErr := os.ReadFile(source)
	require.NoError(t, readErr)
	assert.Equal(t, "new image", string(newBody))
}
