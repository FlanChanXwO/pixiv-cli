package pool_test

import (
	"context"
	"errors"
	"testing"

	poolpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/pool"
	"github.com/stretchr/testify/require"
)

func TestGateSerializesAndReleasesAfterCallback(t *testing.T) {
	gate := poolpixiv.NewGate()
	called := false
	err := gate.Run(context.Background(), func(context.Context) error {
		called = true
		return errors.New("callback failed")
	})
	require.EqualError(t, err, "callback failed")
	require.True(t, called)
	require.NoError(t, gate.Acquire(context.Background()))
	gate.Release()
}

func TestGateAcquireHonorsCancellationWhileOccupied(t *testing.T) {
	gate := poolpixiv.NewGate()
	require.NoError(t, gate.Acquire(context.Background()))
	defer gate.Release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := gate.Acquire(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestGateRejectsMissingConfiguration(t *testing.T) {
	var gate *poolpixiv.Gate
	require.EqualError(t, gate.Acquire(context.Background()), "pixiv rotation gate is not configured")
	require.EqualError(t, gate.Run(context.Background(), func(context.Context) error { return nil }), "pixiv rotation gate is not configured")
	require.EqualError(t, poolpixiv.NewGate().Run(context.Background(), nil), "pixiv rotation gate function is nil")
}
