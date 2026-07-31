package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/stretchr/testify/require"
)

type fakeAccountPoolStateStore struct {
	leases  []int64
	frozen  []int64
	until   []time.Time
	leaseAt int
}

func (s *fakeAccountPoolStateStore) Lease(_ context.Context, _ []int64, _ []int64, _ config.AccountPoolStrategy, _ time.Time) (int64, error) {
	if s.leaseAt >= len(s.leases) {
		return 0, ErrAccountPoolExhausted
	}
	userID := s.leases[s.leaseAt]
	s.leaseAt++
	return userID, nil
}

func (s *fakeAccountPoolStateStore) Freeze(_ context.Context, userID int64, until, _ time.Time) error {
	s.frozen = append(s.frozen, userID)
	s.until = append(s.until, until)
	return nil
}

func TestAccountPoolExecutorFailsOverOnlyBeforeCommit(t *testing.T) {
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	state := &fakeAccountPoolStateStore{leases: []int64{11, 22}}
	executor := AccountPoolExecutor{
		Config: config.AccountPoolConfig{Enabled: true, Accounts: []int64{11, 22}, Strategy: config.AccountPoolStrategyRoundRobin},
		State:  state,
		AvailableAccounts: func(context.Context) ([]int64, error) {
			return []int64{11, 22}, nil
		},
		Now: func() time.Time { return now },
	}

	attempts := make([]int64, 0, 2)
	err := executor.Run(context.Background(), func(_ context.Context, userID int64) (bool, error) {
		attempts = append(attempts, userID)
		if userID == 11 {
			return false, &sdk.Error{Code: sdk.CodeRateLimited, HasRetryAfter: true, RetryAfter: 3 * time.Second}
		}
		return false, nil
	})
	require.NoError(t, err)
	require.Equal(t, []int64{11, 22}, attempts)
	require.Equal(t, []int64{11}, state.frozen)
	require.Equal(t, []time.Time{now.Add(3 * time.Second)}, state.until)

	state = &fakeAccountPoolStateStore{leases: []int64{11, 22}}
	executor.State = state
	committed := errors.New("download failed after file commit")
	err = executor.Run(context.Background(), func(_ context.Context, userID int64) (bool, error) {
		return true, committed
	})
	require.ErrorIs(t, err, committed)
	require.Equal(t, 1, state.leaseAt, "a committed attempt must not switch accounts")
	require.Empty(t, state.frozen)

	state = &fakeAccountPoolStateStore{leases: []int64{11, 22}}
	executor.State = state
	rateLimitedAfterStdoutAttempt := &sdk.Error{Code: sdk.CodeRateLimited, HasRetryAfter: true}
	err = executor.Run(context.Background(), func(context.Context, int64) (bool, error) {
		return true, rateLimitedAfterStdoutAttempt
	})
	require.ErrorIs(t, err, rateLimitedAfterStdoutAttempt)
	require.Equal(t, 1, state.leaseAt, "a typed 429 after stdout/file commit must not switch accounts")
	require.Empty(t, state.frozen)
}

func TestAccountPoolExecutorValidatesAllConfiguredAccountsBeforeAttempts(t *testing.T) {
	executor := AccountPoolExecutor{
		Config: config.AccountPoolConfig{Enabled: true, Accounts: []int64{11, 22}, Strategy: config.AccountPoolStrategyRoundRobin},
		State:  &fakeAccountPoolStateStore{leases: []int64{11, 22}},
		AvailableAccounts: func(context.Context) ([]int64, error) {
			return []int64{11}, nil
		},
	}
	called := false
	err := executor.Run(context.Background(), func(context.Context, int64) (bool, error) {
		called = true
		return false, nil
	})
	require.EqualError(t, err, "account pool account 22 is not available in local auth store")
	require.False(t, called)
}

func TestAccountPoolExecutorUsesLocalStorageOrderWithoutWhitelistAndLogsRotation(t *testing.T) {
	now := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)
	state := &fakeAccountPoolStateStore{leases: []int64{22, 11}}
	executor := AccountPoolExecutor{
		Config: config.AccountPoolConfig{Enabled: true}, State: state,
		AvailableAccounts: func(context.Context) ([]int64, error) { return []int64{22, 11}, nil },
		Now:               func() time.Time { return now },
	}
	attempts := make([]int64, 0, 2)
	err := executor.Run(context.Background(), func(_ context.Context, userID int64) (bool, error) {
		attempts = append(attempts, userID)
		if userID == 22 {
			return false, &sdk.Error{Code: sdk.CodeRateLimited, HasRetryAfter: true, RetryAfter: time.Second}
		}
		return false, nil
	})
	require.NoError(t, err)
	require.Equal(t, []int64{22, 11}, attempts)
}
