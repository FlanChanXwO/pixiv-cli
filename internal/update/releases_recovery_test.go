package update

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteReleaseCachePreservesNewSourceWhenReplacementRecoveryIsUnresolved(t *testing.T) {
	cacheDir := t.TempDir()
	client, err := NewGitHubReleaseClient(ReleaseClientOptions{CacheDir: cacheDir})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(client.cachePath, []byte("old cache"), 0o600))
	var source string
	replaceCause := errors.New("replacement recovery unresolved")
	client.replaceFile = func(sourcePath, _ string) error {
		source = sourcePath
		return preserveInstallerSourceError{err: replaceCause}
	}

	err = client.writeCache(releaseCache{SchemaVersion: releaseCacheSchemaVersion, CheckedAt: time.Now()})
	require.ErrorIs(t, err, replaceCause)

	oldBody, readErr := os.ReadFile(client.cachePath)
	require.NoError(t, readErr)
	assert.Equal(t, "old cache", string(oldBody))
	newBody, readErr := os.ReadFile(source)
	require.NoError(t, readErr)
	assert.Contains(t, string(newBody), `"schema_version":2`)
	assert.Equal(t, cacheDir, filepath.Dir(source))
}

func TestWriteReleaseCacheCleansNewSourceAfterOrdinaryReplacementFailure(t *testing.T) {
	cacheDir := t.TempDir()
	client, err := NewGitHubReleaseClient(ReleaseClientOptions{CacheDir: cacheDir})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(client.cachePath, []byte("old cache"), 0o600))
	var source string
	replaceCause := errors.New("replacement unchanged")
	client.replaceFile = func(sourcePath, _ string) error {
		source = sourcePath
		return replaceCause
	}

	err = client.writeCache(releaseCache{SchemaVersion: releaseCacheSchemaVersion, CheckedAt: time.Now()})
	require.ErrorIs(t, err, replaceCause)
	_, statErr := os.Stat(source)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	oldBody, readErr := os.ReadFile(client.cachePath)
	require.NoError(t, readErr)
	assert.Equal(t, "old cache", string(oldBody))
}
