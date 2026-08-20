package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
)

func (d *DB) SetPixivSchedulable(ctx context.Context, userIDs []int64, schedulable bool) error {
	if err := validatePoolUserIDs(userIDs); err != nil {
		return err
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",")
	args := make([]any, 0, len(userIDs))
	for _, userID := range userIDs {
		args = append(args, userID)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pixiv_account WHERE user_id IN (`+placeholders+`)`, args...).Scan(&count); err != nil {
		return err
	}
	if count != len(userIDs) {
		return accountpixiv.ErrNotFound
	}
	args = append([]any{boolInt(schedulable), nowUnix()}, args...)
	if _, err := tx.ExecContext(ctx, `UPDATE pixiv_account SET schedulable=?, updated_at=? WHERE user_id IN (`+placeholders+`)`, args...); err != nil {
		return err
	}
	return tx.Commit()
}

// SetAllPixivSchedulable 在一个事务内更新当前所有 Pixiv 账号。
func (d *DB) SetAllPixivSchedulable(ctx context.Context, schedulable bool) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE pixiv_account SET schedulable=?, updated_at=?`, boolInt(schedulable), nowUnix()); err != nil {
		return err
	}
	return tx.Commit()
}

// ListPixivPoolStatus 返回 non-secret 调度状态，并在同一个事务中清理已过期冻结。
func (d *DB) ListPixivPoolStatus(ctx context.Context, now int64) (accountpixiv.PoolStatus, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return accountpixiv.PoolStatus{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE pixiv_account SET pool_frozen_until=NULL WHERE pool_frozen_until IS NOT NULL AND pool_frozen_until <= ?`, now); err != nil {
		return accountpixiv.PoolStatus{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT user_id, sort_order, schedulable, pool_frozen_until, pool_last_selected FROM pixiv_account ORDER BY sort_order`)
	if err != nil {
		return accountpixiv.PoolStatus{}, err
	}
	status := accountpixiv.PoolStatus{Accounts: make([]accountpixiv.PoolCandidate, 0)}
	for rows.Next() {
		var account accountpixiv.PoolCandidate
		var schedulable, marker int
		if err := rows.Scan(&account.UserID, &account.SortOrder, &schedulable, &account.PoolFrozenUntil, &marker); err != nil {
			_ = rows.Close()
			return accountpixiv.PoolStatus{}, err
		}
		account.Schedulable = schedulable == 1
		account.PoolLastSelected = marker == 1
		account.Eligible = account.Schedulable && (account.PoolFrozenUntil == nil || *account.PoolFrozenUntil <= now)
		if account.PoolFrozenUntil != nil && (status.EarliestFrozenUntil == nil || *account.PoolFrozenUntil < *status.EarliestFrozenUntil) {
			value := *account.PoolFrozenUntil
			status.EarliestFrozenUntil = &value
		}
		status.Accounts = append(status.Accounts, account)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return accountpixiv.PoolStatus{}, err
	}
	if err := rows.Close(); err != nil {
		return accountpixiv.PoolStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return accountpixiv.PoolStatus{}, err
	}
	return status, nil
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

func poolScope(now int64, attemptedUserIDs []int64) (string, []any) {
	scope := `FROM pixiv_account WHERE schedulable=1 AND (pool_frozen_until IS NULL OR pool_frozen_until <= ?)`
	args := []any{now}
	if len(attemptedUserIDs) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(attemptedUserIDs)), ",")
		scope += ` AND user_id NOT IN (` + placeholders + `)`
		for _, userID := range attemptedUserIDs {
			args = append(args, userID)
		}
	}
	return scope, args
}

func earliestFrozenInTx(ctx context.Context, tx *sql.Tx, now int64) (*int64, error) {
	var value sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT MIN(pool_frozen_until) FROM pixiv_account WHERE schedulable=1 AND pool_frozen_until > ?`, now).Scan(&value)
	if err != nil {
		return nil, err
	}
	if !value.Valid {
		return nil, nil
	}
	return &value.Int64, nil
}

func scanPoolCandidates(rows *sql.Rows, now int64) ([]accountpixiv.PoolCandidate, error) {
	out := make([]accountpixiv.PoolCandidate, 0)
	for rows.Next() {
		var candidate accountpixiv.PoolCandidate
		var schedulable, marker int
		if err := rows.Scan(&candidate.UserID, &candidate.SortOrder, &schedulable, &candidate.PoolFrozenUntil, &marker); err != nil {
			return nil, err
		}
		candidate.Schedulable = schedulable == 1
		candidate.PoolLastSelected = marker == 1
		candidate.Eligible = candidate.Schedulable && (candidate.PoolFrozenUntil == nil || *candidate.PoolFrozenUntil <= now)
		candidate.PoolFrozenUntil = cloneInt64(candidate.PoolFrozenUntil)
		out = append(out, candidate)
	}
	return out, rows.Err()
}
