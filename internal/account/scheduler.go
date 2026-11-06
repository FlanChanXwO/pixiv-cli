package account

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

// ErrAccountPoolExhausted 表示自然候选集合已耗尽。
var ErrAccountPoolExhausted = errors.New("account pool has no available account")

// Scheduler 按传入的策略选择账号、排除已尝试的账号、冻结与安全重放。
type Scheduler struct {
	Strategy Strategy
	State    *Store
	Now      func() time.Time
}

// Run 仅在本次尝试尚未提交且 SDK 的重试建议明确允许安全重放时切换池账号。
// 固定账号执行一次，池账号在合格候选耗尽后返回错误。
func (s Scheduler) Run(ctx context.Context, selection Selection, attempt func(context.Context, int64, *lifecycle.Attempt) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if selection.Mode == SelectionFixed {
		if selection.UserID <= 0 {
			return errors.New("account: fixed user id must be positive")
		}
		return attempt(ctx, selection.UserID, &lifecycle.Attempt{})
	}
	if selection.Mode != SelectionPool {
		return errors.New("account: invalid selection mode")
	}
	attempted := make([]int64, 0)
	var lastRateLimit error
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := s.Now()
		selected, err := s.State.SelectPixiv(ctx, current.UTC().Unix(), attempted, func(snapshot PoolSnapshot) (int64, error) {
			return choose(snapshot, s.Strategy)
		})
		if err != nil {
			return mapSelectionError(err, lastRateLimit, current)
		}
		userID := selected.UserID
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
