package pixiv

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/stretchr/testify/require"
)

type fakePoolSelection struct {
	userID int64
	err    error
}

type fakeAccountPoolStateStore struct {
	selections []fakePoolSelection
	selected   [][]int64
	frozen     []int64
	until      []time.Time
	selectAt   int
}

func (s *fakeAccountPoolStateStore) Select(_ context.Context, _ time.Time, _ config.AccountPoolStrategy, attempted []int64) (int64, error) {
	s.selected = append(s.selected, append([]int64(nil), attempted...))
	if s.selectAt >= len(s.selections) {
		return 0, ErrAccountPoolExhausted
	}
	selection := s.selections[s.selectAt]
	s.selectAt++
	return selection.userID, selection.err
}

func (s *fakeAccountPoolStateStore) Freeze(_ context.Context, userID int64, until, _ time.Time) error {
	s.frozen = append(s.frozen, userID)
	s.until = append(s.until, until)
	return nil
}

func TestAccountPoolExecutorFailsOverOnlyBeforeCommit(t *testing.T) {
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	state := &fakeAccountPoolStateStore{selections: []fakePoolSelection{{userID: 11}, {userID: 22}}}
	executor := AccountPoolExecutor{
		Config: config.AccountPoolConfig{Enabled: true, Strategy: config.AccountPoolStrategyRoundRobin},
		State:  state,
		Now:    func() time.Time { return now },
	}

	attempts := make([]int64, 0, 2)
	err := executor.Run(context.Background(), func(_ context.Context, userID int64) (bool, error) {
		attempts = append(attempts, userID)
		if userID == 11 {
			return false, sdk.NewError("pixiv", "read", sdk.RateLimited, sdk.WithRetry(sdk.RetryAdvice{Safe: true, After: now.Add(3 * time.Second), HasAfter: true}))
		}
		return false, nil
	})
	require.NoError(t, err)
	require.Equal(t, []int64{11, 22}, attempts)
	require.Equal(t, [][]int64{nil, []int64{11}}, state.selected)
	require.Equal(t, []int64{11}, state.frozen)
	require.Equal(t, []time.Time{now.Add(3 * time.Second)}, state.until)

	state = &fakeAccountPoolStateStore{selections: []fakePoolSelection{{userID: 11}, {userID: 22}}}
	executor.State = state
	committed := errors.New("download failed after file commit")
	err = executor.Run(context.Background(), func(context.Context, int64) (bool, error) {
		return true, committed
	})
	require.ErrorIs(t, err, committed)
	require.Equal(t, 1, state.selectAt, "a committed attempt must not switch accounts")
	require.Empty(t, state.frozen)
}

func TestAccountPoolExecutorRequiresSafeFutureRetryAfter(t *testing.T) {
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	cases := []sdk.RetryAdvice{
		{After: now.Add(time.Second), HasAfter: true},
		{Safe: true, After: now, HasAfter: true},
		{Safe: true, After: now.Add(-time.Second), HasAfter: true},
	}
	for index, retry := range cases {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			state := &fakeAccountPoolStateStore{selections: []fakePoolSelection{{userID: 11}}}
			executor := AccountPoolExecutor{
				Config: config.AccountPoolConfig{Enabled: true, Strategy: config.AccountPoolStrategyRoundRobin},
				State:  state,
				Now:    func() time.Time { return now },
			}
			want := sdk.NewError("pixiv", "read", sdk.RateLimited, sdk.WithRetry(retry))
			err := executor.Run(context.Background(), func(context.Context, int64) (bool, error) { return false, want })
			require.ErrorIs(t, err, want)
			require.Empty(t, state.frozen)
		})
	}
}

func TestAccountPoolExecutorMapsSelectionReasons(t *testing.T) {
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		selection  *PoolSelectionError
		wantReason sdk.Reason
		wantDetail string
		wantRetry  bool
	}{
		{name: "no local account", selection: &PoolSelectionError{Kind: PoolSelectionNoLocalAccount}, wantReason: sdk.Unauthorized, wantDetail: "account_pool_no_local_account"},
		{name: "no schedulable account", selection: &PoolSelectionError{Kind: PoolSelectionNoSchedulable}, wantReason: sdk.LocalStateError, wantDetail: "account_pool_no_schedulable_account"},
		{name: "all frozen", selection: &PoolSelectionError{Kind: PoolSelectionAllFrozen, EarliestFrozenUntil: ptrInt64(now.Add(time.Minute).Unix())}, wantReason: sdk.RateLimited, wantDetail: "account_pool_all_frozen", wantRetry: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			state := &fakeAccountPoolStateStore{selections: []fakePoolSelection{{err: test.selection}}}
			executor := AccountPoolExecutor{
				Config: config.AccountPoolConfig{Enabled: true, Strategy: config.AccountPoolStrategyRoundRobin},
				State:  state,
				Now:    func() time.Time { return now },
			}
			err := executor.Run(context.Background(), func(context.Context, int64) (bool, error) { t.Fatal("attempt must not run"); return false, nil })
			var typed *sdk.Error
			require.ErrorAs(t, err, &typed)
			require.Equal(t, test.wantReason, typed.Reason)
			require.Equal(t, test.wantDetail, typed.Detail)
			require.Equal(t, test.wantRetry, typed.Retry.HasAfter)
		})
	}
}

