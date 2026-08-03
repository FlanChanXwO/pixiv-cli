package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// ErrAccountPoolExhausted 表示配置的账号均已冻结或均已在本次安全重放中尝试。
// 调用方可保留导致冻结的 *pixiv.Error 作为 error chain，以便维持稳定分类。
var ErrAccountPoolExhausted = errors.New("account pool has no available account")

// AccountPoolStateStore 让账号选择事务与具体的私有 JSON/文件锁实现隔离。
// Lease 必须在持久状态中原子记录选中的 UID，Freeze 必须原子写入截止时间。
type AccountPoolStateStore interface {
	Lease(context.Context, []int64, []int64, config.AccountPoolStrategy, time.Time) (int64, error)
	Freeze(context.Context, int64, time.Time, time.Time) error
}

// AccountPoolExecutor 将“选择本地受管账号”和“仅在安全边界前重放”收敛在 application。
// 它不暴露给 public SDK，也不保存凭据。
type AccountPoolExecutor struct {
	Config            config.AccountPoolConfig
	State             AccountPoolStateStore
	AvailableAccounts func(context.Context) ([]int64, error)
	Now               func() time.Time
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
	if e.AvailableAccounts == nil {
		return errors.New("account pool local account loader is not configured")
	}
	if attempt == nil {
		return errors.New("account pool attempt is not configured")
	}
	available, err := e.AvailableAccounts(ctx)
	if err != nil {
		return err
	}
	configured := append([]int64(nil), e.Config.Accounts...)
	if len(configured) == 0 {
		configured = append(configured, available...)
	}
	if err := validatePoolAccounts(configured, available); err != nil {
		return err
	}
	now := time.Now
	if e.Now != nil {
		now = e.Now
	}
	tried := make(map[int64]struct{}, len(configured))
	var lastRateLimit error
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		candidates := make([]int64, 0, len(configured)-len(tried))
		for _, userID := range configured {
			if _, wasTried := tried[userID]; !wasTried {
				candidates = append(candidates, userID)
			}
		}
		if len(candidates) == 0 {
			return poolExhaustedError(lastRateLimit)
		}
		userID, err := e.State.Lease(ctx, configured, candidates, e.Config.Strategy, now())
		if err != nil {
			if errors.Is(err, ErrAccountPoolExhausted) {
				return poolExhaustedError(lastRateLimit)
			}
			return err
		}
		committed, attemptErr := attempt(ctx, userID)
		if attemptErr == nil {
			return nil
		}
		if committed || ctx.Err() != nil {
			return attemptErr
		}
		retryAfter, ok := poolRetryAfter(attemptErr, now())
		if !ok {
			return attemptErr
		}
		lastRateLimit = attemptErr
		tried[userID] = struct{}{}
		if err := e.State.Freeze(ctx, userID, now().Add(retryAfter), now()); err != nil {
			return err
		}
	}
}

func validatePoolAccounts(configured, available []int64) error {
	for _, userID := range configured {
		if !slices.Contains(available, userID) {
			return fmt.Errorf("account pool account %d is not available in local auth store", userID)
		}
	}
	return nil
}

func poolRetryAfter(err error, now time.Time) (time.Duration, bool) {
	var typed *sdk.Error
	if !errors.As(err, &typed) || typed.Code != sdk.CodeRateLimited || !typed.Retry.HasAfter {
		return 0, false
	}
	retryAfter := typed.Retry.After.Sub(now)
	if retryAfter < 0 {
		retryAfter = 0
	}
	return retryAfter, true
}

// PoolRetryAfter 从错误中提取有效的 Retry-After 建议，供账号池安全重放使用。
func PoolRetryAfter(err error) (time.Duration, bool) {
	return poolRetryAfter(err, time.Now())
}

func poolExhaustedError(lastRateLimit error) error {
	if lastRateLimit == nil {
		return ErrAccountPoolExhausted
	}
	return fmt.Errorf("%w: %w", ErrAccountPoolExhausted, lastRateLimit)
}
