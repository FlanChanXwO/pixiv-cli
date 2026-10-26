// Package pool 实现 Pixiv 账号池的选择、冻结与安全重放策略。
//
// 账号凭据和持久化事务由 account leaf 提供；本包只编排候选选择、
// attempt 排除、冻结时间和可安全重放的错误。
package pool

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
)

// ErrAccountPoolExhausted 表示自然候选集合已耗尽。
var ErrAccountPoolExhausted = errors.New("account pool has no available account")

// PoolState 是数据库实现的最窄 scheduler port。选择器只接收事务快照，
// scheduler 不持有 SQL 或 marker 状态。
type PoolState interface {
	SelectPixiv(context.Context, int64, []int64, pixivaccount.Chooser) (pixivaccount.Account, error)
	Freeze(context.Context, int64, int64) error
}

// Scheduler 负责账号池 strategy、attempt exclusion、freeze 与安全 replay。
type Scheduler struct {
	Config settings.AccountPoolConfig
	State  PoolState
	Now    func() time.Time
}

// Run 只有在当前 Attempt 未 commit 且现有 SDK retry advice 明确允许 replay
// 时才切换账号；不设置固定重试次数，候选耗尽自然结束。
func (s Scheduler) Run(ctx context.Context, attempt func(context.Context, int64, *lifecycle.Attempt) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	attempted := make([]int64, 0)
	var lastRateLimit error
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := s.Now()
		account, err := s.State.SelectPixiv(ctx, current.UTC().Unix(), attempted, func(snapshot pixivaccount.PoolSnapshot) (int64, error) {
			return choose(snapshot, s.Config.Strategy)
		})
		if err != nil {
			return mapSelectionError(err, lastRateLimit, current)
		}
		userID := account.UserID
		attemptState := &lifecycle.Attempt{}
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
		freezeNow := s.Now()
		if err := s.State.Freeze(ctx, userID, freezeNow.Add(retryAfter).UTC().Unix()); err != nil {
			return err
		}
		logging.From(ctx).WarnContext(ctx, "Account pool froze rate-limited account",
			slog.String(logging.AttrReason, "account frozen"))
	}
}
