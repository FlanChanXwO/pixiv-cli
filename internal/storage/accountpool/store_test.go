package accountpool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/stretchr/testify/require"
)

func TestStoreLeasesAndFreezesWithoutPersistingSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "account-pool.json")
	store := Store{Path: func() (string, error) { return path, nil }}
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)

	first, err := store.Lease(context.Background(), []int64{11, 22}, []int64{11, 22}, config.AccountPoolStrategyRoundRobin, now)
	require.NoError(t, err)
	require.Equal(t, int64(11), first)
	require.NoError(t, store.Freeze(context.Background(), 11, now.Add(time.Minute), now))
	second, err := store.Lease(context.Background(), []int64{11, 22}, []int64{11, 22}, config.AccountPoolStrategyRoundRobin, now)
	require.NoError(t, err)
	require.Equal(t, int64(22), second)

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var state State
	require.NoError(t, json.Unmarshal(body, &state))
	require.Equal(t, int64(22), state.LastUserID)
	require.Equal(t, now.Add(time.Minute), state.FrozenUntil[11])
	require.NotContains(t, string(body), "token")
	require.NotContains(t, string(body), "version")

	_, err = store.Lease(context.Background(), []int64{22}, []int64{22}, config.AccountPoolStrategyRoundRobin, now)
	require.NoError(t, err)
	body, err = os.ReadFile(path)
	require.NoError(t, err)
	state = State{}
	require.NoError(t, json.Unmarshal(body, &state))
	require.Empty(t, state.FrozenUntil, "removed configured accounts are pruned from state")
}

func TestStoreReturnsPoolExhaustedWhenEveryCandidateIsFrozen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "account-pool.json")
	store := Store{Path: func() (string, error) { return path, nil }}
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Freeze(context.Background(), 11, now.Add(time.Minute), now))
	_, err := store.Lease(context.Background(), []int64{11}, []int64{11}, config.AccountPoolStrategyRoundRobin, now)
	require.ErrorIs(t, err, application.ErrAccountPoolExhausted)
}

func TestStoreFreezeKeepsTheLongestActiveRetryAfter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "account-pool.json")
	store := Store{Path: func() (string, error) { return path, nil }}
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	longer := now.Add(2 * time.Minute)
	require.NoError(t, store.Freeze(context.Background(), 11, longer, now))
	require.NoError(t, store.Freeze(context.Background(), 11, now.Add(time.Minute), now))

	var state State
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &state))
	require.Equal(t, longer, state.FrozenUntil[11])
}
