package update

import (
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/filesystem"
	"github.com/stretchr/testify/require"
)

func TestGitHubReleaseClientDefaultCacheUsesApplicationDataDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	client, err := NewGitHubReleaseClient(ReleaseClientOptions{})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, filesystem.AppDataDirName, "cache"), client.cacheDir)
	require.Equal(t, filepath.Join(home, filesystem.AppDataDirName, "cache", releaseCacheFilename), client.cachePath)
}
