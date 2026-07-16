//go:build !windows

package files

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePrivateFileReturnsParentSyncFailureAfterReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), constants.PrivateFileMode))
	ops := defaultPrivateFileOps()
	ops.syncParent = func(string) error {
		return errors.New("parent sync failed")
	}

	err := writePrivateFile(path, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorContains(t, err, "parent sync failed")

	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "new", string(body))
	assertNoPrivateFileTemporaries(t, dir)
}
