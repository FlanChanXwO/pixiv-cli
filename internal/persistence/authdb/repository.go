package authdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PixivAccount 是 pixiv_account 表的一行。RefreshToken 只保存 token 值本身，
// 不保存 Cookie、header 或路径。所有时间使用 UTC Unix seconds。
type PixivAccount struct {
	UserID             int64
	SortOrder          int64
	Username           string
	RefreshToken       []byte
	CredentialRevision int64
	PremiumStatus      *bool
	PremiumCheckedAt   *int64
	PoolFrozenUntil    *int64
	PoolLastSelected   bool
	CreatedAt          int64
	UpdatedAt          int64
	Schedulable        bool
}

// FanboxAccount 是 fanbox_account 表的一行。SessionID 只保存非空 FANBOXSESSID
// value。
type FanboxAccount struct {
	UserID             int64
	SortOrder          int64
	DisplayName        string
	CreatorID          string
	SessionID          []byte
	CredentialRevision int64
	ValidatedAt        int64
	CreatedAt          int64
	UpdatedAt          int64
}

// ErrNotFound 表示按 ID 找不到账号。
var (
	ErrNotFound           = errors.New("authdb: account not found")
	ErrCredentialConflict = errors.New("authdb: credential revision conflict")
)

const pixivAccountColumns = `user_id, sort_order, username, refresh_token, credential_revision,
	premium_status, premium_checked_at, pool_frozen_until, pool_last_selected, created_at, updated_at, schedulable`

const fanboxAccountColumns = `user_id, sort_order, display_name, creator_id, session_id,
	credential_revision, validated_at, created_at, updated_at`

