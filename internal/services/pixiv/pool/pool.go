// Package pool 实现 Pixiv 账号池的选择、冻结与安全重放策略。
//
// 账号凭据和持久化事务由 account leaf 提供；本包只编排候选选择、
// attempt 排除、冻结时间和可安全重放的错误。
package pool

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
)

// ErrAccountPoolExhausted 表示自然候选集合已耗尽。
var ErrAccountPoolExhausted = errors.New("account pool has no available account")

// PoolState 是数据库实现的最窄 scheduler port。选择器只接收事务快照，
// scheduler 不持有 SQL 或 marker 状态。
type PoolState interface {
	SelectPixiv(context.Context, int64, []int64, accountpixiv.Chooser) (accountpixiv.Account, error)
	Freeze(context.Context, int64, int64) error
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
func (s Scheduler) Run(ctx context.Context, attempt func(context.Context, int64, *lifecycle.Attempt) error) error {
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
		diagnostics.Emit(ctx, diagnostics.Event{
			Module:    diagnostics.ModulePixivAccount,
			Kind:      diagnostics.EventAccount,
			Operation: "selected",
			Resource:  "uid " + strconv.FormatInt(userID, 10),
		})
		if _, wasAttempted := attemptedSet[userID]; wasAttempted {
			return fmt.Errorf("account pool state store selected attempted user id %d", userID)
		}
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
		attemptedSet[userID] = struct{}{}
		freezeNow := now()
		if err := s.State.Freeze(ctx, userID, freezeNow.Add(retryAfter).UTC().Unix()); err != nil {
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
