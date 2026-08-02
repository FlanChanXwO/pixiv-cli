package authdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
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
var ErrNotFound = errors.New("authdb: account not found")

const pixivAccountColumns = `user_id, sort_order, username, refresh_token, credential_revision,
	premium_status, premium_checked_at, pool_frozen_until, pool_last_selected, created_at, updated_at`

const fanboxAccountColumns = `user_id, sort_order, display_name, creator_id, session_id,
	credential_revision, validated_at, created_at, updated_at`

// UpsertPixiv 插入或更新一个 Pixiv 账号。新账号获得 max(sort_order)+1，删除
// 其他账号不会重排。同一身份重新导入时保留原 sort_order。
func (d *DB) UpsertPixiv(ctx context.Context, account PixivAccount) error {
	if account.UserID <= 0 || len(account.RefreshToken) == 0 || account.CredentialRevision <= 0 {
		return errors.New("authdb: invalid pixiv account")
	}
	now := time.Now().UTC().Unix()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var sortOrder int64
	err = tx.QueryRowContext(ctx, `SELECT sort_order FROM pixiv_account WHERE user_id = ?`, account.UserID).Scan(&sortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		sortOrder = account.SortOrder
		if sortOrder <= 0 {
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM pixiv_account`).Scan(&sortOrder); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO pixiv_account (`+pixivAccountColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			account.UserID, sortOrder, account.Username, account.RefreshToken, account.CredentialRevision,
			boolPtrToIntPtr(account.PremiumStatus), account.PremiumCheckedAt, account.PoolFrozenUntil, boolInt(account.PoolLastSelected),
			now, now)
	} else if err != nil {
		return err
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE pixiv_account SET username=?, refresh_token=?, credential_revision=?,
			premium_status=?, premium_checked_at=?, pool_frozen_until=?, pool_last_selected=?, updated_at=?
			WHERE user_id=?`,
			account.Username, account.RefreshToken, account.CredentialRevision,
			boolPtrToIntPtr(account.PremiumStatus), account.PremiumCheckedAt, account.PoolFrozenUntil,
			boolInt(account.PoolLastSelected), now, account.UserID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// RotatePixivCredentials 提交一次 refresh token rotation：更新 token 并递增
// credential_revision。
func (d *DB) RotatePixivCredentials(ctx context.Context, userID int64, refreshToken []byte) error {
	if userID <= 0 || len(refreshToken) == 0 {
		return errors.New("authdb: invalid rotation input")
	}
	result, err := d.db.ExecContext(ctx,
		`UPDATE pixiv_account SET refresh_token=?, credential_revision=credential_revision+1, updated_at=? WHERE user_id=?`,
		refreshToken, time.Now().UTC().Unix(), userID)
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

// SelectPooledPixiv 在一个短事务内选择 account-pool 的下一个候选账号并移动
// pool_last_selected 标记。allowedUserIDs 非空时只在这些账号中选择。过期的
// pool_frozen_until 视为未冻结并顺带清理。没有候选时返回 ErrNotFound。
func (d *DB) SelectPooledPixiv(ctx context.Context, now int64, allowedUserIDs []int64) (PixivAccount, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return PixivAccount{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE pixiv_account SET pool_frozen_until = NULL WHERE pool_frozen_until IS NOT NULL AND pool_frozen_until <= ?`, now); err != nil {
		return PixivAccount{}, err
	}
	scope := `FROM pixiv_account WHERE (pool_frozen_until IS NULL OR pool_frozen_until <= ?)`
	args := []any{now}
	if len(allowedUserIDs) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(allowedUserIDs)), ",")
		scope += ` AND user_id IN (` + placeholders + `)`
		for _, id := range allowedUserIDs {
			args = append(args, id)
		}
	}
	var chosen PixivAccount
	// 优先尚未标记为 last-selected 的账号，实现轮换；全部已标记时回退到
	// sort_order 最小者。
	row := tx.QueryRowContext(ctx, `SELECT `+pixivAccountColumns+` `+scope+` ORDER BY pool_last_selected ASC, sort_order ASC LIMIT 1`, args...)
	chosen, err = scanPixivAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return PixivAccount{}, ErrNotFound
	}
	if err != nil {
		return PixivAccount{}, err
	}
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
	result, err := d.db.ExecContext(ctx, `UPDATE pixiv_account SET pool_frozen_until=?, updated_at=? WHERE user_id=?`,
		frozenUntil, time.Now().UTC().Unix(), userID)
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

// UpsertFanbox 插入或更新一个 FANBOX 账号。
func (d *DB) UpsertFanbox(ctx context.Context, account FanboxAccount) error {
	if account.UserID <= 0 || len(account.SessionID) == 0 || account.CredentialRevision <= 0 || account.ValidatedAt <= 0 {
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
			account.CredentialRevision, account.ValidatedAt, now, now)
	} else if err != nil {
		return err
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE fanbox_account SET display_name=?, creator_id=?, session_id=?,
			credential_revision=?, validated_at=?, updated_at=? WHERE user_id=?`,
			account.DisplayName, account.CreatorID, account.SessionID,
			account.CredentialRevision, account.ValidatedAt, now, account.UserID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// RotateFanboxSession 更新 FANBOX session 并递增 credential_revision。
func (d *DB) RotateFanboxSession(ctx context.Context, userID int64, session []byte, validatedAt int64) error {
	if userID <= 0 || len(session) == 0 {
		return errors.New("authdb: invalid fanbox rotation input")
	}
	result, err := d.db.ExecContext(ctx,
		`UPDATE fanbox_account SET session_id=?, credential_revision=credential_revision+1, validated_at=?, updated_at=? WHERE user_id=?`,
		session, validatedAt, time.Now().UTC().Unix(), userID)
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
		&account.CreatedAt, &account.UpdatedAt)
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
