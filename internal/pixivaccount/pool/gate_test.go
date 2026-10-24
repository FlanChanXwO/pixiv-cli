package pool_test

import (
	"context"
	"testing"

	poolpixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount/pool"
	"github.com/stretchr/testify/require"
)

func TestGateSerializesAndReleases(t *testing.T) {
	gate := poolpixiv.NewGate()
	require.NoError(t, gate.Acquire(context.Background()))
	gate.Release()
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
}
