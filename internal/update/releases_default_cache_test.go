package update

import (
	"path/filepath"
	"testing"

	constants "github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/stretchr/testify/require"
)

func TestGitHubReleaseClientDefaultCacheUsesApplicationDataDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	client, err := NewGitHubReleaseClient(ReleaseClientOptions{})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, constants.AppDataDirName, "cache"), client.cacheDir)
	require.Equal(t, filepath.Join(home, constants.AppDataDirName, "cache", releaseCacheFilename), client.cachePath)
}
