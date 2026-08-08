package authdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// PoolStrategy 是 authdb 选择器支持的策略值；config 层负责解析用户输入，
// repository 只接受这两个已验证的值。
type PoolStrategy string

const (
	PoolStrategyRoundRobin PoolStrategy = "round_robin"
	PoolStrategyRandom     PoolStrategy = "random"
)

// PoolSelectionKind 区分账号池没有候选的真实原因，避免 application 猜测
// RetryAdvice 或把本地状态错误伪装成未登录。
type PoolSelectionKind string

const (
	PoolSelectionNoLocalAccount PoolSelectionKind = "no_local_account"
	PoolSelectionNoSchedulable  PoolSelectionKind = "no_schedulable_account"
	PoolSelectionAllFrozen      PoolSelectionKind = "all_frozen"
	PoolSelectionExhausted      PoolSelectionKind = "exhausted"
)

// PoolSelectionError 是 repository 的脱敏选择结果。EarliestFrozenUntil 只来自
// 已经写入 authdb 的 Retry-After 冻结时间。
type PoolSelectionError struct {
	Kind                PoolSelectionKind
	EarliestFrozenUntil *int64
}

func (e *PoolSelectionError) Error() string {
	if e == nil {
		return "authdb: account pool selection failed"
	}
	return fmt.Sprintf("authdb: account pool selection failed: %s", e.Kind)
}

// PixivPoolAccount 是 status/list 使用的非 secret 调度摘要。
type PixivPoolAccount struct {
	UserID           int64
	SortOrder        int64
	Schedulable      bool
	PoolFrozenUntil  *int64
	PoolLastSelected bool
	Eligible         bool
}

// PixivPoolStatus 是一个时间点上的完整调度快照。
type PixivPoolStatus struct {
	Accounts            []PixivPoolAccount
	EarliestFrozenUntil *int64
}

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
		return ErrNotFound
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

// MigratePixivSchedulable 把旧 config 的 UID 列表映射为数据库调度状态。
// 列表中已不存在的 UID 被忽略；整个重映射仍在一个事务内完成。
func (d *DB) MigratePixivSchedulable(ctx context.Context, enabledUserIDs []int64) error {
	if err := validatePoolUserIDsAllowEmpty(enabledUserIDs); err != nil {
		return err
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE pixiv_account SET schedulable=0, updated_at=?`, nowUnix()); err != nil {
		return err
	}
	if len(enabledUserIDs) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(enabledUserIDs)), ",")
		args := make([]any, 0, len(enabledUserIDs)+1)
		args = append(args, nowUnix())
		for _, userID := range enabledUserIDs {
			args = append(args, userID)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE pixiv_account SET schedulable=1, updated_at=? WHERE user_id IN (`+placeholders+`)`, args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListPixivPoolStatus 返回 non-secret 调度状态，并在同一个事务中清理已过期冻结。
func (d *DB) ListPixivPoolStatus(ctx context.Context, now int64) (PixivPoolStatus, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return PixivPoolStatus{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE pixiv_account SET pool_frozen_until=NULL WHERE pool_frozen_until IS NOT NULL AND pool_frozen_until <= ?`, now); err != nil {
		return PixivPoolStatus{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT user_id, sort_order, schedulable, pool_frozen_until, pool_last_selected FROM pixiv_account ORDER BY sort_order`)
	if err != nil {
		return PixivPoolStatus{}, err
	}
	status := PixivPoolStatus{Accounts: make([]PixivPoolAccount, 0)}
	for rows.Next() {
		var account PixivPoolAccount
		var schedulable, marker int
		if err := rows.Scan(&account.UserID, &account.SortOrder, &schedulable, &account.PoolFrozenUntil, &marker); err != nil {
			_ = rows.Close()
			return PixivPoolStatus{}, err
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
		return PixivPoolStatus{}, err
	}
	if err := rows.Close(); err != nil {
		return PixivPoolStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return PixivPoolStatus{}, err
	}
	return status, nil
}

func validatePoolUserIDs(userIDs []int64) error {
	if len(userIDs) == 0 {
		return errors.New("authdb: account pool requires at least one user id")
	}
	return validatePoolUserIDsAllowEmpty(userIDs)
}

func validatePoolUserIDsAllowEmpty(userIDs []int64) error {
	seen := make(map[int64]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			return errors.New("authdb: account pool user id must be positive")
		}
		if _, exists := seen[userID]; exists {
			return fmt.Errorf("authdb: duplicate account pool user id %d", userID)
		}
		seen[userID] = struct{}{}
	}
	return nil
}

func poolRandomIndex(size int) (int, error) {
	if size <= 0 {
		return 0, errors.New("authdb: account pool has no eligible account")
	}
	value, err := rand.Int(rand.Reader, big.NewInt(int64(size)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
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
