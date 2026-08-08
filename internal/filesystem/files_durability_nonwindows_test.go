//go:build !windows

package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePrivateFileReturnsParentSyncFailureAfterReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), PrivateFileMode))
	ops := defaultPrivateFileOps()
	ops.syncParent = func(string) error {
		return errors.New("parent sync failed")
	}

	err := writePrivateFile(path, []byte("new"), PrivateFileMode, ops)
	require.ErrorContains(t, err, "parent sync failed")
	assert.Equal(t, WriteCommitOutcomeCommitted, PrivateFileWriteCommitOutcome(err))

	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "new", string(body))
	assertNoPrivateFileTemporaries(t, dir)
}

func TestWritePrivateFileSyncsSingleNewDirectoryLeafToRootAfterCommit(t *testing.T) {
	root := t.TempDir()
	targetDirectory := filepath.Join(root, "pixiv")
	path := filepath.Join(targetDirectory, "config.toml")
	var synced []string
	ops := defaultPrivateFileOps()
	ops.syncParent = func(path string) error {
		synced = append(synced, path)
		return nil
	}

	require.NoError(t, writePrivateFile(path, []byte("new"), PrivateFileMode, ops))
	assert.Equal(t, []string{targetDirectory, root}, synced)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(body))
}

func TestWritePrivateFileSyncsMultipleNewDirectoriesLeafToRootAfterCommit(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "pixiv")
	leaf := filepath.Join(first, "accounts")
	path := filepath.Join(leaf, "auth.json")
	var synced []string
	ops := defaultPrivateFileOps()
	ops.syncParent = func(path string) error {
		synced = append(synced, path)
		return nil
	}

	require.NoError(t, writePrivateFile(path, []byte("new"), PrivateFileMode, ops))
	assert.Equal(t, []string{leaf, first, root}, synced)
}

func TestWritePrivateFileSyncsOnlyTargetDirectoryWhenDirectoryAlreadyExists(t *testing.T) {
	targetDirectory := t.TempDir()
	path := filepath.Join(targetDirectory, "config.toml")
	var synced []string
	ops := defaultPrivateFileOps()
	ops.syncParent = func(path string) error {
		synced = append(synced, path)
		return nil
	}

	require.NoError(t, writePrivateFile(path, []byte("new"), PrivateFileMode, ops))
	assert.Equal(t, []string{targetDirectory}, synced)
}

func TestWritePrivateFileJoinsEveryPostCommitDirectorySyncFailure(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "pixiv")
	leaf := filepath.Join(first, "accounts")
	path := filepath.Join(leaf, "auth.json")
	leafErr := errors.New("leaf sync failed")
	firstErr := errors.New("first parent sync failed")
	rootErr := errors.New("outer parent sync failed")
	failures := map[string]error{leaf: leafErr, first: firstErr, root: rootErr}
	var synced []string
	ops := defaultPrivateFileOps()
	ops.syncParent = func(path string) error {
		synced = append(synced, path)
		return failures[path]
	}

	err := writePrivateFile(path, []byte("committed"), PrivateFileMode, ops)
	require.ErrorIs(t, err, leafErr)
	require.ErrorIs(t, err, firstErr)
	require.ErrorIs(t, err, rootErr)
	assert.Equal(t, []string{leaf, first, root}, synced)
	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "committed", string(body))
}
