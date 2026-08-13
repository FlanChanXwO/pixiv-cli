package lock_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/file/lock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithPrivateLockRunsActionAndCreatesSidecar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	called := false
	require.NoError(t, lock.WithPrivateLock(context.Background(), path, func() error {
		called = true
		return nil
	}))
	assert.True(t, called)
	_, err := os.Stat(path + ".lock")
	require.NoError(t, err)
}

func TestWithPrivateLockReturnsContextAndActionErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, lock.WithPrivateLock(ctx, filepath.Join(t.TempDir(), "state"), func() error { return nil }), context.Canceled)

	actionErr := errors.New("action failed")
	require.ErrorIs(t, lock.WithPrivateLock(context.Background(), filepath.Join(t.TempDir(), "state"), func() error { return actionErr }), actionErr)
	require.ErrorContains(t, lock.WithPrivateLock(context.Background(), filepath.Join(t.TempDir(), "state"), nil), "action is not configured")
}