func TestAccountPoolExecutorMapsExhaustionAndPreservesLastRateLimit(t *testing.T) {
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	earliest := now.Add(2 * time.Minute).Unix()
	state := &fakeAccountPoolStateStore{selections: []fakePoolSelection{
		{userID: 11},
		{err: &PoolSelectionError{Kind: PoolSelectionExhausted, EarliestFrozenUntil: &earliest}},
	}}
	executor := AccountPoolExecutor{
		Config: config.AccountPoolConfig{Enabled: true, Strategy: config.AccountPoolStrategyRoundRobin},
		State:  state,
		Now:    func() time.Time { return now },
	}
	last := sdk.NewError("pixiv", "read", sdk.RateLimited, sdk.WithRetry(sdk.RetryAdvice{Safe: true, After: now.Add(time.Minute), HasAfter: true}))
	err := executor.Run(context.Background(), func(context.Context, int64) (bool, error) { return false, last })

	require.Error(t, err)
	require.ErrorIs(t, err, ErrAccountPoolExhausted)
	require.ErrorIs(t, err, last)
	var typed *sdk.Error
	require.ErrorAs(t, err, &typed)
	require.Equal(t, sdk.RateLimited, typed.Reason)
	require.Equal(t, "account_pool_exhausted", typed.Detail)
	require.True(t, typed.Retry.HasAfter)
	require.Equal(t, time.Unix(earliest, 0), typed.Retry.After)
}

func TestAccountPoolExecutorMapsStateReadFailureWithoutRetryAdvice(t *testing.T) {
	state := &fakeAccountPoolStateStore{selections: []fakePoolSelection{{err: errors.New("database is closed")}}}
	executor := AccountPoolExecutor{
		Config: config.AccountPoolConfig{Enabled: true, Strategy: config.AccountPoolStrategyRoundRobin},
		State:  state,
	}
	err := executor.Run(context.Background(), func(context.Context, int64) (bool, error) { t.Fatal("attempt must not run"); return false, nil })

	var typed *sdk.Error
	require.ErrorAs(t, err, &typed)
	require.Equal(t, sdk.LocalStateError, typed.Reason)
	require.Equal(t, "account_pool_state_error", typed.Detail)
	require.False(t, typed.Retry.HasAfter)
}

func TestAccountPoolExecutorReturnsContextCancellationWithoutRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	state := &fakeAccountPoolStateStore{selections: []fakePoolSelection{{userID: 11}, {userID: 22}}}
	executor := AccountPoolExecutor{
		Config: config.AccountPoolConfig{Enabled: true, Strategy: config.AccountPoolStrategyRoundRobin},
		State:  state,
	}
	err := executor.Run(ctx, func(context.Context, int64) (bool, error) {
		cancel()
		return false, errors.New("upstream interrupted")
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, state.selectAt)
	require.Empty(t, state.frozen)
}

func TestNewPooledSDKOperationKeepsFactoryAndAccountPoolInApplication(t *testing.T) {
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	state := &fakeAccountPoolStateStore{selections: []fakePoolSelection{{userID: 11}, {userID: 22}}}
	var requests []SDKClientRequest
	operation := NewPooledSDKOperation(PooledSDKOperationOptions{
		LoadRuntime: func() (config.RuntimeConfig, error) {
			return config.RuntimeConfig{AccountPool: config.AccountPoolConfig{Enabled: true, Strategy: config.AccountPoolStrategyRoundRobin}}, nil
		},
		State: state,
		Factory: func(request SDKClientRequest) (ClientSet, error) {
			requests = append(requests, request)
			return ClientSet{}, nil
		},
		Now: func() time.Time { return now },
	})

	err := operation(context.Background(), SDKClientRequest{HTTPSProxyOverride: stringPointer("http://proxy.invalid")}, func(context.Context, ClientSet) (bool, error) {
		if len(requests) == 1 {
			return false, sdk.NewError("pixiv", "read", sdk.RateLimited, sdk.WithRetry(sdk.RetryAdvice{Safe: true, After: now.Add(time.Second), HasAfter: true}))
		}
		return false, nil
	})
	require.NoError(t, err)
	require.Equal(t, []SDKClientRequest{
		{UserID: 11, HTTPSProxyOverride: stringPointer("http://proxy.invalid")},
		{UserID: 22, HTTPSProxyOverride: stringPointer("http://proxy.invalid")},
	}, requests)
}

func ptrInt64(value int64) *int64 { return &value }

func stringPointer(value string) *string { return &value }
