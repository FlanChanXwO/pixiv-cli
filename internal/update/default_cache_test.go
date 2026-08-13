package update_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/FlanChanXwO/pixiv-cli/internal/update/release"
	"github.com/stretchr/testify/require"
)

func TestNewGitHubReleaseClientDefaultCacheUsesApplicationDataDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"tag_name":"v1.0.0","draft":false,"prerelease":false}]`)
	}))
	defer server.Close()

	client, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{APIBaseURL: server.URL})
	require.NoError(t, err)
	result, err := client.Check(context.Background(), update.ReleaseCheckOptions{})
	require.NoError(t, err)
	require.NotNil(t, result.Release)

	cachePath := filepath.Join(home, localstate.AppDataDirName, "cache", release.CacheFilename)
	cacheBytes, err := os.ReadFile(cachePath)
	require.NoError(t, err)
	require.Contains(t, string(cacheBytes), `"schema_version":2`)
}
