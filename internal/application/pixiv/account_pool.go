package pixiv

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// ErrAccountPoolExhausted 表示配置的账号均已冻结或均已在本次安全重放中尝试。
// 调用方可保留导致冻结的 *pixiv.Error 作为 error chain，以便维持稳定分类。
var ErrAccountPoolExhausted = errors.New("account pool has no available account")

// AccountPoolStateStore 让账号选择事务与 authdb 的具体实现隔离。
// Select 必须在持久状态中原子记录选中的 UID，Freeze 必须原子写入截止时间。
type AccountPoolStateStore interface {
	Select(context.Context, time.Time, config.AccountPoolStrategy, []int64) (int64, error)
	Freeze(context.Context, int64, time.Time, time.Time) error
}

// AccountPoolExecutor 将“选择本地受管账号”和“仅在安全边界前重放”收敛在 application。
// 它不暴露给 public SDK，也不保存凭据。
type AccountPoolExecutor struct {
	Config config.AccountPoolConfig
	State  AccountPoolStateStore
	Now    func() time.Time
}

// Run 依次执行可用账号。attempt 的 committed=true 表示已向调用方暴露记录或已提交
// 下载文件；此后即使收到 429 也不能切换账号或重放。
func (e AccountPoolExecutor) Run(ctx context.Context, attempt func(context.Context, int64) (committed bool, err error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !e.Config.Enabled {
		return errors.New("account pool is disabled")
	}
	if e.State == nil {
		return errors.New("account pool state store is not configured")
	}
	if attempt == nil {
		return errors.New("account pool attempt is not configured")
	}
	now := time.Now
	if e.Now != nil {
		now = e.Now
	}
	attempted := make([]int64, 0)
	attemptedSet := make(map[int64]struct{})
	var lastRateLimit error
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := now()
		userID, err := e.State.Select(ctx, current, e.Config.Strategy, attempted)
		if err != nil {
			return mapPoolSelectionError(err, lastRateLimit, current)
		}
		if userID <= 0 {
			return errors.New("account pool state store selected an invalid user id")
		}
		diagnostics.Emit(ctx, diagnostics.Event{
			Module:    diagnostics.ModulePixivAccount,
			Kind:      diagnostics.EventAccount,
			Operation: "selected",
			Resource:  "uid " + strconv.FormatInt(userID, 10),
		})
		if _, wasAttempted := attemptedSet[userID]; wasAttempted {
			return fmt.Errorf("account pool state store selected attempted user id %d", userID)
		}
		committed, attemptErr := attempt(ctx, userID)
		if attemptErr == nil {
			return nil
		}
		if committed {
			return attemptErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		retryAfter, ok := poolRetryAfter(attemptErr, current)
		if !ok {
			return attemptErr
		}
		lastRateLimit = attemptErr
		attempted = append(attempted, userID)
		attemptedSet[userID] = struct{}{}
		freezeNow := now()
		if err := e.State.Freeze(ctx, userID, freezeNow.Add(retryAfter), freezeNow); err != nil {
			return err
		}
		diagnostics.Emit(ctx, diagnostics.Event{
			Module:    diagnostics.ModulePixivAccount,
			Kind:      diagnostics.EventAccount,
			Operation: "froze",
			Resource:  "uid " + strconv.FormatInt(userID, 10),
			Reason:    diagnostics.ReasonAccountFrozen,
		})
	}
}

func poolRetryAfter(err error, now time.Time) (time.Duration, bool) {
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

func mapPoolSelectionError(err, lastRateLimit error, now time.Time) error {
	var selectionErr *PoolSelectionError
	if !errors.As(err, &selectionErr) {
		if errors.Is(err, ErrAccountPoolExhausted) {
			return poolExhaustedError(lastRateLimit, nil, now)
		}
		return sdk.NewError("pixiv", "account_pool", sdk.LocalStateError,
			sdk.WithDetail("account_pool_state_error"), sdk.WithCause(err))
	}
	switch selectionErr.Kind {
	case PoolSelectionNoLocalAccount:
		return sdk.NewError("pixiv", "account_pool", sdk.Unauthorized, sdk.WithDetail("account_pool_no_local_account"))
	case PoolSelectionNoSchedulable:
		return sdk.NewError("pixiv", "account_pool", sdk.LocalStateError, sdk.WithDetail("account_pool_no_schedulable_account"))
	case PoolSelectionAllFrozen:
		return poolSelectionRateLimit("account_pool_all_frozen", selectionErr.EarliestFrozenUntil, nil, now)
	case PoolSelectionExhausted:
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

type accountPoolExhaustedError struct {
	classified *sdk.Error
}

func (e *accountPoolExhaustedError) Error() string { return e.classified.Error() }

func (e *accountPoolExhaustedError) Unwrap() []error {
	return []error{ErrAccountPoolExhausted, e.classified}
}