// SavePixivCredential 保存一次已验证的 Pixiv credential。
// 新账号固定从 revision 1 开始；已有账号在 SQL 内递增 revision，不能由调用方回退。
func (d *DB) SavePixivCredential(ctx context.Context, account PixivAccount) error {
	if account.UserID <= 0 || len(account.RefreshToken) == 0 {
		return errors.New("authdb: invalid pixiv account")
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := savePixivCredentialTx(ctx, tx, account); err != nil {
		return err
	}
	return tx.Commit()
}

// SavePixivCredentials 在一个事务中保存一组首次导入的 Pixiv credentials。
// 迁移调用它保证任一账号失败时不会留下部分提交。
func (d *DB) SavePixivCredentials(ctx context.Context, accounts []PixivAccount) error {
	seen := make(map[int64]struct{}, len(accounts))
	for _, account := range accounts {
		if account.UserID <= 0 || len(account.RefreshToken) == 0 {
			return errors.New("authdb: invalid pixiv account")
		}
		if _, ok := seen[account.UserID]; ok {
			return fmt.Errorf("authdb: duplicate pixiv account %d", account.UserID)
		}
		seen[account.UserID] = struct{}{}
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, account := range accounts {
		if err := savePixivCredentialTx(ctx, tx, account); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func savePixivCredentialTx(ctx context.Context, tx *sql.Tx, account PixivAccount) error {
	now := time.Now().UTC().Unix()
	var sortOrder int64
	err := tx.QueryRowContext(ctx, `SELECT sort_order FROM pixiv_account WHERE user_id = ?`, account.UserID).Scan(&sortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		sortOrder = account.SortOrder
		if sortOrder <= 0 {
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM pixiv_account`).Scan(&sortOrder); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO pixiv_account (`+pixivAccountColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			account.UserID, sortOrder, account.Username, account.RefreshToken, 1,
			boolPtrToIntPtr(account.PremiumStatus), account.PremiumCheckedAt, account.PoolFrozenUntil, boolInt(account.PoolLastSelected),
			now, now, 1)
	} else if err != nil {
		return err
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE pixiv_account SET username=?, refresh_token=?,
			credential_revision=credential_revision+1, updated_at=? WHERE user_id=?`,
			account.Username, account.RefreshToken, now, account.UserID)
	}
	return err
}

// UpdatePixivMetadata 更新会员快照等非 credential 元数据，不改变 revision 或 token。
func (d *DB) UpdatePixivMetadata(ctx context.Context, userID int64, username string, premiumStatus *bool, premiumCheckedAt *int64) error {
	if userID <= 0 {
		return errors.New("authdb: invalid pixiv metadata input")
	}
	result, err := d.db.ExecContext(ctx, `UPDATE pixiv_account SET username=?, premium_status=?, premium_checked_at=?, updated_at=? WHERE user_id=?`,
		username, boolPtrToIntPtr(premiumStatus), premiumCheckedAt, time.Now().UTC().Unix(), userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// RotatePixivCredentials 提交一次 refresh token rotation：更新 token 并递增
// credential_revision。
func (d *DB) RotatePixivCredentials(ctx context.Context, userID, expectedRevision int64, refreshToken []byte) error {
	if userID <= 0 || expectedRevision <= 0 || len(refreshToken) == 0 {
		return errors.New("authdb: invalid rotation input")
	}
	result, err := d.db.ExecContext(ctx,
		`UPDATE pixiv_account SET refresh_token=?, credential_revision=credential_revision+1, updated_at=? WHERE user_id=? AND credential_revision=?`,
		refreshToken, time.Now().UTC().Unix(), userID, expectedRevision)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var exists int
		if err := d.db.QueryRowContext(ctx, `SELECT 1 FROM pixiv_account WHERE user_id=?`, userID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		return ErrCredentialConflict
	}
	return nil
}

// ListPixiv 返回全部 Pixiv 账号，按 sort_order 升序。
func (d *DB) ListPixiv(ctx context.Context) ([]PixivAccount, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+pixivAccountColumns+` FROM pixiv_account ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPixivAccounts(rows)
}

// GetPixiv 返回指定 user_id 的 Pixiv 账号；不存在返回 ErrNotFound。
func (d *DB) GetPixiv(ctx context.Context, userID int64) (PixivAccount, error) {
	row := d.db.QueryRowContext(ctx, `SELECT `+pixivAccountColumns+` FROM pixiv_account WHERE user_id = ?`, userID)
	account, err := scanPixivAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return PixivAccount{}, ErrNotFound
	}
	return account, err
}

// RemovePixiv 删除一个 Pixiv 账号，其 pool 标记随之自然删除。
func (d *DB) RemovePixiv(ctx context.Context, userID int64) error {
	result, err := d.db.ExecContext(ctx, `DELETE FROM pixiv_account WHERE user_id = ?`, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// SelectPooledPixiv 在一个短事务内从 schedulable 且未冻结的账号中选择下一个
// 候选，并移动 pool_last_selected 标记。attemptedUserIDs 只属于当前 operation，
// 不会写入数据库；过期冻结在选择时清理。
func (d *DB) SelectPooledPixiv(ctx context.Context, now int64, strategy PoolStrategy, attemptedUserIDs []int64) (PixivAccount, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return PixivAccount{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE pixiv_account SET pool_frozen_until = NULL WHERE pool_frozen_until IS NOT NULL AND pool_frozen_until <= ?`, now); err != nil {
		return PixivAccount{}, err
	}
	var total, schedulable, eligible int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pixiv_account`).Scan(&total); err != nil {
		return PixivAccount{}, err
	}
	if total == 0 {
		return PixivAccount{}, &PoolSelectionError{Kind: PoolSelectionNoLocalAccount}
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pixiv_account WHERE schedulable=1`).Scan(&schedulable); err != nil {
		return PixivAccount{}, err
	}
	if schedulable == 0 {
		return PixivAccount{}, &PoolSelectionError{Kind: PoolSelectionNoSchedulable}
	}
	eligibleScope, eligibleArgs := poolScope(now, nil)
	if err := tx.QueryRowContext(ctx, `SELECT count(*) `+eligibleScope, eligibleArgs...).Scan(&eligible); err != nil {
		return PixivAccount{}, err
	}
	if eligible == 0 {
		earliest, err := earliestFrozenInTx(ctx, tx, now)
		if err != nil {
			return PixivAccount{}, err
		}
		return PixivAccount{}, &PoolSelectionError{Kind: PoolSelectionAllFrozen, EarliestFrozenUntil: earliest}
	}
	scope, args := poolScope(now, attemptedUserIDs)
	rows, err := tx.QueryContext(ctx, `SELECT `+pixivAccountColumns+` `+scope+` ORDER BY sort_order`, args...)
	if err != nil {
		return PixivAccount{}, err
	}
	candidates, err := scanPixivAccounts(rows)
	if err != nil {
		_ = rows.Close()
		return PixivAccount{}, err
	}
	if err := rows.Close(); err != nil {
		return PixivAccount{}, err
	}
	if len(candidates) == 0 {
		earliest, err := earliestFrozenInTx(ctx, tx, now)
		if err != nil {
			return PixivAccount{}, err
		}
		return PixivAccount{}, &PoolSelectionError{Kind: PoolSelectionExhausted, EarliestFrozenUntil: earliest}
	}
	var markerSort sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT sort_order FROM pixiv_account WHERE pool_last_selected=1`).Scan(&markerSort); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return PixivAccount{}, err
	}
	chosenIndex := 0
	switch strategy {
	case PoolStrategyRoundRobin:
		if markerSort.Valid {
			for index, candidate := range candidates {
				if candidate.SortOrder > markerSort.Int64 {
					chosenIndex = index
					break
				}
			}
		}
	case PoolStrategyRandom:
		randomIndex := d.poolRandom
		if randomIndex == nil {
			randomIndex = poolRandomIndex
		}
		chosenIndex, err = randomIndex(len(candidates))
		if err != nil {
			return PixivAccount{}, err
		}
		if chosenIndex < 0 || chosenIndex >= len(candidates) {
			return PixivAccount{}, errors.New("authdb: account pool random source returned an invalid index")
		}
	default:
		return PixivAccount{}, fmt.Errorf("authdb: unsupported account pool strategy %q", strategy)
	}
	chosen := candidates[chosenIndex]
	if _, err := tx.ExecContext(ctx, `UPDATE pixiv_account SET pool_last_selected = 0 WHERE pool_last_selected = 1`); err != nil {
		return PixivAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE pixiv_account SET pool_last_selected = 1, updated_at = ? WHERE user_id = ?`, now, chosen.UserID); err != nil {
		return PixivAccount{}, err
	}
	if err := tx.Commit(); err != nil {
		return PixivAccount{}, err
	}
	chosen.PoolLastSelected = true
	return chosen, nil
}

// FreezePooledPixiv 记录一个账号的有效冻结时间。
func (d *DB) FreezePooledPixiv(ctx context.Context, userID, frozenUntil int64) error {
	result, err := d.db.ExecContext(ctx, `UPDATE pixiv_account SET pool_frozen_until=CASE WHEN pool_frozen_until IS NULL OR pool_frozen_until < ? THEN ? ELSE pool_frozen_until END, updated_at=? WHERE user_id=?`,
		frozenUntil, frozenUntil, time.Now().UTC().Unix(), userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// SaveFanboxCredential 保存一次已验证的 FANBOX credential。
// 新账号固定从 revision 1 开始；已有账号在 SQL 内递增 revision。
func (d *DB) SaveFanboxCredential(ctx context.Context, account FanboxAccount) error {
	if account.UserID <= 0 || len(account.SessionID) == 0 || account.ValidatedAt <= 0 {
		return errors.New("authdb: invalid fanbox account")
	}
	now := time.Now().UTC().Unix()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var sortOrder int64
	err = tx.QueryRowContext(ctx, `SELECT sort_order FROM fanbox_account WHERE user_id = ?`, account.UserID).Scan(&sortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		sortOrder = account.SortOrder
		if sortOrder <= 0 {
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM fanbox_account`).Scan(&sortOrder); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO fanbox_account (`+fanboxAccountColumns+`) VALUES (?,?,?,?,?,?,?,?,?)`,
			account.UserID, sortOrder, account.DisplayName, account.CreatorID, account.SessionID,
			1, account.ValidatedAt, now, now)
	} else if err != nil {
		return err
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE fanbox_account SET display_name=?, creator_id=?, session_id=?,
			credential_revision=credential_revision+1, validated_at=?, updated_at=? WHERE user_id=?`,
			account.DisplayName, account.CreatorID, account.SessionID,
			account.ValidatedAt, now, account.UserID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// RotateFanboxSession 更新 FANBOX session 并递增 credential_revision。
func (d *DB) RotateFanboxSession(ctx context.Context, userID, expectedRevision int64, session []byte, validatedAt int64) error {
	if userID <= 0 || expectedRevision <= 0 || len(session) == 0 || validatedAt <= 0 {
		return errors.New("authdb: invalid fanbox rotation input")
	}
	result, err := d.db.ExecContext(ctx,
		`UPDATE fanbox_account SET session_id=?, credential_revision=credential_revision+1, validated_at=?, updated_at=? WHERE user_id=? AND credential_revision=?`,
		session, validatedAt, time.Now().UTC().Unix(), userID, expectedRevision)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var exists int
		if err := d.db.QueryRowContext(ctx, `SELECT 1 FROM fanbox_account WHERE user_id=?`, userID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		return ErrCredentialConflict
	}
	return nil
}

// ListFanbox 返回全部 FANBOX 账号，按 sort_order 升序。
func (d *DB) ListFanbox(ctx context.Context) ([]FanboxAccount, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+fanboxAccountColumns+` FROM fanbox_account ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FanboxAccount{}
	for rows.Next() {
		var account FanboxAccount
		if err := rows.Scan(&account.UserID, &account.SortOrder, &account.DisplayName, &account.CreatorID,
			&account.SessionID, &account.CredentialRevision, &account.ValidatedAt, &account.CreatedAt, &account.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, account)
	}
	return out, rows.Err()
}

// GetFanbox 返回指定 user_id 的 FANBOX 账号；不存在返回 ErrNotFound。
func (d *DB) GetFanbox(ctx context.Context, userID int64) (FanboxAccount, error) {
	row := d.db.QueryRowContext(ctx, `SELECT `+fanboxAccountColumns+` FROM fanbox_account WHERE user_id = ?`, userID)
	var account FanboxAccount
	err := row.Scan(&account.UserID, &account.SortOrder, &account.DisplayName, &account.CreatorID,
		&account.SessionID, &account.CredentialRevision, &account.ValidatedAt, &account.CreatedAt, &account.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return FanboxAccount{}, ErrNotFound
	}
	return account, err
}

// RemoveFanbox 删除一个 FANBOX 账号。
func (d *DB) RemoveFanbox(ctx context.Context, userID int64) error {
	result, err := d.db.ExecContext(ctx, `DELETE FROM fanbox_account WHERE user_id = ?`, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func scanPixivAccounts(rows *sql.Rows) ([]PixivAccount, error) {
	out := []PixivAccount{}
	for rows.Next() {
		account, err := scanPixivAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, account)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPixivAccount(scanner rowScanner) (PixivAccount, error) {
	var account PixivAccount
	var premiumStatus *int64
	var poolLastSelected int
	err := scanner.Scan(&account.UserID, &account.SortOrder, &account.Username, &account.RefreshToken, &account.CredentialRevision,
		&premiumStatus, &account.PremiumCheckedAt, &account.PoolFrozenUntil, &poolLastSelected,
		&account.CreatedAt, &account.UpdatedAt, &account.Schedulable)
	account.PremiumStatus = intPtrToBoolPtr(premiumStatus)
	account.PoolLastSelected = poolLastSelected == 1
	return account, err
}

func boolPtrToIntPtr(value *bool) *int64 {
	if value == nil {
		return nil
	}
	if *value {
		one := int64(1)
		return &one
	}
	zero := int64(0)
	return &zero
}

func intPtrToBoolPtr(value *int64) *bool {
	if value == nil {
		return nil
	}
	out := *value == 1
	return &out
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
