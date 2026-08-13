package installer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/update/release"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteReleaseCachePreservesNewSourceWhenReplacementRecoveryIsUnresolved(t *testing.T) {
	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, release.CacheFilename)
	fileCache := NewFileReleaseCache(cacheDir, cachePath).(*fileReleaseCache)
	require.NoError(t, os.WriteFile(cachePath, []byte("old cache"), 0o600))
	var source string
	replaceCause := errors.New("replacement recovery unresolved")
	fileCache.replaceFile = func(sourcePath, _ string) error {
		source = sourcePath
		return preserveInstallerSourceError{err: replaceCause}
	}

	err := fileCache.Write(context.Background(), []byte(`{"schema_version":2,"checked_at":"2026-07-11T12:00:00Z"}`+"\n"))
	require.ErrorIs(t, err, replaceCause)

	oldBody, readErr := os.ReadFile(cachePath)
	require.NoError(t, readErr)
	assert.Equal(t, "old cache", string(oldBody))
	newBody, readErr := os.ReadFile(source)
	require.NoError(t, readErr)
	assert.Contains(t, string(newBody), `"schema_version":2`)
	assert.Equal(t, cacheDir, filepath.Dir(source))
}

func TestWriteReleaseCacheCleansNewSourceAfterOrdinaryReplacementFailure(t *testing.T) {
	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, release.CacheFilename)
	fileCache := NewFileReleaseCache(cacheDir, cachePath).(*fileReleaseCache)
	require.NoError(t, os.WriteFile(cachePath, []byte("old cache"), 0o600))
	var source string
	replaceCause := errors.New("replacement unchanged")
	fileCache.replaceFile = func(sourcePath, _ string) error {
		source = sourcePath
		return replaceCause
	}

	err := fileCache.Write(context.Background(), []byte(`{"schema_version":2,"checked_at":"2026-07-11T12:00:00Z"}`+"\n"))
	require.ErrorIs(t, err, replaceCause)
	_, statErr := os.Stat(source)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	oldBody, readErr := os.ReadFile(cachePath)
	require.NoError(t, readErr)
	assert.Equal(t, "old cache", string(oldBody))
}
