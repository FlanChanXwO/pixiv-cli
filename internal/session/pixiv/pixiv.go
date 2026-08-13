// Package pixiv 拥有 Pixiv account pool、replay policy、freeze 与 rotation
// serialization；数据库只负责事务和持久状态，不负责 strategy。
package pixiv

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"time"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// ErrAccountPoolExhausted 表示自然候选集合已耗尽。
var ErrAccountPoolExhausted = errors.New("account pool has no available account")

// PoolState 是数据库实现的最窄 scheduler port。SelectPixiv 的 chooser 在
// database transaction 内执行，scheduler 不持有 SQL 或 marker 状态。
type PoolState interface {
	SelectPixiv(context.Context, int64, []int64, accountpixiv.Chooser) (accountpixiv.Account, error)
	FreezePooledPixiv(context.Context, int64, int64) error
}

// Scheduler 负责账号池 strategy、attempt exclusion、freeze 与安全 replay。
type Scheduler struct {
	Config config.AccountPoolConfig
	State  PoolState
	Now    func() time.Time
	Random func(int) (int, error)
}

// Run 只有在当前 Attempt 未 commit 且现有 SDK retry advice 明确允许 replay
// 时才切换账号；不设置固定重试次数，候选耗尽自然结束。
func (s Scheduler) Run(ctx context.Context, attempt func(context.Context, int64, *session.Attempt) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.Config.Enabled {
		return errors.New("account pool is disabled")
	}
	if s.State == nil {
		return errors.New("account pool state store is not configured")
	}
	if attempt == nil {
		return errors.New("account pool attempt is not configured")
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	attempted := make([]int64, 0)
	attemptedSet := make(map[int64]struct{})
	var lastRateLimit error
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := now()
		account, err := s.State.SelectPixiv(ctx, current.UTC().Unix(), attempted, func(snapshot accountpixiv.PoolSnapshot) (int64, error) {
			return Choose(snapshot, s.Config.Strategy, s.Random)
		})
		if err != nil {
			return mapSelectionError(err, lastRateLimit, current)
		}
		userID := account.UserID
		if userID <= 0 {
			return errors.New("account pool state store selected an invalid user id")
		}
		diagnostics.Emit(ctx, diagnostics.Event{Module: diagnostics.ModulePixivAccount, Kind: diagnostics.EventAccount, Operation: "selected", Resource: "uid " + strconv.FormatInt(userID, 10)})
		if _, wasAttempted := attemptedSet[userID]; wasAttempted {
			return fmt.Errorf("account pool state store selected attempted user id %d", userID)
		}
		attemptState := &session.Attempt{}
		attemptErr := attempt(ctx, userID, attemptState)
		if attemptErr == nil {
			return nil
		}
		if attemptState.Committed() {
			return attemptErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		retryAfter, ok := retryAfterFor(attemptErr, current)
		if !ok {
			return attemptErr
		}
		lastRateLimit = attemptErr
		attempted = append(attempted, userID)
		attemptedSet[userID] = struct{}{}
		freezeNow := now()
		if err := s.State.FreezePooledPixiv(ctx, userID, freezeNow.Add(retryAfter).UTC().Unix()); err != nil {
			return err
		}
		diagnostics.Emit(ctx, diagnostics.Event{Module: diagnostics.ModulePixivAccount, Kind: diagnostics.EventAccount, Operation: "froze", Resource: "uid " + strconv.FormatInt(userID, 10), Reason: diagnostics.ReasonAccountFrozen})
	}
}

// Choose 是无 IO 的 round-robin/random chooser。Database 会在提交前再次验证
// 返回 UID 属于候选快照。
func Choose(snapshot accountpixiv.PoolSnapshot, strategy config.AccountPoolStrategy, random func(int) (int, error)) (int64, error) {
	if len(snapshot.Candidates) == 0 {
		return 0, &accountpixiv.PoolSelectionError{Kind: accountpixiv.PoolSelectionExhausted, EarliestFrozenUntil: cloneInt64(snapshot.EarliestFrozenUntil)}
	}
	switch strategy {
	case config.AccountPoolStrategyRoundRobin:
		if snapshot.MarkerSortOrder != nil {
			for _, candidate := range snapshot.Candidates {
				if candidate.SortOrder > *snapshot.MarkerSortOrder {
					return candidate.UserID, nil
				}
			}
		}
		return snapshot.Candidates[0].UserID, nil
	case config.AccountPoolStrategyRandom:
		if random == nil {
			random = randomIndex
		}
		index, err := random(len(snapshot.Candidates))
		if err != nil {
			return 0, err
		}
		if index < 0 || index >= len(snapshot.Candidates) {
			return 0, errors.New("pixiv account pool random source returned an invalid index")
		}
		return snapshot.Candidates[index].UserID, nil
	default:
		return 0, fmt.Errorf("unsupported account pool strategy %q", strategy)
	}
}

func randomIndex(size int) (int, error) {
	if size <= 0 {
		return 0, errors.New("pixiv account pool has no eligible account")
	}
	value, err := rand.Int(rand.Reader, big.NewInt(int64(size)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

func retryAfterFor(err error, now time.Time) (time.Duration, bool) {
	var typed *sdk.Error
	if !errors.As(err, &typed) || typed.Reason != sdk.RateLimited || !typed.Retry.Safe || !typed.Retry.HasAfter {
		return 0, false
	}
	retryAfter := typed.Retry.After.Sub(now)
	if retryAfter <= 0 {
		return 0, false
	}
	return retryAfter, true
}

func mapSelectionError(err, lastRateLimit error, now time.Time) error {
	var selectionErr *accountpixiv.PoolSelectionError
	if !errors.As(err, &selectionErr) {
		if errors.Is(err, ErrAccountPoolExhausted) {
			return poolExhaustedError(lastRateLimit, nil, now)
		}
		return sdk.NewError("pixiv", "account_pool", sdk.LocalStateError, sdk.WithDetail("account_pool_state_error"), sdk.WithCause(err))
	}
	switch selectionErr.Kind {
	case accountpixiv.PoolSelectionNoLocalAccount:
		return sdk.NewError("pixiv", "account_pool", sdk.Unauthorized, sdk.WithDetail("account_pool_no_local_account"))
	case accountpixiv.PoolSelectionNoSchedulable:
		return sdk.NewError("pixiv", "account_pool", sdk.LocalStateError, sdk.WithDetail("account_pool_no_schedulable_account"))
	case accountpixiv.PoolSelectionAllFrozen:
		return poolSelectionRateLimit("account_pool_all_frozen", selectionErr.EarliestFrozenUntil, nil, now)
	case accountpixiv.PoolSelectionExhausted:
		return poolExhaustedError(lastRateLimit, selectionErr.EarliestFrozenUntil, now)
	default:
		return fmt.Errorf("account pool state store returned unknown selection kind %q", selectionErr.Kind)
	}
}

func poolSelectionRateLimit(detail string, earliest *int64, cause error, now time.Time) error {
	opts := []sdk.ErrorOption{sdk.WithDetail(detail)}
	if retry, ok := poolRetryAdvice(earliest, now); ok {
		opts = append(opts, sdk.WithRetry(retry))
	}
	if cause != nil {
		opts = append(opts, sdk.WithCause(cause))
	}
	return sdk.NewError("pixiv", "account_pool", sdk.RateLimited, opts...)
}

func poolRetryAdvice(earliest *int64, now time.Time) (sdk.RetryAdvice, bool) {
	if earliest == nil {
		return sdk.RetryAdvice{}, false
	}
	after := time.Unix(*earliest, 0)
	if !after.After(now) {
		return sdk.RetryAdvice{}, false
	}
	return sdk.RetryAdvice{Safe: true, After: after, HasAfter: true}, true
}

func poolExhaustedError(lastRateLimit error, earliest *int64, now time.Time) error {
	if lastRateLimit == nil {
		return ErrAccountPoolExhausted
	}
	classified := poolSelectionRateLimit("account_pool_exhausted", earliest, lastRateLimit, now)
	var typed *sdk.Error
	if !errors.As(classified, &typed) {
		return fmt.Errorf("%w: %w", ErrAccountPoolExhausted, classified)
	}
	return &accountPoolExhaustedError{classified: typed}
}

type accountPoolExhaustedError struct{ classified *sdk.Error }

func (e *accountPoolExhaustedError) Error() string { return e.classified.Error() }

func (e *accountPoolExhaustedError) Unwrap() []error {
	return []error{ErrAccountPoolExhausted, e.classified}
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

// Gate serializes operations that may consume the same Pixiv refresh token.
// It is product session state, not an MCP server global.
type Gate struct{ slots chan struct{} }

// NewGate creates a gate with one slot.
func NewGate() *Gate { return &Gate{slots: make(chan struct{}, 1)} }

// Acquire waits for the serialization slot and is paired with Release.
func (g *Gate) Acquire(ctx context.Context) error {
	if g == nil || g.slots == nil {
		return errors.New("pixiv rotation gate is not configured")
	}
	select {
	case g.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release releases a slot acquired by Acquire.
func (g *Gate) Release() {
	if g != nil && g.slots != nil {
		<-g.slots
	}
}

// Run executes fn while holding the rotation serialization slot.
func (g *Gate) Run(ctx context.Context, fn func(context.Context) error) error {
	if g == nil || g.slots == nil {
		return errors.New("pixiv rotation gate is not configured")
	}
	if fn == nil {
		return errors.New("pixiv rotation gate function is nil")
	}
	if err := g.Acquire(ctx); err != nil {
		return err
	}
	defer g.Release()
	return fn(ctx)
}

// RotateCredential performs the identity check and revision CAS at the product
// session boundary. The database remains the only writer of credential rows.
func RotateCredential(ctx context.Context, repository interface {
	RotatePixivCredentials(context.Context, int64, int64, []byte) error
}, selectedUserID, authenticatedUserID, revision int64, refreshToken []byte) error {
	if err := VerifyAccountIdentity(selectedUserID, authenticatedUserID); err != nil {
		return err
	}
	if repository == nil {
		return errors.New("pixiv credential repository is not configured")
	}
	return repository.RotatePixivCredentials(ctx, selectedUserID, revision, refreshToken)
}

// VerifyAccountIdentity rejects a credential whose authenticated UID does not
// match the selected local account before rotation or content requests.
func VerifyAccountIdentity(selectedUserID, authenticatedUserID int64) error {
	if selectedUserID <= 0 || authenticatedUserID <= 0 || selectedUserID != authenticatedUserID {
		return sdk.NewError("pixiv", "OpenAccountClient", sdk.LocalStateError, sdk.WithDetail("credential identity does not match selected account"))
	}
	return nil
}
