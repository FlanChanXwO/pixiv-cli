// Pixiv 账号池的调度状态查询和 membership 写入。
// 事务内的状态读取在 repository.go 的 SelectPixiv。

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	"gorm.io/gorm"
)

func (d *DB) SetPixivSchedulable(ctx context.Context, userIDs []int64, schedulable bool) error {
	return d.withLock(ctx, func() error {
		if err := validatePoolUserIDs(userIDs); err != nil {
			return err
		}
		return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var count int64
			if err := tx.Model(&pixivAccountRow{}).Where(colUserID+" IN ?", userIDs).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(userIDs)) {
				return accountpixiv.ErrNotFound
			}
			return tx.Model(&pixivAccountRow{}).Where(colUserID+" IN ?", userIDs).
				Updates(map[string]any{colSchedulable: schedulable, colUpdatedAt: nowUnix()}).Error
		})
	})
}

// SetAllPixivSchedulable 在一个事务内更新当前所有 Pixiv 账号。
func (d *DB) SetAllPixivSchedulable(ctx context.Context, schedulable bool) error {
	return d.withLock(ctx, func() error {
		return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return tx.Model(&pixivAccountRow{}).
				Where(colUserID+" > ?", 0).
				Updates(map[string]any{colSchedulable: schedulable, colUpdatedAt: nowUnix()}).Error
		})
	})
}

// ListPixivPoolStatus 返回 non-secret 调度状态，并在同一个事务中清理已过期冻结。
func (d *DB) ListPixivPoolStatus(ctx context.Context, now int64) (accountpixiv.PoolStatus, error) {
	return withDatabaseValue(ctx, d, func() (accountpixiv.PoolStatus, error) {
		var status accountpixiv.PoolStatus
		err := d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&pixivAccountRow{}).
				Where(colPoolFrozenUntil+" IS NOT NULL AND "+colPoolFrozenUntil+" <= ?", now).
				Update(colPoolFrozenUntil, nil).Error; err != nil {
				return err
			}
			var rows []poolCandidateRow
			if err := tx.Table(tablePixivAccount).Select(colUserID, colSortOrder, colSchedulable, colPoolFrozenUntil, colPoolLastSelected).
				Order(colSortOrder).Find(&rows).Error; err != nil {
				return err
			}
			status.Accounts = make([]accountpixiv.PoolCandidate, 0, len(rows))
			for _, row := range rows {
				candidate := row.candidate(now)
				if candidate.PoolFrozenUntil != nil && (status.EarliestFrozenUntil == nil || *candidate.PoolFrozenUntil < *status.EarliestFrozenUntil) {
					value := *candidate.PoolFrozenUntil
					status.EarliestFrozenUntil = &value
				}
				status.Accounts = append(status.Accounts, candidate)
			}
			return nil
		})
		return status, err
	})
}

func validatePoolUserIDs(userIDs []int64) error {
	if len(userIDs) == 0 {
		return errors.New("database: account pool requires at least one user id")
	}
	return validatePoolUserIDsAllowEmpty(userIDs)
}

func validatePoolUserIDsAllowEmpty(userIDs []int64) error {
	seen := make(map[int64]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			return errors.New("database: account pool user id must be positive")
		}
		if _, exists := seen[userID]; exists {
			return fmt.Errorf("database: duplicate account pool user id %d", userID)
		}
		seen[userID] = struct{}{}
	}
	return nil
}

func nowUnix() int64 { return time.Now().UTC().Unix() }

func poolScope(now int64) (string, []any) {
	return colSchedulable + " = ? AND (" + colPoolFrozenUntil + " IS NULL OR " + colPoolFrozenUntil + " <= ?)", []any{true, now}
}

func earliestFrozenInTx(ctx context.Context, tx *gorm.DB, now int64) (*int64, error) {
	var rows []poolCandidateRow
	if err := tx.WithContext(ctx).Table(tablePixivAccount).Select(colPoolFrozenUntil).
		Where(colSchedulable+" = ? AND "+colPoolFrozenUntil+" > ?", true, now).
		Order(colPoolFrozenUntil).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 || rows[0].PoolFrozenUntil == nil {
		return nil, nil
	}
	return rows[0].PoolFrozenUntil, nil
}
