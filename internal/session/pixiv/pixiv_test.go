package pixiv_test

import (
	"context"
	"errors"
	"testing"
	"time"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
	session "github.com/FlanChanXwO/pixiv-cli/internal/session"
	sessionpixiv "github.com/FlanChanXwO/pixiv-cli/internal/session/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/stretchr/testify/require"
)

type fakePoolSelection struct {
	userID int64
	err    error
}

type fakePoolState struct {
	selections []fakePoolSelection
	selected   [][]int64
	frozen     []int64
	until      []int64
	selectAt   int
}

func (s *fakePoolState) SelectPixiv(_ context.Context, _ int64, attempted []int64, _ accountpixiv.Chooser) (accountpixiv.Account, error) {
	s.selected = append(s.selected, append([]int64(nil), attempted...))
	if s.selectAt >= len(s.selections) {
		return accountpixiv.Account{}, sessionpixiv.ErrAccountPoolExhausted
	}
	selection := s.selections[s.selectAt]
	s.selectAt++
	if selection.err != nil {
		return accountpixiv.Account{}, selection.err
	}
	return accountpixiv.New(selection.userID, "user", []byte("token")), nil
}

func (s *fakePoolState) FreezePooledPixiv(_ context.Context, userID, until int64) error {
	s.frozen = append(s.frozen, userID)
	s.until = append(s.until, until)
	return nil
}

func scheduler(state *fakePoolState, now time.Time) sessionpixiv.Scheduler {
	return sessionpixiv.Scheduler{
		Config: config.AccountPoolConfig{Enabled: true, Strategy: config.AccountPoolStrategyRoundRobin},
		State:  state,
		Now:    func() time.Time { return now },
	}
}

func retryError(now time.Time) error {
	return sdk.NewError("pixiv", "read", sdk.RateLimited, sdk.WithRetry(sdk.RetryAdvice{Safe: true, After: now.Add(3 * time.Second), HasAfter: true}))
}

func TestSchedulerFailsOverOnlyBeforeCommit(t *testing.T) {
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	state := &fakePoolState{selections: []fakePoolSelection{{userID: 11}, {userID: 22}}}
	attempts := make([]int64, 0, 2)
	err := scheduler(state, now).Run(context.Background(), func(_ context.Context, userID int64, _ *session.Attempt) error {
		attempts = append(attempts, userID)
		if userID == 11 {
			return retryError(now)
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []int64{11, 22}, attempts)
	require.Equal(t, [][]int64{nil, {11}}, state.selected)
	require.Equal(t, []int64{11}, state.frozen)
	require.Equal(t, []int64{now.Add(3 * time.Second).Unix()}, state.until)

	state = &fakePoolState{selections: []fakePoolSelection{{userID: 11}, {userID: 22}}}
	committed := errors.New("download failed after commit")
	err = scheduler(state, now).Run(context.Background(), func(_ context.Context, _ int64, attempt *session.Attempt) error {
		attempt.Commit()
		return committed
	})
	require.ErrorIs(t, err, committed)
	require.Equal(t, 1, state.selectAt)
	require.Empty(t, state.frozen)
}

func TestSchedulerRequiresSafeFutureRetryAfter(t *testing.T) {
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	for _, retry := range []sdk.RetryAdvice{
		{After: now.Add(time.Second), HasAfter: true},
		{Safe: true, After: now, HasAfter: true},
		{Safe: true, After: now.Add(-time.Second), HasAfter: true},
	} {
		state := &fakePoolState{selections: []fakePoolSelection{{userID: 11}}}
		err := scheduler(state, now).Run(context.Background(), func(context.Context, int64, *session.Attempt) error {
			return sdk.NewError("pixiv", "read", sdk.RateLimited, sdk.WithRetry(retry))
		})
		require.Error(t, err)
		require.Empty(t, state.frozen)
	}
}

func TestSchedulerMapsSelectionReasons(t *testing.T) {
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		selection  *accountpixiv.PoolSelectionError
		wantReason sdk.Reason
		wantDetail string
		wantRetry  bool
	}{
		{name: "no local", selection: &accountpixiv.PoolSelectionError{Kind: accountpixiv.PoolSelectionNoLocalAccount}, wantReason: sdk.Unauthorized, wantDetail: "account_pool_no_local_account"},
		{name: "no schedulable", selection: &accountpixiv.PoolSelectionError{Kind: accountpixiv.PoolSelectionNoSchedulable}, wantReason: sdk.LocalStateError, wantDetail: "account_pool_no_schedulable_account"},
		{name: "frozen", selection: &accountpixiv.PoolSelectionError{Kind: accountpixiv.PoolSelectionAllFrozen, EarliestFrozenUntil: ptrInt64(now.Add(time.Minute).Unix())}, wantReason: sdk.RateLimited, wantDetail: "account_pool_all_frozen", wantRetry: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			state := &fakePoolState{selections: []fakePoolSelection{{err: test.selection}}}
			err := scheduler(state, now).Run(context.Background(), func(context.Context, int64, *session.Attempt) error { t.Fatal("attempt must not run"); return nil })
			var typed *sdk.Error
			require.ErrorAs(t, err, &typed)
			require.Equal(t, test.wantReason, typed.Reason)
			require.Equal(t, test.wantDetail, typed.Detail)
			require.Equal(t, test.wantRetry, typed.Retry.HasAfter)
		})
	}
}

func TestSchedulerPreservesLastRateLimitOnExhaustion(t *testing.T) {
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	earliest := now.Add(2 * time.Minute).Unix()
	state := &fakePoolState{selections: []fakePoolSelection{{userID: 11}, {err: &accountpixiv.PoolSelectionError{Kind: accountpixiv.PoolSelectionExhausted, EarliestFrozenUntil: &earliest}}}}
	last := retryError(now)
	err := scheduler(state, now).Run(context.Background(), func(context.Context, int64, *session.Attempt) error { return last })
	require.ErrorIs(t, err, sessionpixiv.ErrAccountPoolExhausted)
	require.ErrorIs(t, err, last)
	var typed *sdk.Error
	require.ErrorAs(t, err, &typed)
	require.Equal(t, "account_pool_exhausted", typed.Detail)
	require.Equal(t, time.Unix(earliest, 0), typed.Retry.After)
}

func TestChooseIsPureAndValidatesRandomIndex(t *testing.T) {
	snapshot := accountpixiv.PoolSnapshot{
		Candidates:   []accountpixiv.PoolCandidate{{UserID: 1, SortOrder: 1}, {UserID: 2, SortOrder: 2}},
		MarkerUserID: ptrInt64(1), MarkerSortOrder: ptrInt64(1),
	}
	selected, err := sessionpixiv.Choose(snapshot, config.AccountPoolStrategyRoundRobin, nil)
	require.NoError(t, err)
	require.Equal(t, int64(2), selected)
	selected, err = sessionpixiv.Choose(snapshot, config.AccountPoolStrategyRandom, func(int) (int, error) { return 0, nil })
	require.NoError(t, err)
	require.Equal(t, int64(1), selected)
	_, err = sessionpixiv.Choose(snapshot, config.AccountPoolStrategyRandom, func(int) (int, error) { return 9, nil })
	require.Error(t, err)
}

func ptrInt64(value int64) *int64 { return &value }
