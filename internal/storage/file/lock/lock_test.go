package lock_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/file/lock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type checkedLockContext struct {
	context.Context
	checked chan struct{}
	once    sync.Once
}

func (c *checkedLockContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.checked) })
	return err
}

func TestPrivateLockCancellationDoesNotWaitForHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	release, acquired, err := lock.TryPrivateLock(path)
	require.NoError(t, err)
	require.True(t, acquired)
	defer release()
	inner, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &checkedLockContext{Context: inner, checked: make(chan struct{})}
	var called atomic.Bool
	done := make(chan error, 1)
	go func() { done <- lock.WithPrivateLock(ctx, path, func() error { called.Store(true); return nil }) }()
	<-ctx.checked
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancelled waiter remained blocked by the lock holder")
	}
	require.False(t, called.Load(), "cancelled waiter must not run its transaction")
	second, acquired, err := lock.TryPrivateLock(path)
	require.NoError(t, err)
	require.False(t, acquired, "cancelling a waiter must not unlock the holder")
	require.Nil(t, second)
}

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

func TestTryPrivateLockDetectsLiveOwnerAndReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance")
	release, acquired, err := lock.TryPrivateLock(path)
	require.NoError(t, err)
	require.True(t, acquired)
	second, acquired, err := lock.TryPrivateLock(path)
	require.NoError(t, err)
	require.False(t, acquired)
	require.Nil(t, second)
	require.NoError(t, release())
	release, acquired, err = lock.TryPrivateLock(path)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, release())
}
