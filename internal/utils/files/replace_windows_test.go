//go:build windows

package files

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplaceFileWithBackupPreservesOldTargetForDeferredCleanup(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "new.exe")
	target := filepath.Join(directory, "pixiv.exe")
	backup := target + ".old"
	require.NoError(t, os.WriteFile(source, []byte("new"), 0o600))
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))

	require.NoError(t, ReplaceFileWithBackup(source, target, backup))
	installed, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "new", string(installed))
	old, err := os.ReadFile(backup)
	require.NoError(t, err)
	require.Equal(t, "old", string(old))
	_, err = os.Stat(source)
	require.ErrorIs(t, err, os.ErrNotExist)
}
