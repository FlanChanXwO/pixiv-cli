package filelock_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/filelock"
	"github.com/stretchr/testify/require"
)

func TestPrivateLockExcludesWritersAndReleasesAfterFailure(t *testing.T) {
	target := filepath.Join(t.TempDir(), "state")
	abort := errors.New("write aborted")
	err := filelock.WithPrivateLock(t.Context(), target, func() error {
		release, acquired, err := filelock.TryPrivateLock(target)
		require.NoError(t, err)
		require.False(t, acquired)
		require.Nil(t, release)
		return abort
	})
	require.ErrorIs(t, err, abort)
	release, acquired, err := filelock.TryPrivateLock(target)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, release())
}
